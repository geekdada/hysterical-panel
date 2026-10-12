package api

import (
	"errors"
	"log"
	"net/url"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/oidc"
)

func (h *Handlers) getOIDCAuthorizationRequest(e *core.RequestEvent) error {
	now := time.Now()
	p, err := h.pendingOIDCRequest(e, now)
	if err != nil {
		return err
	}
	needsConsent, err := p.NeedsConsent(h.app, e.Auth.Id)
	if err != nil {
		return apis.NewInternalServerError("failed to check consent", err)
	}
	return ok(e, OIDCAuthorizationRequest{
		ClientName:               p.Client.GetString("name"),
		RedirectOrigin:           redirectOrigin(p.RedirectURI()),
		Scopes:                   p.Scopes(),
		ConsentRequired:          needsConsent,
		ReauthenticationRequired: p.NeedsReauthentication(h.oidcSession(e, now), now),
		PromptNone:               p.PromptNone(),
		ExpiresAt:                p.ExpiresAt().UTC().Format(types.DefaultDateLayout),
	})
}

func (h *Handlers) pendingOIDCRequest(e *core.RequestEvent, now time.Time) (*oidc.PendingRequest, error) {
	p, err := oidc.FindPendingRequest(h.app, e.Request.PathValue("handle"), now)
	if err != nil {
		return nil, oidcDecisionError(err)
	}
	if !oidc.UserMayUse(p.Client, e.Auth) {
		return nil, oidcDecisionError(oidc.ErrUserNotAllowed)
	}
	return p, nil
}

func (h *Handlers) approveOIDCAuthorizationRequest(e *core.RequestEvent) error {
	now := time.Now()
	redirect, err := oidc.Approve(h.app, e.Request.PathValue("handle"), e.Auth, h.oidcSession(e, now), oidcIssuer(e), now)
	return oidcDecision(e, redirect, err)
}

func (h *Handlers) denyOIDCAuthorizationRequest(e *core.RequestEvent) error {
	redirect, err := oidc.Deny(h.app, e.Request.PathValue("handle"), oidcIssuer(e), time.Now())
	return oidcDecision(e, redirect, err)
}

// abortOIDCAuthorizationRequest is public, because the User is not signed in
// to the panel. The request handle is the only credential.
func (h *Handlers) abortOIDCAuthorizationRequest(e *core.RequestEvent) error {
	now := time.Now()
	if !h.oidcLimits.authorize.allow(e.RealIP(), now) {
		return apis.NewTooManyRequestsError("too many requests", nil)
	}
	redirect, err := oidc.Abort(h.app, e.Request.PathValue("handle"), oidcIssuer(e), now)
	return oidcDecision(e, redirect, err)
}

func oidcDecision(e *core.RequestEvent, redirect string, err error) error {
	if err != nil {
		return oidcDecisionError(err)
	}
	return ok(e, OIDCAuthorizationDecision{RedirectTo: redirect})
}

func oidcDecisionError(err error) error {
	switch {
	case errors.Is(err, oidc.ErrRequestNotFound):
		return apis.NewNotFoundError("authorization request not found or expired", nil)
	case errors.Is(err, oidc.ErrReauthenticationRequired):
		return apis.NewBadRequestError("reauthentication required", nil)
	case errors.Is(err, oidc.ErrUserNotAllowed):
		return apis.NewForbiddenError("account cannot sign in to connected apps", nil)
	}
	return apis.NewInternalServerError("failed to complete the authorization request", err)
}

// oidcSession reads when the calling panel session signed in.
func (h *Handlers) oidcSession(e *core.RequestEvent, now time.Time) oidc.Session {
	authTime, known, err := oidc.SessionAuthTime(h.app, bearerToken(e.Request), now)
	if err != nil {
		log.Printf("[oidc] read session auth time: %v", err)
	}
	return oidc.Session{AuthTime: authTime, Known: known}
}

func redirectOrigin(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
