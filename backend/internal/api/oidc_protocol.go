package api

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
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
	g.POST(oidc.TokenPath, h.oidcToken)
	g.GET(oidc.UserInfoPath, h.oidcUserInfo)
	g.POST(oidc.UserInfoPath, h.oidcUserInfo)
	g.POST(oidc.RevocationPath, h.oidcRevoke)
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
	consentPage := h.publicConfig.FrontendURL + "/oidc/authorize?request=" + url.QueryEscape(handle)
	if slices.Contains(req.Prompt, oidc.PromptNone) {
		// The page cannot read the request before the visitor signs in, and a
		// prompt=none request must end at once instead of showing the sign-in
		// form. The abort endpoint still decides the error from the stored
		// request, so a forged hint changes nothing the app receives.
		consentPage += "&prompt=none"
	}
	return e.Redirect(http.StatusSeeOther, consentPage)
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

// oauthErrorBody is the error response of RFC 6749 5.2.
type oauthErrorBody struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func (h *Handlers) oidcToken(e *core.RequestEvent) error {
	setNoStore(e)
	client, oauthErr := h.oidcAuthenticatedClient(e)
	if oauthErr != nil {
		return writeOAuthError(e, oauthErr)
	}
	req := oidc.TokenRequest{Client: client, Form: e.Request.PostForm, Issuer: oidcIssuer(e), Box: h.box, Now: time.Now()}
	var resp *oidc.TokenResponse
	switch e.Request.PostForm.Get("grant_type") {
	case oidc.GrantTypeAuthorizationCode:
		resp, oauthErr = oidc.ExchangeCode(h.app, req)
	case oidc.GrantTypeRefreshToken:
		resp, oauthErr = oidc.RefreshTokens(h.app, req)
	case "":
		oauthErr = oidc.NewError(oidc.InvalidRequest, "grant_type is required")
	default:
		oauthErr = oidc.NewError(oidc.UnsupportedGrantType, "use authorization_code or refresh_token")
	}
	if oauthErr != nil {
		return writeOAuthError(e, oauthErr)
	}
	return e.JSON(http.StatusOK, resp)
}

// oidcAuthenticatedClient parses a back-channel request and authenticates
// the app. Failures past the per-address budget answer 429, but the right
// secret always passes. Behind a proxy that PocketBase does not trust, every
// caller shares one address, and a hard lockout would let anyone shut out
// every app. Secrets are 256-bit, so the lockout bought no brute-force
// protection anyway. Query parameters are ignored, so credentials in a URL
// never count.
func (h *Handlers) oidcAuthenticatedClient(e *core.RequestEvent) (*core.Record, *oidc.Error) {
	ip, now := e.RealIP(), time.Now()
	if !h.oidcLimits.backchannel.allow(ip, now) {
		return nil, oidc.NewError(oidc.TemporarilyUnavailable, "too many requests")
	}
	if err := e.Request.ParseForm(); err != nil || oidc.HasRepeatedParams(e.Request.PostForm) {
		return nil, oidc.NewError(oidc.InvalidRequest, "send each parameter once in a form-encoded body")
	}
	client, oauthErr := oidc.AuthenticateClient(h.app, e.Request)
	if oauthErr != nil && oauthErr.Code == oidc.InvalidClient && !h.oidcLimits.clientFailures.allow(ip, now) {
		return nil, oidc.NewError(oidc.TemporarilyUnavailable, "too many failed client authentications")
	}
	return client, oauthErr
}

// writeOAuthError answers with RFC 6749 5.2 statuses. invalid_client always
// carries a Basic challenge, which keeps the 401 valid HTTP whichever method
// the client used.
func writeOAuthError(e *core.RequestEvent, oauthErr *oidc.Error) error {
	status := http.StatusBadRequest
	switch oauthErr.Code {
	case oidc.InvalidClient:
		status = http.StatusUnauthorized
		e.Response.Header().Set("WWW-Authenticate", `Basic realm="oidc"`)
	case oidc.TemporarilyUnavailable:
		status = http.StatusTooManyRequests
		e.Response.Header().Set("Retry-After", strconv.Itoa(int(oidcRateWindow.Seconds())))
	case oidc.ServerError:
		status = http.StatusInternalServerError
		log.Printf("[oidc] %s %s: %v", e.Request.Method, e.Request.URL.Path, oauthErr)
	}
	return e.JSON(status, oauthErrorBody{Error: oauthErr.Code, Description: oauthErr.Description})
}

// setNoStore keeps tokens and claims out of caches (RFC 6749 5.1).
func setNoStore(e *core.RequestEvent) {
	e.Response.Header().Set("Cache-Control", "no-store")
	e.Response.Header().Set("Pragma", "no-cache")
}

// oidcUserInfo returns the claims the access token's scope releases
// (OIDC Core 5.3). Errors follow RFC 6750 3.
func (h *Handlers) oidcUserInfo(e *core.RequestEvent) error {
	setNoStore(e)
	now := time.Now()
	if !h.oidcLimits.backchannel.allow(e.RealIP(), now) {
		return writeOAuthError(e, oidc.NewError(oidc.TemporarilyUnavailable, "too many requests"))
	}
	raw, found := oidcBearerToken(e.Request)
	if !found {
		e.Response.Header().Set("WWW-Authenticate", `Bearer realm="oidc"`)
		return e.NoContent(http.StatusUnauthorized)
	}
	user, scope, err := oidc.LookupAccessToken(h.app, raw, now)
	if errors.Is(err, oidc.ErrInvalidToken) {
		e.Response.Header().Set("WWW-Authenticate", `Bearer realm="oidc", error="invalid_token"`)
		return e.NoContent(http.StatusUnauthorized)
	}
	if err != nil {
		return writeOAuthError(e, oidc.WrapServerError(err))
	}
	return e.JSON(http.StatusOK, oidc.UserClaims(user, scope))
}

// oidcRevoke implements RFC 7009. It answers 200 for unknown tokens too.
func (h *Handlers) oidcRevoke(e *core.RequestEvent) error {
	setNoStore(e)
	client, oauthErr := h.oidcAuthenticatedClient(e)
	if oauthErr != nil {
		return writeOAuthError(e, oauthErr)
	}
	raw := e.Request.PostForm.Get("token")
	if raw == "" {
		return writeOAuthError(e, oidc.NewError(oidc.InvalidRequest, "token is required"))
	}
	if err := oidc.RevokeToken(h.app, client, raw, time.Now()); err != nil {
		return writeOAuthError(e, oidc.WrapServerError(err))
	}
	return e.NoContent(http.StatusOK)
}

// oidcBearerToken reads an RFC 6750 bearer token from the Authorization
// header. A panel session without the Bearer scheme does not count.
func oidcBearerToken(r *http.Request) (string, bool) {
	scheme, raw, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return "", false
	}
	return raw, true
}
