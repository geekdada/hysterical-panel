package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/sendouts"
)

const (
	maxSendoutSubjectRunes = 200
	maxSendoutHTMLBytes    = 1_000_000
	maxSendoutTextBytes    = 256_000
	maxSendoutContentBytes = 1 << 20
	sendoutCandidateLimit  = 20
)

// sendoutNotifier wakes the Sendout worker after rows are queued.
type sendoutNotifier interface {
	Notify()
}

func errSMTPUnavailable() error {
	return apis.NewApiError(http.StatusServiceUnavailable, "email sendouts are unavailable: SMTP is not configured", nil)
}

func (h *Handlers) emailSendoutContext(e *core.RequestEvent) error {
	count, err := sendouts.CountEligible(h.app)
	if err != nil {
		return apis.NewBadRequestError("failed to count eligible recipients", err)
	}
	return ok(e, EmailSendoutContextResponse{
		AppName:                h.app.Settings().Meta.AppName,
		FrontendURL:            h.publicConfig.FrontendURL,
		SMTPEnabled:            h.smtpEnabled(),
		EligibleRecipientCount: count,
		RatePerMinute:          sendouts.RatePerMinute(h.app),
	})
}

func (h *Handlers) listEmailSendoutCandidates(e *core.RequestEvent) error {
	search := strings.TrimSpace(e.Request.URL.Query().Get("search"))
	users, err := sendouts.FindEligible(h.app, search, sendoutCandidateLimit)
	if err != nil {
		return apis.NewBadRequestError("failed to search recipients", err)
	}
	out := make([]EmailSendoutRecipientCandidate, 0, len(users))
	for _, user := range users {
		out = append(out, EmailSendoutRecipientCandidate{ID: user.Id, Email: user.Email()})
	}
	return ok(e, out)
}

func (h *Handlers) createEmailSendout(e *core.RequestEvent) error {
	var in EmailSendoutCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if !h.smtpEnabled() {
		return errSMTPUnavailable()
	}
	draft, err := validateEmailSendout(in)
	if err != nil {
		return err
	}
	users, err := h.sendoutAudience(draft.Audience, in.UserID)
	if err != nil {
		return err
	}
	rec, err := sendouts.Create(h.app, draft, users, e.Auth, time.Now().UTC())
	if err != nil {
		return apis.NewBadRequestError("failed to save email sendout", err)
	}
	h.sendoutQueue.Notify()
	return h.respondEmailSendout(e, rec)
}

// sendoutAudience expands "all" at creation time; Users who become eligible
// later do not receive this Sendout.
func (h *Handlers) sendoutAudience(audience, userID string) ([]*core.Record, error) {
	if audience == sendouts.AudienceAll {
		users, err := sendouts.FindEligible(h.app, "", 0)
		if err != nil {
			return nil, apis.NewBadRequestError("failed to load recipients", err)
		}
		if len(users) == 0 {
			return nil, apis.NewBadRequestError("no users are active and verified", nil)
		}
		return users, nil
	}
	user, err := h.app.FindRecordById("users", userID)
	if err != nil {
		return nil, apis.NewBadRequestError("recipient not found", nil)
	}
	if !sendouts.Eligible(user) {
		return nil, apis.NewBadRequestError("recipient must be active and verified", nil)
	}
	return []*core.Record{user}, nil
}

func validateEmailSendout(in EmailSendoutCreateRequest) (sendouts.Draft, error) {
	subject, err := normalizeSendoutSubject(in.Subject)
	if err != nil {
		return sendouts.Draft{}, err
	}
	if !slices.Contains(sendouts.Languages, in.Language) {
		return sendouts.Draft{}, apis.NewBadRequestError("language must be en or zh-cn", nil)
	}
	if err := validateSendoutAudience(in.Audience, in.UserID); err != nil {
		return sendouts.Draft{}, err
	}
	if err := validateSendoutBody(in); err != nil {
		return sendouts.Draft{}, err
	}
	return sendouts.Draft{Subject: subject, Language: in.Language, Audience: in.Audience, HTML: in.HTML, Text: in.Text, Content: in.Content}, nil
}

func normalizeSendoutSubject(input string) (string, error) {
	subject := strings.TrimSpace(input)
	if subject == "" || utf8.RuneCountInString(subject) > maxSendoutSubjectRunes {
		return "", apis.NewBadRequestError("subject must be between 1 and 200 characters", nil)
	}
	// Control characters include CR and LF, which would let a subject inject mail headers.
	if strings.IndexFunc(subject, unicode.IsControl) >= 0 {
		return "", apis.NewBadRequestError("subject cannot contain control characters", nil)
	}
	return subject, nil
}

func validateSendoutAudience(audience, userID string) error {
	switch {
	case audience == sendouts.AudienceAll && userID == "", audience == sendouts.AudienceSingle && userID != "":
		return nil
	case audience == sendouts.AudienceAll:
		return apis.NewBadRequestError("user_id is only allowed for a single recipient", nil)
	case audience == sendouts.AudienceSingle:
		return apis.NewBadRequestError("user_id is required for a single recipient", nil)
	}
	return apis.NewBadRequestError("audience must be single or all", nil)
}

func validateSendoutBody(in EmailSendoutCreateRequest) error {
	if strings.TrimSpace(in.HTML) == "" || strings.TrimSpace(in.Text) == "" {
		return apis.NewBadRequestError("html and text are required", nil)
	}
	if len(in.HTML) > maxSendoutHTMLBytes || len(in.Text) > maxSendoutTextBytes {
		return apis.NewBadRequestError("email content is too large", nil)
	}
	if in.Content == nil {
		return apis.NewBadRequestError("content is required", nil)
	}
	raw, err := json.Marshal(in.Content)
	if err != nil || len(raw) > maxSendoutContentBytes {
		return apis.NewBadRequestError("content is too large", nil)
	}
	return nil
}

func (h *Handlers) respondEmailSendout(e *core.RequestEvent, rec *core.Record) error {
	counts, err := sendouts.CountRecipients(h.app, rec.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to count recipients", err)
	}
	return ok(e, publicEmailSendout(rec, counts[rec.Id]))
}

func publicEmailSendout(rec *core.Record, counts sendouts.Counts) EmailSendout {
	var cancelledAt *string
	if value := rec.GetString("cancelled_at"); value != "" {
		cancelledAt = &value
	}
	return EmailSendout{
		ID:       rec.Id,
		Subject:  rec.GetString("subject"),
		Language: rec.GetString("language"),
		Audience: rec.GetString("audience"),
		Status:   sendouts.Status(rec, counts),
		Counts: EmailSendoutCounts{
			Total: counts.Total, Pending: counts.Pending, Sent: counts.Sent,
			Failed: counts.Failed, Skipped: counts.Skipped, Cancelled: counts.Cancelled,
		},
		CreatedByEmail: rec.GetString("created_by_email"),
		Created:        rec.GetString("created"),
		CancelledAt:    cancelledAt,
	}
}
