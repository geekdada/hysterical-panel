// Package api registers the panel's custom HTTP routes under /api/panel.
// Routes require a logged-in users record; mutating/admin-wide routes additionally
// require the admin role. Node API secrets are never returned to clients.
package api

import (
	"net/http"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/ipmeta"
)

type ipMetadataLookup interface {
	LookupHost(host string) *ipmeta.Info
}

// Handlers bundles dependencies shared by all route handlers.
type Handlers struct {
	app          core.App
	box          *cryptobox.Box
	ipLookup     ipMetadataLookup
	passkeys     *webauthn.WebAuthn
	passkeyLimit *passkeyRateLimiter
	publicConfig PanelConfigResponse
}

// Register wires every /api/panel/* route onto the serve event router.
func Register(se *core.ServeEvent, app core.App, box *cryptobox.Box, ipLookup ipMetadataLookup, passkeys *webauthn.WebAuthn, public PanelConfigResponse) {
	h := &Handlers{
		app:          app,
		box:          box,
		ipLookup:     ipLookup,
		passkeys:     passkeys,
		passkeyLimit: newPasskeyRateLimiter(30, passkeyChallengeTTL),
		publicConfig: public,
	}

	h.bindAuthGate()

	g := se.Router.Group("/api/panel")
	g.Bind(apis.RequireAuth("users")) // must be a logged-in users-collection record
	adminOnly := requireAdmin()
	adminOrSelf := requireAdminOrSelf()
	activeAdminOrSelf := requireActiveAdminOrSelf()
	activeSelf := requireActiveSelf()

	// dashboard traffic
	g.GET("/traffic", h.panelTraffic).Bind(adminOnly)
	g.GET("/traffic/series", h.panelTrafficSeries).Bind(adminOnly)
	g.GET("/nodes/traffic/summary", h.panelNodeTrafficSummary).Bind(adminOnly)

	// database management
	g.GET("/database/stats", h.databaseStats).Bind(adminOnly)
	g.POST("/database/prune", h.pruneDatabaseTraffic).Bind(adminOnly)

	// nodes
	g.GET("/nodes", h.listNodes).Bind(adminOnly)
	g.POST("/nodes", h.createNode).Bind(adminOnly)
	g.GET("/nodes/{id}", h.getNode).Bind(adminOnly)
	g.PATCH("/nodes/{id}", h.updateNode).Bind(adminOnly)
	g.DELETE("/nodes/{id}", h.deleteNode).Bind(adminOnly)
	g.POST("/nodes/{id}/test", h.testNode).Bind(adminOnly)

	// node-scoped traffic + live (node-wide, across all users)
	g.GET("/nodes/{id}/traffic/summary", h.nodeTrafficSummary).Bind(adminOnly)
	g.GET("/nodes/{id}/traffic/series", h.nodeTrafficSeries).Bind(adminOnly)
	g.GET("/nodes/{id}/live", h.nodeLive).Bind(adminOnly)

	// users
	g.GET("/users", h.listUsers).Bind(adminOnly)
	g.POST("/users", h.createUser).Bind(adminOnly)
	g.GET("/users/{id}", h.getUser).Bind(adminOrSelf)
	g.PATCH("/users/{id}", h.updateUser).Bind(adminOnly)
	g.DELETE("/users/{id}", h.deleteUser).Bind(adminOnly)
	g.GET("/users/{id}/passkeys", h.listPasskeys).Bind(activeAdminOrSelf)
	g.POST("/users/{id}/passkeys/registration/options", h.passkeyRegistrationOptions).Bind(activeSelf)
	g.POST("/users/{id}/passkeys/registration/finish", h.passkeyRegistrationFinish).Bind(activeSelf)
	g.DELETE("/users/{id}/passkeys/{passkeyId}", h.deletePasskey).Bind(activeAdminOrSelf)

	// traffic + live
	g.GET("/users/{id}/traffic/summary", h.trafficSummary).Bind(adminOrSelf)
	g.GET("/users/{id}/traffic/series", h.trafficSeries).Bind(adminOrSelf)
	g.GET("/users/{id}/live", h.userLive).Bind(adminOnly)

	// Public panel config — no auth required
	se.Router.GET("/api/panel/config", h.handlePanelConfig)

	// Public passkey login endpoints — no auth required.
	se.Router.POST("/api/panel/passkeys/login/options", h.passkeyLoginOptions)
	se.Router.POST("/api/panel/passkeys/login/finish", h.passkeyLoginFinish)

	// openapi schema — no auth required (contains no secrets)
	se.Router.GET("/api/openapi.json", handleOpenAPISpec)

	// Public: Hysteria 2 servers POST here to authenticate clients.
	// Kept out of openapi.go on purpose — the spec drives the panel's
	// TypeScript client, but this endpoint is called by Hysteria servers,
	// not the panel UI.
	se.Router.POST("/api/hysteria/auth", h.hysteriaAuth)
}

// requireAdmin rejects any authenticated user whose role is not "admin".
func requireAdmin() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			if e.Auth == nil || e.Auth.GetString("role") != "admin" {
				return apis.NewForbiddenError("admin role required", nil)
			}
			return e.Next()
		},
	}
}

// requireAdminOrSelf allows admins to view any record, and regular users to
// view only the user id carried in the route path.
func requireAdminOrSelf() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			if e.Auth == nil {
				return apis.NewForbiddenError("user access required", nil)
			}
			if e.Auth.GetString("role") == "admin" || e.Auth.Id == e.Request.PathValue("id") {
				return e.Next()
			}
			return apis.NewForbiddenError("admin or self access required", nil)
		},
	}
}

func requireActiveAdminOrSelf() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			if e.Auth == nil || e.Auth.GetString("status") != "active" {
				return apis.NewForbiddenError("active account required", nil)
			}
			if e.Auth.GetString("role") == "admin" || e.Auth.Id == e.Request.PathValue("id") {
				return e.Next()
			}
			return apis.NewForbiddenError("admin or self access required", nil)
		},
	}
}

func requireActiveSelf() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			if e.Auth == nil || e.Auth.GetString("status") != "active" {
				return apis.NewForbiddenError("active account required", nil)
			}
			if e.Auth.Id != e.Request.PathValue("id") {
				return apis.NewForbiddenError("self access required", nil)
			}
			return e.Next()
		},
	}
}

// bindAuthGate blocks authentication for any user whose status is not "active".
// It fires on every auth response (password, refresh, OAuth2, OTP), so a
// disabled user can neither sign in nor keep refreshing an existing token.
func (h *Handlers) bindAuthGate() {
	h.app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if e.Record == nil || e.Record.GetString("status") != "active" {
			return apis.NewForbiddenError("account is disabled", nil)
		}
		if err := h.rejectPasswordForPasskeyUser(e); err != nil {
			return err
		}
		return e.Next()
	})
}

// nodesForUser is the single choke point for node visibility. Today every
// enabled node is visible to every user; user-group filtering can be added here
// later without touching callers.
func (h *Handlers) nodesForUser(_ string) ([]*core.Record, error) {
	return h.app.FindRecordsByFilter("nodes", "enabled = true", "", 0, 0)
}

// publicNode strips the secret and derives a health field.
func publicNode(n *core.Record) map[string]any {
	health := "never"
	if n.GetString("last_error") != "" {
		health = "error"
	} else if !n.GetDateTime("last_polled_at").IsZero() {
		health = "ok"
	}
	return map[string]any{
		"id":               n.Id,
		"name":             n.GetString("name"),
		"api_url":          n.GetString("api_url"),
		"poll_interval":    n.GetInt("poll_interval"),
		"enabled":          n.GetBool("enabled"),
		"last_polled_at":   n.GetString("last_polled_at"),
		"last_error":       n.GetString("last_error"),
		"health":           health,
		"current_tx_speed": n.GetInt("current_tx_speed"),
		"current_rx_speed": n.GetInt("current_rx_speed"),
	}
}

func publicUser(u *core.Record) map[string]any {
	return map[string]any{
		"id":          u.Id,
		"email":       u.GetString("email"),
		"role":        u.GetString("role"),
		"auth_string": u.GetString("auth_string"),
		"quota_bytes": u.GetInt("quota_bytes"),
		"used_tx":     u.GetInt("used_tx"),
		"used_rx":     u.GetInt("used_rx"),
		"status":      u.GetString("status"),
	}
}

func ok(e *core.RequestEvent, data any) error {
	return e.JSON(http.StatusOK, data)
}
