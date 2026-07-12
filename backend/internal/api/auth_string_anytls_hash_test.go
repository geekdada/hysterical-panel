package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/token"
	_ "hysterical-panel/migrations"
)

func newMigratedTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

func newUsersTestRecord(t *testing.T, app core.App, email, authString string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("role", "user")
	rec.Set("status", "active")
	rec.Set("verified", true)
	if err := app.RunInTransaction(func(txApp core.App) error {
		if err := txApp.Save(rec); err != nil {
			return err
		}
		_, err := authstrings.CreateCurrent(txApp, rec.Id, authString)
		return err
	}); err != nil {
		t.Fatalf("save user with auth string: %v", err)
	}
	return rec
}

func TestNodeAuthReturnsStableUserIDAndRejectsRetiredCredentials(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "node-auth@example.com", "CurrentSecret")
	h := &Handlers{app: app}

	assertNodeAuthID(t, h, false, "CurrentSecret", user.Id)
	assertNodeAuthID(t, h, true, token.Sha256Hex("CurrentSecret"), user.Id)

	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "NextSecret")
		return err
	}); err != nil {
		t.Fatalf("rotate auth string: %v", err)
	}
	if _, err := authstrings.FindCurrentUserByAuthString(app, "CurrentSecret"); err == nil {
		t.Fatal("retired Hysteria credential still authenticates")
	}
	if _, err := authstrings.FindCurrentUserByAnytlsHash(app, token.Sha256Hex("CurrentSecret")); err == nil {
		t.Fatal("retired AnyTLS credential still authenticates")
	}
	assertNodeAuthID(t, h, false, "NextSecret", user.Id)
	assertNodeAuthID(t, h, true, token.Sha256Hex("NextSecret"), user.Id)
}

func TestManagementLookupMatchesOnlyCurrentAuthString(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "mgmt-lookup@example.com", "OldSecret")
	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "CurrentSecret")
		return err
	}); err != nil {
		t.Fatalf("rotate auth string: %v", err)
	}
	h := &Handlers{app: app}

	oldRequest := httptest.NewRequest("GET", "/api/mgmt/users?auth_string=OldSecret", nil)
	oldEvent := &core.RequestEvent{App: app, Event: router.Event{Request: oldRequest, Response: httptest.NewRecorder()}}
	if err := h.mgmtGetUser(oldEvent); err == nil {
		t.Fatal("Management API found User by Retired Auth String")
	}

	currentRequest := httptest.NewRequest("GET", "/api/mgmt/users?auth_string=CurrentSecret", nil)
	response := httptest.NewRecorder()
	currentEvent := &core.RequestEvent{App: app, Event: router.Event{Request: currentRequest, Response: response}}
	if err := h.mgmtGetUser(currentEvent); err != nil {
		t.Fatalf("Management API current lookup: %v", err)
	}
	var body PanelUser
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != user.Id || body.AuthString != "CurrentSecret" {
		t.Fatalf("Management API response = %+v", body)
	}
}

func TestUserListSearchMatchesOnlyCurrentAuthString(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "list-lookup@example.com", "OldListSecret")
	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "CurrentListSecret")
		return err
	}); err != nil {
		t.Fatalf("rotate auth string: %v", err)
	}
	h := &Handlers{app: app}

	for _, test := range []struct {
		search    string
		wantTotal int64
	}{
		{search: "OldListSecret", wantTotal: 0},
		{search: "CurrentListSecret", wantTotal: 1},
	} {
		request := httptest.NewRequest("GET", "/api/panel/users?search="+test.search, nil)
		response := httptest.NewRecorder()
		event := &core.RequestEvent{App: app, Event: router.Event{Request: request, Response: response}}
		if err := h.listUsers(event); err != nil {
			t.Fatalf("listUsers(%s): %v", test.search, err)
		}
		var body UserListResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode list response: %v", err)
		}
		if body.Total != test.wantTotal {
			t.Fatalf("search %q total = %d, want %d", test.search, body.Total, test.wantTotal)
		}
	}
}

func TestOfficialUserCreationRollsBackWhenAuthStringWasAlreadyUsed(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	first, err := h.newUserRecord(newUserParams{
		Email: "first-create@example.com", Password: "password12345", Role: "user", Status: "active", Verified: true,
	})
	if err != nil {
		t.Fatalf("new first user: %v", err)
	}
	if err := h.saveNewUserWithAuthString(first, "NeverReusable"); err != nil {
		t.Fatalf("save first user: %v", err)
	}

	second, err := h.newUserRecord(newUserParams{
		Email: "second-create@example.com", Password: "password12345", Role: "user", Status: "active", Verified: true,
	})
	if err != nil {
		t.Fatalf("new second user: %v", err)
	}
	if err := h.saveNewUserWithAuthString(second, "NeverReusable"); err == nil {
		t.Fatal("reused Auth String was accepted")
	}
	if _, err := app.FindFirstRecordByFilter("users", "email = 'second-create@example.com'"); err == nil {
		t.Fatal("failed credential creation left an orphan User")
	}
}

func assertNodeAuthID(t *testing.T, h *Handlers, anytls bool, auth, wantID string) {
	t.Helper()
	before, err := h.app.FindRecordById("users", wantID)
	if err != nil {
		t.Fatalf("load User before node auth: %v", err)
	}
	beforeUpdate := before.GetDateTime("last_connected_at").Time()
	if !beforeUpdate.IsZero() {
		time.Sleep(2 * time.Millisecond)
	}
	request := httptest.NewRequest("POST", "/auth", strings.NewReader(`{"addr":"192.0.2.1:1234","auth":"`+auth+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	event := &core.RequestEvent{App: h.app, Event: router.Event{Request: request, Response: response}}
	err = nil
	if anytls {
		err = h.anytlsAuth(event)
	} else {
		err = h.hysteriaAuth(event)
	}
	if err != nil {
		t.Fatalf("node auth: %v", err)
	}
	var body struct {
		OK bool   `json:"ok"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.ID != wantID {
		t.Fatalf("node auth response = %+v, want id %q", body, wantID)
	}
	deadline := time.Now().Add(time.Second)
	for {
		updated, err := h.app.FindRecordById("users", wantID)
		if err != nil {
			t.Fatalf("reload User after node auth: %v", err)
		}
		afterUpdate := updated.GetDateTime("last_connected_at").Time()
		if (!beforeUpdate.IsZero() && afterUpdate.After(beforeUpdate)) || (beforeUpdate.IsZero() && !afterUpdate.IsZero()) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for node auth metadata update")
		}
		time.Sleep(time.Millisecond)
	}
}
