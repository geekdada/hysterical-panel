// Package api registers the panel's custom HTTP routes under /api/panel.
// All routes require an authenticated admin user. Node API secrets are never
// returned to clients.
package api

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"hysterical-panel/internal/cryptobox"
)

// Handlers bundles dependencies shared by all route handlers.
type Handlers struct {
	app core.App
	box *cryptobox.Box
}

// Register wires every /api/panel/* route onto the serve event router.
func Register(se *core.ServeEvent, app core.App, box *cryptobox.Box) {
	h := &Handlers{app: app, box: box}

	bindAuthGate(app)

	g := se.Router.Group("/api/panel")
	g.Bind(apis.RequireAuth("users")) // must be a logged-in users-collection record
	g.Bind(requireAdmin())

	// nodes
	g.GET("/nodes", h.listNodes)
	g.POST("/nodes", h.createNode)
	g.GET("/nodes/{id}", h.getNode)
	g.PATCH("/nodes/{id}", h.updateNode)
	g.DELETE("/nodes/{id}", h.deleteNode)
	g.POST("/nodes/{id}/test", h.testNode)

	// node-scoped traffic + live (node-wide, across all users)
	g.GET("/nodes/{id}/traffic/summary", h.nodeTrafficSummary)
	g.GET("/nodes/{id}/traffic/series", h.nodeTrafficSeries)
	g.GET("/nodes/{id}/live", h.nodeLive)

	// users
	g.GET("/users", h.listUsers)
	g.POST("/users", h.createUser)
	g.GET("/users/{id}", h.getUser)
	g.PATCH("/users/{id}", h.updateUser)
	g.DELETE("/users/{id}", h.deleteUser)

	// traffic + live
	g.GET("/users/{id}/traffic/summary", h.trafficSummary)
	g.GET("/users/{id}/traffic/series", h.trafficSeries)
	g.GET("/users/{id}/live", h.userLive)

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

// bindAuthGate blocks authentication for any user whose status is not "active".
// It fires on every auth response (password, refresh, OAuth2, OTP), so a
// disabled user can neither sign in nor keep refreshing an existing token.
func bindAuthGate(app core.App) {
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if e.Record == nil || e.Record.GetString("status") != "active" {
			return apis.NewForbiddenError("account is disabled", nil)
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
		"id":             n.Id,
		"name":           n.GetString("name"),
		"api_url":        n.GetString("api_url"),
		"poll_interval":  n.GetInt("poll_interval"),
		"enabled":        n.GetBool("enabled"),
		"last_polled_at": n.GetString("last_polled_at"),
		"last_error":     n.GetString("last_error"),
		"health":         health,
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
