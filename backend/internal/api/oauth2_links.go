package api

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// listOAuthLinks returns the OAuth2 identities linked to a User. PocketBase's
// _externalAuths rules only let a user see their own links, so admins need
// this route to manage other users.
func (h *Handlers) listOAuthLinks(e *core.RequestEvent) error {
	user, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	links, err := h.app.FindAllExternalAuthsByRecord(user)
	if err != nil {
		return apis.NewBadRequestError("failed to list oauth links", err)
	}
	out := make([]OAuthLink, 0, len(links))
	for _, link := range links {
		out = append(out, publicOAuthLink(link))
	}
	return ok(e, out)
}

func (h *Handlers) deleteOAuthLink(e *core.RequestEvent) error {
	user, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	link, err := h.app.FindFirstExternalAuthByExpr(dbx.HashExp{
		"id":            e.Request.PathValue("linkId"),
		"collectionRef": user.Collection().Id,
		"recordRef":     user.Id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return apis.NewNotFoundError("oauth link not found", nil)
	}
	if err != nil {
		return apis.NewBadRequestError("failed to find oauth link", err)
	}
	if err := h.app.Delete(link); err != nil {
		return apis.NewBadRequestError("failed to delete oauth link", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func publicOAuthLink(link *core.ExternalAuth) OAuthLink {
	return OAuthLink{
		ID:       link.Id,
		Provider: link.Provider(),
		Created:  link.Created().Time().UTC().Format(time.RFC3339),
	}
}
