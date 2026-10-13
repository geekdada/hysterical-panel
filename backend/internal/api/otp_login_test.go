package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func triggerOTPRequest(t *testing.T, app core.App, record *core.Record) (*core.Record, bool, error) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	event := &core.RecordCreateOTPRequestEvent{
		RequestEvent: &core.RequestEvent{App: app},
		Record:       record,
	}
	event.Collection = users
	var submitted *core.Record
	reached := false
	err = app.OnRecordRequestOTPRequest().Trigger(event, func(e *core.RecordCreateOTPRequestEvent) error {
		reached = true
		submitted = e.Record
		return nil
	})
	return submitted, reached, err
}

func TestOTPRequestRequiresSMTP(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	h.bindOTPLoginGate()
	app.Settings().SMTP.Enabled = false
	user := newUsersTestRecord(t, app, "otp-nosmtp@example.com", "OtpNoSmtpSecret1234")

	_, reached, err := triggerOTPRequest(t, app, user)
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("OTP request without SMTP err = %v, want 503", err)
	}
	if reached {
		t.Fatal("OTP request without SMTP reached the send step")
	}
}

func TestOTPRequestSendsOnlyToActiveUsers(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	h.bindOTPLoginGate()
	app.Settings().SMTP.Enabled = true

	active := newUsersTestRecord(t, app, "otp-active@example.com", "OtpActiveSecret1234")
	submitted, reached, err := triggerOTPRequest(t, app, active)
	if err != nil || !reached || submitted == nil || submitted.Id != active.Id {
		t.Fatalf("active user err = %v reached = %v record = %v", err, reached, submitted)
	}

	unverified := newUsersTestRecord(t, app, "otp-unverified@example.com", "OtpUnverifiedSecret1234")
	unverified.Set("verified", false)
	submitted, reached, err = triggerOTPRequest(t, app, unverified)
	if err != nil || !reached || submitted == nil || submitted.Id != unverified.Id {
		t.Fatalf("unverified user err = %v reached = %v record = %v", err, reached, submitted)
	}

	disabled := newUsersTestRecord(t, app, "otp-disabled@example.com", "OtpDisabledSecret1234")
	disabled.Set("status", "disabled")
	submitted, reached, err = triggerOTPRequest(t, app, disabled)
	if err != nil || !reached {
		t.Fatalf("disabled user err = %v reached = %v", err, reached)
	}
	if submitted != nil {
		t.Fatalf("disabled user record = %v, want nil so no code is sent", submitted.Id)
	}
}
