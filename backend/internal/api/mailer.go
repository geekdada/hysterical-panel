package api

import (
	"fmt"
	"html"
	"net/mail"
	"net/url"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"

	"hysterical-panel/internal/sendouts"
)

// smtpEnabled reports whether PocketBase has SMTP configured. When false the
// local sendmail fallback would silently fail in most deployments, so callers
// degrade gracefully (return a copyable link, or reject the verification path).
func (h *Handlers) smtpEnabled() bool {
	return h.app.Settings().SMTP.Enabled
}

func (h *Handlers) sender() mail.Address {
	return sendouts.Sender(h.app)
}

// inviteLink builds the public registration link carrying an invite code. When
// no frontend origin is configured it returns a relative path so the admin can
// still copy and prefix it manually.
func (h *Handlers) inviteLink(code string) string {
	return h.publicConfig.FrontendURL + "/register?code=" + url.QueryEscape(code)
}

// verifyLink builds the public email-verification link carrying a PocketBase
// verification token consumed by the frontend /verify page.
func (h *Handlers) verifyLink(token string) string {
	return h.publicConfig.FrontendURL + "/verify?token=" + url.QueryEscape(token)
}

// sendInviteEmail delivers an invite link to the recipient. It reports whether
// an email was actually sent (false when SMTP is not configured), so the caller
// can surface the link for manual sharing instead.
func (h *Handlers) sendInviteEmail(to, link string) (bool, error) {
	if !h.smtpEnabled() {
		return false, nil
	}
	from := h.sender()
	msg := &mailer.Message{
		From:    from,
		To:      []mail.Address{{Address: to}},
		Subject: fmt.Sprintf("You're invited to %s", from.Name),
		HTML:    fmt.Sprintf(`<p>You've been invited to %s.</p><p><a href="%s">Create your account</a></p><p>Or open this link: %s</p>`, html.EscapeString(from.Name), link, link),
		Text:    fmt.Sprintf("You've been invited to %s.\n\nCreate your account: %s\n", from.Name, link),
	}
	if err := h.app.NewMailClient().Send(msg); err != nil {
		return false, err
	}
	return true, nil
}

// sendVerificationEmail mints a PocketBase verification token for the user and
// emails a link to the frontend /verify page (which calls the built-in
// confirm-verification endpoint). We build the link ourselves rather than using
// PocketBase's template so it always targets the configured frontend origin.
func (h *Handlers) sendVerificationEmail(user *core.Record) error {
	token, err := user.NewVerificationToken()
	if err != nil {
		return err
	}
	link := h.verifyLink(token)
	from := h.sender()
	msg := &mailer.Message{
		From:    from,
		To:      []mail.Address{{Address: user.Email()}},
		Subject: fmt.Sprintf("Verify your %s email", from.Name),
		HTML:    fmt.Sprintf(`<p>Confirm your email to activate your %s account.</p><p><a href="%s">Verify email</a></p><p>Or open this link: %s</p>`, html.EscapeString(from.Name), link, link),
		Text:    fmt.Sprintf("Confirm your email to activate your account: %s\n", link),
	}
	return h.app.NewMailClient().Send(msg)
}
