package api

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/hysteria"
)

// GET /nodes/:id/live
// On-demand diagnostics for a single node: pulls /dump/streams and /online once,
// groups every stream by the panel user that owns its auth string (no auth
// filter, unlike the user-scoped endpoint). Never cached, never persisted.
func (h *Handlers) nodeLive(e *core.RequestEvent) error {
	n, err := h.app.FindRecordById("nodes", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("node not found", err)
	}

	secret, derr := h.box.Decrypt(n.GetString("api_secret"))
	if derr != nil {
		return ok(e, nodeLiveError("decrypt secret failed"))
	}

	cl := hysteria.New(n.GetString("api_url"), secret, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streams, serr := cl.DumpStreams(ctx)
	if serr != nil {
		return ok(e, nodeLiveError(serr.Error()))
	}
	online, _ := cl.Online(ctx) // best-effort; a nil map reads back as zero

	// Resolve auth_string -> panel user once, so each stream can be attributed.
	users, err := h.app.FindRecordsByFilter("users", "", "", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list users", err)
	}
	userByAuth := make(map[string]map[string]any, len(users))
	for _, u := range users {
		userByAuth[u.GetString("auth_string")] = map[string]any{
			"id":    u.Id,
			"email": u.GetString("email"),
		}
	}

	now := time.Now().UTC()
	agg := newLiveAggregator()

	type userGroup struct {
		ref     map[string]any
		streams []map[string]any
	}
	groups := map[string]*userGroup{}
	order := []string{} // preserve first-seen auth order for stable output

	for _, s := range streams {
		g := groups[s.Auth]
		if g == nil {
			ref := userByAuth[s.Auth]
			if ref == nil {
				ref = map[string]any{"id": "", "email": "unknown"}
			}
			g = &userGroup{ref: ref}
			groups[s.Auth] = g
			order = append(order, s.Auth)
		}
		g.streams = append(g.streams, agg.add(s, now))
	}

	byUser := make([]map[string]any, 0, len(order))
	for _, auth := range order {
		g := groups[auth]
		byUser = append(byUser, map[string]any{
			"user":           g.ref,
			"online_devices": online[auth],
			"streams":        g.streams,
		})
	}

	// True node-wide online count: every device the node reports, including users
	// that are connected but have no active stream right now.
	totalOnline := 0
	for _, c := range online {
		totalOnline += c
	}

	return ok(e, map[string]any{
		"online_devices": totalOnline,
		"active_streams": len(streams),
		"by_user":        byUser,
		"top_domains":    agg.topDomains(),
		"by_connection":  agg.byConnection(),
	})
}

// nodeLiveError is the empty diagnostics envelope returned when a node can't be
// reached, so the frontend renders a normal "error" state instead of a failure.
func nodeLiveError(msg string) map[string]any {
	return map[string]any{
		"online_devices": 0,
		"active_streams": 0,
		"by_user":        []any{},
		"top_domains":    []any{},
		"by_connection":  []any{},
		"error":          msg,
	}
}
