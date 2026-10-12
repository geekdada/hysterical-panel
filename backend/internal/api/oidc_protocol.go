package api

import (
	"net/http"
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
