package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/authstrings"
)

func TestAdminCreatePatchAndResetMaintainCurrentAuthString(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}

	createEvent, createResponse := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users", map[string]any{
		"email": "admin-created@example.com", "password": "password12345", "auth_string": "AdminCreatedSecret",
	})
	if err := h.createUser(createEvent); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	var created PanelUser
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created User: %v", err)
	}
	if created.AuthString != "AdminCreatedSecret" {
		t.Fatalf("created auth_string = %q", created.AuthString)
	}

	patchEvent, patchResponse := jsonRequestEvent(t, app, http.MethodPatch, "/api/panel/users/"+created.ID, map[string]any{
		"auth_string": "PatchedSecret",
	})
	patchEvent.Request.SetPathValue("id", created.ID)
	if err := h.updateUser(patchEvent); err != nil {
		t.Fatalf("updateUser: %v", err)
	}
	var patched PanelUser
	if err := json.Unmarshal(patchResponse.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patched User: %v", err)
	}
	if patched.AuthString != "PatchedSecret" {
		t.Fatalf("patched auth_string = %q", patched.AuthString)
	}
	assertCredentialState(t, app, "AdminCreatedSecret", authstrings.Retired)

	resetEvent, resetResponse := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+created.ID+"/reset-auth-string", nil)
	resetEvent.Request.SetPathValue("id", created.ID)
	if err := h.resetUserAuthString(resetEvent); err != nil {
		t.Fatalf("resetUserAuthString: %v", err)
	}
	var reset PanelUser
	if err := json.Unmarshal(resetResponse.Body.Bytes(), &reset); err != nil {
		t.Fatalf("decode reset User: %v", err)
	}
	if reset.AuthString == "" || reset.AuthString == "PatchedSecret" {
		t.Fatalf("reset auth_string = %q", reset.AuthString)
	}
	assertCredentialState(t, app, "PatchedSecret", authstrings.Retired)
	assertCredentialState(t, app, reset.AuthString, authstrings.Current)
}

func TestRegistrationAndManagementCreationProvisionCurrentAuthStrings(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{
		app:           app,
		registerLimit: newPasskeyRateLimiter(registerRateMax, registerRateWindow),
	}
	settings, err := h.settingsRecord()
	if err != nil {
		t.Fatalf("settingsRecord: %v", err)
	}
	settings.Set("open_registration", true)
	settings.Set("invitations_enabled", true)
	settings.Set("require_invite_for_open", true)
	if err := app.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	invitations, err := app.FindCollectionByNameOrId("invitations")
	if err != nil {
		t.Fatalf("find invitations: %v", err)
	}
	invite := core.NewRecord(invitations)
	invite.Set("code", "InviteCode")
	invite.Set("max_uses", 1)
	if err := app.Save(invite); err != nil {
		t.Fatalf("save invite: %v", err)
	}

	registerEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/register", map[string]any{
		"email": "registered@example.com", "password": "password12345", "code": "InviteCode",
	})
	if err := h.register(registerEvent); err != nil {
		t.Fatalf("register: %v", err)
	}
	registered, err := app.FindFirstRecordByFilter("users", "email = 'registered@example.com'")
	if err != nil {
		t.Fatalf("find registered User: %v", err)
	}
	if current, err := authstrings.CurrentValue(app, registered.Id); err != nil || current == "" {
		t.Fatalf("registered Current Auth String = %q err=%v", current, err)
	}

	mgmtEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/mgmt/users", map[string]any{
		"email": "managed@example.com",
	})
	if err := h.mgmtCreateUser(mgmtEvent); err != nil {
		t.Fatalf("mgmtCreateUser: %v", err)
	}
	managed, err := app.FindFirstRecordByFilter("users", "email = 'managed@example.com'")
	if err != nil {
		t.Fatalf("find managed User: %v", err)
	}
	if current, err := authstrings.CurrentValue(app, managed.Id); err != nil || current == "" {
		t.Fatalf("managed Current Auth String = %q err=%v", current, err)
	}
}

func assertCredentialState(t *testing.T, app core.App, authString, wantState string) {
	t.Helper()
	record, err := app.FindFirstRecordByFilter(
		authstrings.Collection,
		"auth_string = {:auth}",
		map[string]any{"auth": authString},
	)
	if err != nil {
		t.Fatalf("find credential %q: %v", authString, err)
	}
	if got := record.GetString("state"); got != wantState {
		t.Fatalf("credential %q state = %q, want %q", authString, got, wantState)
	}
}

func jsonRequestEvent(t *testing.T, app core.App, method, path string, body any) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	return &core.RequestEvent{App: app, Event: router.Event{Request: request, Response: response}}, response
}
