package api

import (
	"errors"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

// listOIDCGrants returns the Connected Apps a User has authorized.
func (h *Handlers) listOIDCGrants(e *core.RequestEvent) error {
	userID := e.Request.PathValue("id")
	if _, err := h.app.FindRecordById("users", userID); err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	grants, err := oidc.ListActiveGrants(h.app, userID)
	if err != nil {
		return apis.NewInternalServerError("failed to list connected app grants", err)
	}
	out := make([]OIDCGrant, 0, len(grants))
	for _, grant := range grants {
		out = append(out, publicOIDCGrant(grant))
	}
	return ok(e, out)
}

func (h *Handlers) revokeOIDCGrant(e *core.RequestEvent) error {
	err := oidc.RevokeGrant(h.app, e.Request.PathValue("id"), e.Request.PathValue("grantId"), time.Now())
	if errors.Is(err, oidc.ErrGrantNotFound) {
		return apis.NewNotFoundError("connected app grant not found", nil)
	}
	if err != nil {
		return apis.NewInternalServerError("failed to revoke connected app grant", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func publicOIDCGrant(grant oidc.ActiveGrant) OIDCGrant {
	out := OIDCGrant{
		ID:         grant.ID,
		ClientName: grant.ClientName,
		Scopes:     grant.Scopes,
		Created:    grant.Created.UTC().Format(time.RFC3339),
	}
	if !grant.LastUsedAt.IsZero() {
		lastUsed := grant.LastUsedAt.UTC().Format(time.RFC3339)
		out.LastUsedAt = &lastUsed
	}
	return out
}
