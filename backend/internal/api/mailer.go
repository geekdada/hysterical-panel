package api

import (
	"fmt"
	"net/mail"
	"net/url"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

// smtpEnabled reports whether PocketBase has SMTP configured. When false the
// local sendmail fallback would silently fail in most deployments, so callers
// degrade gracefully (return a copyable link, or reject the verification path).
func (h *Handlers) smtpEnabled() bool {
	return h.app.Settings().SMTP.Enabled
}

func (h *Handlers) sender() mail.Address {
	meta := h.app.Settings().Meta
	return mail.Address{Name: meta.SenderName, Address: meta.SenderAddress}
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
	msg := &mailer.Message{
		From:    h.sender(),
		To:      []mail.Address{{Address: to}},
		Subject: "You're invited to Hysterical Panel",
		HTML:    fmt.Sprintf(`<p>You've been invited to Hysterical Panel.</p><p><a href="%s">Create your account</a></p><p>Or open this link: %s</p>`, link, link),
		Text:    fmt.Sprintf("You've been invited to Hysterical Panel.\n\nCreate your account: %s\n", link),
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
	msg := &mailer.Message{
		From:    h.sender(),
		To:      []mail.Address{{Address: user.Email()}},
		Subject: "Verify your Hysterical Panel email",
		HTML:    fmt.Sprintf(`<p>Confirm your email to activate your Hysterical Panel account.</p><p><a href="%s">Verify email</a></p><p>Or open this link: %s</p>`, link, link),
		Text:    fmt.Sprintf("Confirm your email to activate your account: %s\n", link),
	}
	return h.app.NewMailClient().Send(msg)
}
