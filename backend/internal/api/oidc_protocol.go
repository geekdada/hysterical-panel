package api

import (
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"hysterical-panel/internal/oidc"
)

// oidcIssuerKey stores the serving issuer on the request event.
const oidcIssuerKey = "oidcIssuer"

// bindOIDCRoutes mounts the provider endpoints (ADR-0010). Relying parties
// call them, not the panel UI, so they stay out of openapi.go.
func (h *Handlers) bindOIDCRoutes(se *core.ServeEvent) {
	g := se.Router.Group(oidc.IssuerPath)
	g.Bind(h.requireOIDCServing())
	g.GET(oidc.DiscoveryPath, h.oidcDiscovery)
	g.GET(oidc.JWKSPath, h.oidcJWKS)
	g.GET(oidc.AuthorizePath, h.oidcAuthorize)
	g.POST(oidc.AuthorizePath, h.oidcAuthorize)
}

// requireOIDCServing hides the provider behind 404 while it is off or
// misconfigured, like the Management API.
func (h *Handlers) requireOIDCServing() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			issuer, ok := h.oidcServingIssuer()
			if !ok {
				return apis.NewNotFoundError("", nil)
			}
			e.Set(oidcIssuerKey, issuer)
			return e.Next()
		},
	}
}

func oidcIssuer(e *core.RequestEvent) string {
	issuer, _ := e.Get(oidcIssuerKey).(string)
	return issuer
}

func (h *Handlers) oidcDiscovery(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "public, max-age=300")
	return e.JSON(http.StatusOK, oidc.Discovery(oidcIssuer(e)))
}

// oidcJWKS creates the signing key first, so a switch set outside the panel
// never publishes an empty key set.
func (h *Handlers) oidcJWKS(e *core.RequestEvent) error {
	now := time.Now()
	if _, err := oidc.ActiveSigningKey(h.app, h.box, now); err != nil {
		return apis.NewInternalServerError("failed to prepare the oidc signing key", err)
	}
	jwks, err := oidc.PublicJWKS(h.app, now)
	if err != nil {
		return apis.NewInternalServerError("failed to load the oidc signing keys", err)
	}
	e.Response.Header().Set("Cache-Control", "public, max-age=300")
	return e.JSON(http.StatusOK, jwks)
}

// Per-IP limits. A relying party backend sends every user's token requests
// from one address, so the back-channel limit is generous and failed client
// authentication has a much smaller budget of its own.
const (
	oidcAuthorizeRateMax   = 60
	oidcBackchannelRateMax = 300
	oidcClientFailureMax   = 10
	oidcRateWindow         = time.Minute

	oidcPageRateLimited = "rate_limited"
)

type oidcLimits struct {
	authorize      *passkeyRateLimiter
	backchannel    *passkeyRateLimiter
	clientFailures *passkeyRateLimiter
}

func newOIDCLimits() oidcLimits {
	return oidcLimits{
		authorize:      newPasskeyRateLimiter(oidcAuthorizeRateMax, oidcRateWindow),
		backchannel:    newPasskeyRateLimiter(oidcBackchannelRateMax, oidcRateWindow),
		clientFailures: newPasskeyRateLimiter(oidcClientFailureMax, oidcRateWindow),
	}
}

// oidcAuthorize validates the request, stores it and hands the browser to the
// panel's consent page. Every redirect is a 303 (RFC 9700 4.12).
func (h *Handlers) oidcAuthorize(e *core.RequestEvent) error {
	now := time.Now()
	if !h.oidcLimits.authorize.allow(e.RealIP(), now) {
		return h.oidcErrorPage(e, oidcPageRateLimited)
	}
	if err := e.Request.ParseForm(); err != nil {
		return h.oidcErrorPage(e, oidc.PageInvalidRequest)
	}
	req, failure := oidc.ParseAuthorizeRequest(h.app, e.Request.Form)
	if failure != nil {
		return h.oidcAuthorizeFailure(e, failure)
	}
	handle, err := oidc.SaveAuthRequest(h.app, req, now)
	if err != nil {
		log.Printf("[oidc] save authorization request: %v", err)
		return h.oidcAuthorizeFailure(e, &oidc.AuthorizeFailure{
			Err: oidc.NewError(oidc.ServerError, ""), RedirectURI: req.RedirectURI, State: req.State,
		})
	}
	return e.Redirect(http.StatusSeeOther, h.publicConfig.FrontendURL+"/oidc/authorize?request="+url.QueryEscape(handle))
}

func (h *Handlers) oidcAuthorizeFailure(e *core.RequestEvent, failure *oidc.AuthorizeFailure) error {
	if failure.Err == nil {
		return h.oidcErrorPage(e, failure.Page)
	}
	return e.Redirect(http.StatusSeeOther, oidc.ErrorRedirect(failure.RedirectURI, failure.Err, failure.State, oidcIssuer(e)))
}

func (h *Handlers) oidcErrorPage(e *core.RequestEvent, reason string) error {
	return e.Redirect(http.StatusSeeOther, h.publicConfig.FrontendURL+"/oidc/error?reason="+url.QueryEscape(reason))
}
