package api

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

func publicOIDCClient(rec *core.Record) OIDCClient {
	var lastUsedAt *string
	if value := rec.GetString("last_used_at"); value != "" {
		lastUsedAt = &value
	}
	return OIDCClient{
		ID:             rec.Id,
		ClientID:       rec.GetString("client_id"),
		Name:           rec.GetString("name"),
		RedirectURIs:   jsonStringList(rec, "redirect_uris"),
		AllowedScopes:  jsonStringList(rec, "allowed_scopes"),
		RefreshEnabled: rec.GetBool("refresh_enabled"),
		SkipConsent:    rec.GetBool("skip_consent"),
		Enabled:        rec.GetBool("enabled"),
		LastUsedAt:     lastUsedAt,
		Created:        rec.GetString("created"),
		Updated:        rec.GetString("updated"),
	}
}

// jsonStringList reads a JSON string array field. A malformed value reads as
// empty instead of failing the whole list.
func jsonStringList(rec *core.Record, field string) []string {
	var out []string
	if err := rec.UnmarshalJSONField(field, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func (h *Handlers) findOIDCClient(id string) (*core.Record, error) {
	rec, err := h.app.FindRecordById(oidc.ClientsCollection, id)
	if err != nil {
		return nil, apis.NewNotFoundError("connected app not found", err)
	}
	return rec, nil
}

func (h *Handlers) listOIDCClients(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter(oidc.ClientsCollection, "", "name", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list connected apps", err)
	}
	out := make([]OIDCClient, 0, len(records))
	for _, rec := range records {
		out = append(out, publicOIDCClient(rec))
	}
	return ok(e, out)
}

func (h *Handlers) createOIDCClient(e *core.RequestEvent) error {
	var in OIDCClientCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	coll, err := h.app.FindCollectionByNameOrId(oidc.ClientsCollection)
	if err != nil {
		return apis.NewBadRequestError("failed to load connected apps", err)
	}
	rec := core.NewRecord(coll)
	if err := applyOIDCClientUpdate(rec, createAsUpdate(in)); err != nil {
		return err
	}
	clientID, err := oidc.NewClientID()
	if err != nil {
		return apis.NewBadRequestError("failed to generate client credentials", err)
	}
	secret, secretHash, err := oidc.NewClientSecret()
	if err != nil {
		return apis.NewBadRequestError("failed to generate client credentials", err)
	}
	rec.Set("client_id", clientID)
	rec.Set("secret_hash", secretHash)
	if e.Auth != nil {
		rec.Set("created_by", e.Auth.Id)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save connected app", err)
	}
	return ok(e, OIDCClientSecretResponse{Client: publicOIDCClient(rec), ClientSecret: secret})
}

// createAsUpdate applies the create defaults: openid and email scopes, enabled.
func createAsUpdate(in OIDCClientCreateRequest) OIDCClientUpdateRequest {
	scopes := in.AllowedScopes
	if scopes == nil {
		scopes = []string{oidc.ScopeOpenID, oidc.ScopeEmail}
	}
	enabled := in.Enabled == nil || *in.Enabled
	return OIDCClientUpdateRequest{
		Name:           &in.Name,
		RedirectURIs:   &in.RedirectURIs,
		AllowedScopes:  &scopes,
		RefreshEnabled: in.RefreshEnabled,
		SkipConsent:    in.SkipConsent,
		Enabled:        &enabled,
	}
}

func (h *Handlers) updateOIDCClient(e *core.RequestEvent) error {
	rec, err := h.findOIDCClient(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in OIDCClientUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if err := applyOIDCClientUpdate(rec, in); err != nil {
		return err
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save connected app", err)
	}
	return ok(e, publicOIDCClient(rec))
}

func (h *Handlers) deleteOIDCClient(e *core.RequestEvent) error {
	rec, err := h.findOIDCClient(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := h.app.Delete(rec); err != nil {
		return apis.NewBadRequestError("failed to delete connected app", err)
	}
	return ok(e, DeleteResponse{Deleted: true})
}

// rotateOIDCClientSecret replaces the secret. Existing tokens stay valid, but
// a refresh needs client authentication, so only the new secret can refresh.
func (h *Handlers) rotateOIDCClientSecret(e *core.RequestEvent) error {
	rec, err := h.findOIDCClient(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	secret, secretHash, err := oidc.NewClientSecret()
	if err != nil {
		return apis.NewBadRequestError("failed to generate client credentials", err)
	}
	rec.Set("secret_hash", secretHash)
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save connected app", err)
	}
	return ok(e, OIDCClientSecretResponse{Client: publicOIDCClient(rec), ClientSecret: secret})
}

// applyOIDCClientUpdate validates every given field before the caller saves,
// so a rejected request changes nothing.
func applyOIDCClientUpdate(rec *core.Record, in OIDCClientUpdateRequest) error {
	if in.Name != nil {
		name, err := normalizeOIDCClientName(*in.Name)
		if err != nil {
			return err
		}
		rec.Set("name", name)
	}
	if in.RedirectURIs != nil {
		uris, err := oidc.NormalizeRedirectURIs(*in.RedirectURIs)
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		rec.Set("redirect_uris", uris)
	}
	if in.AllowedScopes != nil {
		scopes, err := oidc.NormalizeScopes(*in.AllowedScopes)
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		rec.Set("allowed_scopes", scopes)
	}
	setOptionalBool(rec, "refresh_enabled", in.RefreshEnabled)
	setOptionalBool(rec, "skip_consent", in.SkipConsent)
	setOptionalBool(rec, "enabled", in.Enabled)
	return nil
}

func setOptionalBool(rec *core.Record, field string, value *bool) {
	if value != nil {
		rec.Set(field, *value)
	}
}

func normalizeOIDCClientName(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return "", apis.NewBadRequestError("connected app name must be between 1 and 128 characters", nil)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", apis.NewBadRequestError("connected app name cannot contain control characters", nil)
		}
	}
	return name, nil
}
