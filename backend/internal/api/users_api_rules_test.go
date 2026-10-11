package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func serveCollectionRequest(t *testing.T, app core.App, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("build mux: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestGenericUsersRecordAPIIsSuperuserOnly(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "rules@example.com", "RulesSecret123")
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}

	patch := serveCollectionRequest(t, app, http.MethodPatch,
		"/api/collections/users/records/"+user.Id, `{"role":"admin"}`, token)
	if patch.Code < 400 {
		t.Fatalf("self PATCH role=admin status = %d, want an error", patch.Code)
	}
	reloaded, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.GetString("role") != "user" {
		t.Fatalf("role = %q after self PATCH", reloaded.GetString("role"))
	}

	create := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/records",
		`{"email":"intruder@example.com","password":"password12345","passwordConfirm":"password12345","role":"admin","status":"active"}`, "")
	if create.Code < 400 {
		t.Fatalf("anonymous create status = %d, want an error", create.Code)
	}
	if _, err := app.FindAuthRecordByEmail("users", "intruder@example.com"); err == nil {
		t.Fatal("anonymous create stored a user")
	}

	login := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-with-password",
		`{"identity":"rules@example.com","password":"password12345"}`, "")
	if login.Code != http.StatusOK {
		t.Fatalf("password login status = %d body=%s", login.Code, login.Body.String())
	}
	refresh := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-refresh", "", token)
	if refresh.Code != http.StatusOK {
		t.Fatalf("auth refresh status = %d body=%s", refresh.Code, refresh.Body.String())
	}
}
