package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
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

func (h *Handlers) findEmailSendout(id string) (*core.Record, error) {
	rec, err := h.app.FindRecordById(sendouts.SendoutsCollection, id)
	if err != nil {
		return nil, apis.NewNotFoundError("email sendout not found", err)
	}
	return rec, nil
}

// listEmailSendouts covers publicEmailSendout's fields only, so a history
// page never pulls every Sendout's stored html/text/content off disk.
func (h *Handlers) listEmailSendouts(e *core.RequestEvent) error {
	var records []*core.Record
	err := h.app.RecordQuery(sendouts.SendoutsCollection).
		Select("id", "subject", "language", "audience", "created_by_email", "cancelled_at", "created").
		OrderBy("created DESC").
		All(&records)
	if err != nil {
		return apis.NewBadRequestError("failed to list email sendouts", err)
	}
	counts, err := sendouts.CountRecipients(h.app, "")
	if err != nil {
		return apis.NewBadRequestError("failed to count recipients", err)
	}
	out := make([]EmailSendout, 0, len(records))
	for _, rec := range records {
		out = append(out, publicEmailSendout(rec, counts[rec.Id]))
	}
	return ok(e, out)
}

func (h *Handlers) getEmailSendout(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	counts, err := sendouts.CountRecipients(h.app, rec.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to count recipients", err)
	}
	return ok(e, EmailSendoutDetail{
		EmailSendout: publicEmailSendout(rec, counts[rec.Id]),
		HTML:         rec.GetString("html"),
		Text:         rec.GetString("text"),
	})
}

func (h *Handlers) listEmailSendoutRecipients(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	filter, params := "sendout = {:id}", dbx.Params{"id": rec.Id}
	if status := e.Request.URL.Query().Get("status"); status != "" {
		if !slices.Contains(sendouts.RecipientStatuses, status) {
			return apis.NewBadRequestError("unknown recipient status", nil)
		}
		filter += " && status = {:status}"
		params["status"] = status
	}
	rows, err := h.app.FindRecordsByFilter(sendouts.RecipientsCollection, filter, "email", 0, 0, params)
	if err != nil {
		return apis.NewBadRequestError("failed to list recipients", err)
	}
	out := make([]EmailSendoutRecipient, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicSendoutRecipient(row))
	}
	return ok(e, out)
}

func publicSendoutRecipient(row *core.Record) EmailSendoutRecipient {
	var reason *string
	if value := row.GetString("reason"); value != "" {
		reason = &value
	}
	var lastAttemptAt *string
	if value := row.GetString("last_attempt_at"); value != "" {
		lastAttemptAt = &value
	}
	var sentAt *string
	if value := row.GetString("sent_at"); value != "" {
		sentAt = &value
	}
	return EmailSendoutRecipient{
		ID:            row.Id,
		UserID:        row.GetString("user"),
		Email:         row.GetString("email"),
		Status:        row.GetString("status"),
		Reason:        reason,
		Attempts:      row.GetInt("attempts"),
		QueuedAt:      row.GetString("queued_at"),
		LastAttemptAt: lastAttemptAt,
		SentAt:        sentAt,
	}
}

func (h *Handlers) cancelEmailSendout(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := sendouts.Cancel(h.app, rec, time.Now().UTC()); err != nil {
		return sendoutStateError(err, "cancel")
	}
	return h.respondEmailSendout(e, rec)
}

func (h *Handlers) resendEmailSendout(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in EmailSendoutResendRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if !h.smtpEnabled() {
		return errSMTPUnavailable()
	}
	requeued, err := sendouts.Requeue(h.app, rec, in.RecipientIDs, time.Now().UTC())
	if err != nil {
		return sendoutStateError(err, "resend")
	}
	if requeued > 0 {
		h.sendoutQueue.Notify()
	}
	return ok(e, EmailSendoutResendResponse{Requeued: requeued})
}

func sendoutStateError(err error, action string) error {
	switch {
	case errors.Is(err, sendouts.ErrCancelled):
		return apis.NewBadRequestError("email sendout is cancelled", nil)
	case errors.Is(err, sendouts.ErrNothingToCancel):
		return apis.NewBadRequestError("email sendout has no pending recipients", nil)
	}
	return apis.NewBadRequestError("failed to "+action+" email sendout", err)
}
