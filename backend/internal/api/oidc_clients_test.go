package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
	"hysterical-panel/internal/token"
)

const helpCenterCallback = "https://help.example/api/auth/callback/panel"

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func createTestOIDCClient(t *testing.T, h *Handlers, admin *core.Record, body map[string]any) OIDCClientSecretResponse {
	t.Helper()
	e, rec := panelEvent(t, h.app, http.MethodPost, "/api/panel/oidc-clients", "", body, admin)
	if err := h.createOIDCClient(e); err != nil {
		t.Fatalf("create: %v", err)
	}
	return decodeBody[OIDCClientSecretResponse](t, rec)
}

func TestCreateOIDCClientReturnsSecretOnceAndStoresOnlyHash(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newUsersTestRecord(t, app, "oidc-admin@example.com", "AdminSecret123")
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")

	created := createTestOIDCClient(t, h, admin, map[string]any{
		"name": "  Help center  ", "redirect_uris": []string{helpCenterCallback},
	})
	client := created.Client
	if created.ClientSecret == "" || client.ClientID == "" || client.Name != "Help center" {
		t.Fatalf("create response = %+v", created)
	}
	if !slices.Equal(client.AllowedScopes, []string{"openid", "email"}) || !client.Enabled || client.RefreshEnabled || client.SkipConsent {
		t.Fatalf("defaults = %+v", client)
	}
	stored, err := app.FindRecordById(oidc.ClientsCollection, client.ID)
	if err != nil {
		t.Fatalf("find stored client: %v", err)
	}
	if stored.GetString("secret_hash") != token.Sha256Hex(created.ClientSecret) || stored.GetString("created_by") != admin.Id {
		t.Fatalf("stored hash or creator wrong: %v", stored.PublicExport())
	}

	e, rec := panelEvent(t, app, http.MethodGet, "/api/panel/oidc-clients", "", nil, admin)
	if err := h.listOIDCClients(e); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, secret := range []string{created.ClientSecret, stored.GetString("secret_hash")} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("list leaked %q: %s", secret, rec.Body.String())
		}
	}
	if list := decodeBody[[]OIDCClient](t, rec); len(list) != 1 || list[0].ClientID != client.ClientID {
		t.Fatalf("list = %+v", list)
	}
}

func TestCreateOIDCClientRejectsInvalidInput(t *testing.T) {
	cases := map[string]struct {
		body    map[string]any
		message string
	}{
		"empty name":         {map[string]any{"name": " ", "redirect_uris": []string{helpCenterCallback}}, "connected app name must be between 1 and 128 characters"},
		"control character":  {map[string]any{"name": "Help\ncenter", "redirect_uris": []string{helpCenterCallback}}, "connected app name cannot contain control characters"},
		"no redirect":        {map[string]any{"name": "Help", "redirect_uris": []string{}}, "redirect_uris must contain between 1 and 10 entries"},
		"plain http":         {map[string]any{"name": "Help", "redirect_uris": []string{"http://help.example/cb"}}, "redirect uri must be an absolute https url, or http on localhost, without a fragment or credentials"},
		"duplicate redirect": {map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback, helpCenterCallback}}, "redirect uris must be unique"},
		"missing openid":     {map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}, "allowed_scopes": []string{"email"}}, "allowed_scopes must include openid and only contain openid or email"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			app := newMigratedTestApp(t)
			h := newOIDCHandlers(t, app, "", "")
			e, _ := panelEvent(t, app, http.MethodPost, "/api/panel/oidc-clients", "", c.body, nil)
			assertAPIError(t, h.createOIDCClient(e), http.StatusBadRequest, c.message)
			if recs, _ := app.FindAllRecords(oidc.ClientsCollection); len(recs) != 0 {
				t.Fatalf("stored %d clients", len(recs))
			}
		})
	}
}

func TestUpdateOIDCClientChangesOnlyGivenFields(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "", "")
	created := createTestOIDCClient(t, h, nil, map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}})
	id := created.Client.ID

	uris := []string{helpCenterCallback, "http://localhost:3000/api/auth/callback/panel"}
	e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/oidc-clients/"+id, id,
		map[string]any{"enabled": false, "redirect_uris": uris, "refresh_enabled": true}, nil)
	if err := h.updateOIDCClient(e); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated := decodeBody[OIDCClient](t, rec)
	if updated.Name != "Help" || updated.Enabled || !updated.RefreshEnabled || !slices.Equal(updated.RedirectURIs, uris) || updated.ClientID != created.Client.ClientID {
		t.Fatalf("updated = %+v", updated)
	}

	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/oidc-clients/"+id, id,
		map[string]any{"name": "Renamed", "redirect_uris": []string{"http://help.example/cb"}}, nil)
	assertAPIError(t, h.updateOIDCClient(e), http.StatusBadRequest,
		"redirect uri must be an absolute https url, or http on localhost, without a fragment or credentials")
	stored, _ := app.FindRecordById(oidc.ClientsCollection, id)
	if stored.GetString("name") != "Help" {
		t.Fatal("a rejected update changed the stored name")
	}

	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/oidc-clients/missing", "missing", map[string]any{"enabled": true}, nil)
	assertAPIError(t, h.updateOIDCClient(e), http.StatusNotFound, "connected app not found")
}

func TestRotateOIDCClientSecretReplacesHash(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "", "")
	created := createTestOIDCClient(t, h, nil, map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}})
	id := created.Client.ID

	e, rec := panelEvent(t, app, http.MethodPost, "/api/panel/oidc-clients/"+id+"/rotate-secret", id, nil, nil)
	if err := h.rotateOIDCClientSecret(e); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	rotated := decodeBody[OIDCClientSecretResponse](t, rec)
	stored, _ := app.FindRecordById(oidc.ClientsCollection, id)
	if rotated.ClientSecret == created.ClientSecret || stored.GetString("secret_hash") != token.Sha256Hex(rotated.ClientSecret) {
		t.Fatal("rotation did not replace the stored secret")
	}
	if rotated.Client.ClientID != created.Client.ClientID {
		t.Fatal("rotation changed the client id")
	}
}

func TestDeleteOIDCClientRemovesItsGrants(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "oidc-grant@example.com", "GrantSecret123")
	h := newOIDCHandlers(t, app, "", "")
	created := createTestOIDCClient(t, h, nil, map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}})
	grants, _ := app.FindCollectionByNameOrId(oidc.GrantsCollection)
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("client", created.Client.ID)
	grant.Set("scopes", []string{"openid"})
	if err := app.Save(grant); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	id := created.Client.ID
	e, _ := panelEvent(t, app, http.MethodDelete, "/api/panel/oidc-clients/"+id, id, nil, nil)
	if err := h.deleteOIDCClient(e); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := app.FindRecordById(oidc.GrantsCollection, grant.Id); err == nil {
		t.Fatal("grant survived its client")
	}
}
