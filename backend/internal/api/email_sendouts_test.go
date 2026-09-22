package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/sendouts"
)

func sendoutEvent(
	t *testing.T,
	app core.App,
	method, target, id string,
	body any,
	auth *core.Record,
) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	raw := []byte("{}")
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	e := &core.RequestEvent{App: app, Event: router.Event{Request: req, Response: response}}
	e.Auth = auth
	return e, response
}

func sendoutAPIStatus(t *testing.T, err error) int {
	t.Helper()
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an ApiError", err)
	}
	return apiErr.Status
}

func TestSettingsEmailSendoutRate(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}

	e, response := sendoutEvent(t, app, http.MethodGet, "/api/panel/settings", "", nil, nil)
	if err := h.getSettings(e); err != nil {
		t.Fatalf("getSettings() error = %v", err)
	}
	var current SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil || current.EmailSendoutRatePerMinute != 30 {
		t.Fatalf("default rate = %d (%v), want 30", current.EmailSendoutRatePerMinute, err)
	}

	for _, rate := range []int{0, 601} {
		e, _ := sendoutEvent(t, app, http.MethodPatch, "/api/panel/settings", "", SettingsUpdateRequest{EmailSendoutRatePerMinute: &rate}, nil)
		if err := h.updateSettings(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
			t.Fatalf("updateSettings(rate=%d) error = %v, want 400", rate, err)
		}
	}

	rate := 45
	e, response = sendoutEvent(t, app, http.MethodPatch, "/api/panel/settings", "", SettingsUpdateRequest{EmailSendoutRatePerMinute: &rate}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("updateSettings(rate=45) error = %v", err)
	}
	var updated SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil || updated.EmailSendoutRatePerMinute != 45 {
		t.Fatalf("updated rate = %d (%v), want 45", updated.EmailSendoutRatePerMinute, err)
	}
}

type fakeSendoutNotifier struct{ calls int }

func (f *fakeSendoutNotifier) Notify() { f.calls++ }

func newSendoutTestHandlers(t *testing.T) (*Handlers, *fakeSendoutNotifier, *core.Record) {
	t.Helper()
	app := newMigratedTestApp(t)
	app.Settings().SMTP.Enabled = true
	admin := newUsersTestRecord(t, app, "admin@example.com", "AdminSecret")
	admin.Set("role", "admin")
	if err := app.Save(admin); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	notifier := &fakeSendoutNotifier{}
	return &Handlers{app: app, sendoutQueue: notifier}, notifier, admin
}

func newSendoutTestUser(t *testing.T, app core.App, email, status string, verified bool) *core.Record {
	t.Helper()
	user := newUsersTestRecord(t, app, email, strings.ReplaceAll(email, "@", "-at-")+"-secret")
	user.Set("status", status)
	user.Set("verified", verified)
	if err := app.Save(user); err != nil {
		t.Fatalf("update test user: %v", err)
	}
	return user
}

func validSendoutRequest() EmailSendoutCreateRequest {
	return EmailSendoutCreateRequest{
		Subject:  "  Scheduled maintenance  ",
		Language: "en",
		Audience: "all",
		HTML:     "<!DOCTYPE html><html><body><p>Maintenance tonight.</p></body></html>",
		Text:     "Maintenance tonight.",
		Content:  map[string]any{"type": "doc"},
	}
}

func TestValidateEmailSendout(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*EmailSendoutCreateRequest)
		ok     bool
	}{
		{"valid broadcast", func(*EmailSendoutCreateRequest) {}, true},
		{"valid single", func(in *EmailSendoutCreateRequest) { in.Audience, in.UserID = "single", "user1" }, true},
		{"blank subject", func(in *EmailSendoutCreateRequest) { in.Subject = "   " }, false},
		{"header injection", func(in *EmailSendoutCreateRequest) { in.Subject = "Hi\r\nBcc: x@example.com" }, false},
		{"long subject", func(in *EmailSendoutCreateRequest) { in.Subject = strings.Repeat("a", 201) }, false},
		{"unknown language", func(in *EmailSendoutCreateRequest) { in.Language = "fr" }, false},
		{"single without user", func(in *EmailSendoutCreateRequest) { in.Audience = "single" }, false},
		{"broadcast with user", func(in *EmailSendoutCreateRequest) { in.UserID = "user1" }, false},
		{"unknown audience", func(in *EmailSendoutCreateRequest) { in.Audience = "admins" }, false},
		{"blank html", func(in *EmailSendoutCreateRequest) { in.HTML = " " }, false},
		{"blank text", func(in *EmailSendoutCreateRequest) { in.Text = "" }, false},
		{"oversized html", func(in *EmailSendoutCreateRequest) { in.HTML = strings.Repeat("a", 1_000_001) }, false},
		{"missing content", func(in *EmailSendoutCreateRequest) { in.Content = nil }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validSendoutRequest()
			tc.mutate(&in)
			draft, err := validateEmailSendout(in)
			if tc.ok != (err == nil) {
				t.Fatalf("validateEmailSendout() error = %v, want ok=%v", err, tc.ok)
			}
			if tc.ok && draft.Subject != "Scheduled maintenance" {
				t.Fatalf("subject = %q, want trimmed", draft.Subject)
			}
		})
	}
}

func TestCreateEmailSendoutQueuesEligibleUsersOnly(t *testing.T) {
	h, notifier, admin := newSendoutTestHandlers(t)
	newSendoutTestUser(t, h.app, "active@example.com", "active", true)
	newSendoutTestUser(t, h.app, "disabled@example.com", "disabled", true)
	newSendoutTestUser(t, h.app, "unverified@example.com", "active", false)

	e, response := sendoutEvent(t, h.app, http.MethodPost, "/api/panel/email-sendouts", "", validSendoutRequest(), admin)
	if err := h.createEmailSendout(e); err != nil {
		t.Fatalf("createEmailSendout() error = %v", err)
	}
	var created EmailSendout
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Counts.Total != 2 || created.Counts.Pending != 2 || created.Status != "sending" || created.CreatedByEmail != "admin@example.com" {
		t.Fatalf("created = %+v, want 2 pending recipients created by admin", created)
	}
	if notifier.calls != 1 {
		t.Fatalf("worker notified %d times, want 1", notifier.calls)
	}
	rows, err := h.app.FindRecordsByFilter(sendouts.RecipientsCollection, "", "email", 0, 0)
	if err != nil {
		t.Fatalf("find recipients: %v", err)
	}
	var emails []string
	for _, row := range rows {
		emails = append(emails, row.GetString("email"))
	}
	if got := strings.Join(emails, ","); got != "active@example.com,admin@example.com" {
		t.Fatalf("recipients = %s, want only active and verified users", got)
	}
}

func TestCreateEmailSendoutRequiresSMTP(t *testing.T) {
	h, notifier, admin := newSendoutTestHandlers(t)
	h.app.Settings().SMTP.Enabled = false

	e, _ := sendoutEvent(t, h.app, http.MethodPost, "/api/panel/email-sendouts", "", validSendoutRequest(), admin)
	err := h.createEmailSendout(e)
	if err == nil || sendoutAPIStatus(t, err) != http.StatusServiceUnavailable {
		t.Fatalf("createEmailSendout() error = %v, want 503", err)
	}
	if n, _ := h.app.CountRecords(sendouts.SendoutsCollection); n != 0 || notifier.calls != 0 {
		t.Fatalf("stored %d sendouts and notified %d times, want none", n, notifier.calls)
	}
}

func TestCreateEmailSendoutRejectsIneligibleSingleRecipient(t *testing.T) {
	h, _, admin := newSendoutTestHandlers(t)
	disabled := newSendoutTestUser(t, h.app, "disabled@example.com", "disabled", true)
	in := validSendoutRequest()
	in.Audience, in.UserID = "single", disabled.Id

	e, _ := sendoutEvent(t, h.app, http.MethodPost, "/api/panel/email-sendouts", "", in, admin)
	if err := h.createEmailSendout(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
		t.Fatalf("createEmailSendout() error = %v, want 400", err)
	}
}

func TestEmailSendoutComposerEndpoints(t *testing.T) {
	h, _, admin := newSendoutTestHandlers(t)
	newSendoutTestUser(t, h.app, "ops@example.com", "active", true)
	newSendoutTestUser(t, h.app, "off@example.com", "disabled", true)

	e, response := sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts/context", "", nil, admin)
	if err := h.emailSendoutContext(e); err != nil {
		t.Fatalf("emailSendoutContext() error = %v", err)
	}
	var context EmailSendoutContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &context); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if context.EligibleRecipientCount != 2 || !context.SMTPEnabled || context.RatePerMinute != 30 || context.AppName != h.app.Settings().Meta.AppName {
		t.Fatalf("context = %+v", context)
	}

	e, response = sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts/eligible-recipients?search=ops", "", nil, admin)
	if err := h.listEmailSendoutCandidates(e); err != nil {
		t.Fatalf("listEmailSendoutCandidates() error = %v", err)
	}
	var candidates []EmailSendoutRecipientCandidate
	if err := json.Unmarshal(response.Body.Bytes(), &candidates); err != nil || len(candidates) != 1 || candidates[0].Email != "ops@example.com" {
		t.Fatalf("candidates = %+v (%v), want only ops@example.com", candidates, err)
	}
}
