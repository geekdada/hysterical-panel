package api

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// bindOTPLoginGate guards PocketBase's email code (OTP) login, which the admin
// enables on the users collection in /_/. Without SMTP the sendmail fallback
// drops the code without an error, so the request returns 503 instead. A disabled
// User gets the same dummy response as an unknown email: no code is sent, and
// the response does not reveal that the account exists. Unverified Users still
// receive a code because PocketBase marks the email verified once it is used.
func (h *Handlers) bindOTPLoginGate() {
	h.app.OnRecordRequestOTPRequest("users").BindFunc(func(e *core.RecordCreateOTPRequestEvent) error {
		if !h.smtpEnabled() {
			return apis.NewApiError(http.StatusServiceUnavailable, "email sign-in is unavailable: SMTP is not configured", nil)
		}
		if e.Record != nil && e.Record.GetString("status") != "active" {
			e.Record = nil
		}
		return e.Next()
	})
}
