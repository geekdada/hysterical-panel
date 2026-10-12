package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hysterical-panel/internal/oidc"
)

func TestSessionAuthTimeFollowsSignInAndRefresh(t *testing.T) {
	app := newMigratedTestApp(t)
	newUsersTestRecord(t, app, "oidc-session@example.com", "SessionSecret123")
	(&Handlers{app: app}).bindOIDCSessionAuthTimes()

	login := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-with-password",
		`{"identity":"oidc-session@example.com","password":"password12345"}`, "")
	loginToken := responseToken(t, login)
	signedIn, ok, err := oidc.SessionAuthTime(app, loginToken, time.Now())
	if err != nil || !ok {
		t.Fatalf("password login was not recorded: ok=%v err=%v", ok, err)
	}

	// A refresh within the same second returns a byte-identical JWT.
	time.Sleep(1100 * time.Millisecond)
	refresh := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-refresh", "", "Bearer "+loginToken)
	refreshed := responseToken(t, refresh)
	if refreshed == loginToken {
		t.Fatal("refresh returned the login token")
	}
	carried, ok, err := oidc.SessionAuthTime(app, refreshed, time.Now())
	if err != nil || !ok || !carried.Equal(signedIn) {
		t.Fatalf("refreshed auth time = %v ok=%v err=%v, want %v", carried, ok, err, signedIn)
	}
}

func TestSessionAuthTimeUnknownForTokensIssuedOutsideSignIn(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "oidc-legacy@example.com", "LegacySecret123")
	(&Handlers{app: app}).bindOIDCSessionAuthTimes()
	legacy, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}
	refresh := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-refresh", "", legacy)
	if _, ok, err := oidc.SessionAuthTime(app, responseToken(t, refresh), time.Now()); err != nil || ok {
		t.Fatalf("refresh of an unrecorded token produced an auth time: ok=%v err=%v", ok, err)
	}
}

func TestBearerTokenAcceptsOptionalPrefix(t *testing.T) {
	for header, want := range map[string]string{"Bearer abc": "abc", "bearer abc": "abc", "abc": "abc", "": ""} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", header)
		if got := bearerToken(r); got != want {
			t.Fatalf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}

func responseToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Token == "" {
		t.Fatalf("decode token: %v body=%s", err, rec.Body.String())
	}
	return body.Token
}

func TestIssueSessionTokenRecordsSignIn(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "oidc-register@example.com", "RegisterSecret123")
	sessionToken, err := issueSessionToken(app, user)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, ok, err := oidc.SessionAuthTime(app, sessionToken, time.Now()); err != nil || !ok {
		t.Fatalf("registration token has no auth time: ok=%v err=%v", ok, err)
	}
}
