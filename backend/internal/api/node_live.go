package api

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/hysteria"
)

// GET /nodes/:id/live
// On-demand diagnostics for a single node: pulls /dump/streams once,
// groups every stream by the panel user that owns its Node Client ID (no auth
// filter, unlike the user-scoped endpoint). Never cached, never persisted.
func (h *Handlers) nodeLive(e *core.RequestEvent) error {
	n, err := h.findActiveNode(e.Request.PathValue("id"))
	if err != nil {
		return err
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
	resolver, err := authstrings.LoadResolver(h.app)
	if err != nil {
		return apis.NewBadRequestError("failed to load auth strings", err)
	}

	now := time.Now().UTC()
	agg := newLiveAggregator(h.ipLookup)

	type userGroup struct {
		ref     map[string]any
		streams []map[string]any
	}
	groups := map[string]*userGroup{}
	order := []string{} // preserve first-seen canonical client ID order

	for _, s := range streams {
		groupID := s.Auth
		var ref map[string]any
		if user := resolver.Resolve(s.Auth); user != nil {
			groupID = user.Id
			ref = map[string]any{"id": user.Id, "email": user.GetString("email")}
		}
		g := groups[groupID]
		if g == nil {
			if ref == nil {
				ref = map[string]any{"id": "", "email": "unknown"}
			}
			g = &userGroup{ref: ref}
			groups[groupID] = g
			order = append(order, groupID)
		}
		g.streams = append(g.streams, agg.add(s, now))
	}

	byUser := make([]map[string]any, 0, len(order))
	for _, groupID := range order {
		g := groups[groupID]
		byUser = append(byUser, map[string]any{
			"user":    g.ref,
			"streams": g.streams,
		})
	}

	return ok(e, map[string]any{
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
		"active_streams": 0,
		"by_user":        []any{},
		"top_domains":    []any{},
		"by_connection":  []any{},
		"error":          msg,
	}
}
