package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

const (
	registerRateMax    = 10
	registerRateWindow = time.Hour
)

type registerInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

// registrationDecision derives, from the runtime settings, whether self-service
// registration is allowed and whether a valid invite code is required. The
// returned codeRequired also determines email-verification: code-backed signups
// are trusted (verified), the open no-code path must verify their email.
func registrationDecision(s settings) (allowed, codeRequired bool) {
	if s.OpenRegistration {
		return true, s.RequireInviteForOpen
	}
	if s.InvitationsEnabled {
		return true, true // invite-only
	}
	return false, false // closed
}

// generateUniqueAuthString mints a random Hysteria auth key, retrying on the
// (astronomically unlikely) chance of a collision with an existing user.
func (h *Handlers) generateUniqueAuthString() (string, error) {
	for i := 0; i < 8; i++ {
		candidate, err := token.Alphanumeric(16)
		if err != nil {
			return "", err
		}
		existing, _ := h.app.FindFirstRecordByFilter(
			"users",
			"auth_string = {:a}",
			map[string]any{"a": candidate},
		)
		if existing == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique auth_string")
}

// register is the public self-service signup endpoint. Access and the
// email-verification requirement are governed entirely by the runtime settings;
// the client cannot influence role, status or auth_string.
func (h *Handlers) register(e *core.RequestEvent) error {
	if !h.registerLimit.allow(e.RealIP(), time.Now().UTC()) {
		return apis.NewTooManyRequestsError("too many registration attempts", nil)
	}

	var in registerInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	email := strings.TrimSpace(in.Email)
	if email == "" || in.Password == "" {
		return apis.NewBadRequestError("email and password are required", nil)
	}

	allowed, codeRequired := registrationDecision(h.loadSettings())
	if !allowed {
		return apis.NewForbiddenError("registration is disabled", nil)
	}

	var invite *core.Record
	if codeRequired {
		code := strings.TrimSpace(in.Code)
		if code == "" {
			return apis.NewBadRequestError("an invite code is required", nil)
		}
		inv, err := h.app.FindFirstRecordByFilter(
			"invitations",
			"code = {:c}",
			map[string]any{"c": code},
		)
		if err != nil || inv == nil {
			return apis.NewBadRequestError("invalid invite code", nil)
		}
		if ok, reason := inviteValid(inv, time.Now().UTC()); !ok {
			return apis.NewBadRequestError("invite code "+reason, nil)
		}
		invite = inv
	}

	// Code-backed signups are trusted; the open no-code path must verify email.
	verified := codeRequired
	if !verified && !h.smtpEnabled() {
		return apis.NewApiError(http.StatusServiceUnavailable, "registration is unavailable: email verification is not configured", nil)
	}

	coll, err := h.app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	authString, err := h.generateUniqueAuthString()
	if err != nil {
		return apis.NewBadRequestError("failed to provision account", err)
	}

	u := core.NewRecord(coll)
	u.SetEmail(email)
	u.SetPassword(in.Password)
	u.SetVerified(verified)
	u.Set("auth_string", authString)
	u.Set("role", "user")
	u.Set("status", "active")
	u.Set("used_tx", 0)
	u.Set("used_rx", 0)
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError("failed to register (email may be taken)", err)
	}

	if invite != nil {
		invite.Set("used_count", invite.GetInt("used_count")+1)
		invite.Set("last_used_at", time.Now().UTC())
		if err := h.app.Save(invite); err != nil {
			log.Printf("[register] failed to record invite usage: %v", err)
		}
	}

	if !verified {
		if err := h.sendVerificationEmail(u); err != nil {
			// Roll back so no un-verifiable orphan account lingers.
			if delErr := h.app.Delete(u); delErr != nil {
				log.Printf("[register] failed to roll back user after email error: %v", delErr)
			}
			return apis.NewApiError(http.StatusServiceUnavailable, "could not send verification email", nil)
		}
		return ok(e, RegisterResponse{RequiresVerification: true, VerificationSent: true})
	}

	authToken, err := u.NewAuthToken()
	if err != nil {
		return apis.NewBadRequestError("failed to issue auth token", err)
	}
	record := panelUser(u)
	return ok(e, RegisterResponse{Token: authToken, Record: &record})
}
