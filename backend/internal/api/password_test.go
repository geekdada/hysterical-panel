package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/oidc"
)

func newPasswordTestMux(t *testing.T, app core.App) http.Handler {
	t.Helper()
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	box, err := cryptobox.New("password-test-master-key")
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	Register(&core.ServeEvent{App: app, Router: r}, app, box, nil, nil, nil, nil, PanelConfigResponse{})
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("mux: %v", err)
	}
	return mux
}

func servePasswordRequest(mux http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func passwordChangeBody(password, confirm string) string {
	body, _ := json.Marshal(PasswordChangeRequest{Password: password, PasswordConfirm: confirm})
	return string(body)
}

func TestChangeOwnPasswordKeepsCallerAndSignsOutOtherSessions(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "password-change@example.com", "PasswordChangeKey123")
	mux := newPasswordTestMux(t, app)

	login := servePasswordRequest(mux, http.MethodPost, "/api/collections/users/auth-with-password",
		`{"identity":"password-change@example.com","password":"password12345"}`, "")
	oldToken := responseToken(t, login)
	signedInAt := reloadUser(t, app, user.Id).GetDateTime("last_login_at")
	authTime, ok, err := oidc.SessionAuthTime(app, oldToken, time.Now())
	if err != nil || !ok {
		t.Fatalf("session auth time before change: ok=%v err=%v", ok, err)
	}

	changed := servePasswordRequest(mux, http.MethodPost, "/api/panel/users/"+user.Id+"/password",
		passwordChangeBody("BrandNewSecret123", "BrandNewSecret123"), oldToken)
	if changed.Code != http.StatusOK {
		t.Fatalf("change status = %d body=%s", changed.Code, changed.Body.String())
	}
	newToken := responseToken(t, changed)

	profilePath := "/api/panel/users/" + user.Id
	if res := servePasswordRequest(mux, http.MethodGet, profilePath, "", oldToken); res.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want 401", res.Code)
	}
	if res := servePasswordRequest(mux, http.MethodGet, profilePath, "", newToken); res.Code != http.StatusOK {
		t.Fatalf("new token status = %d body=%s", res.Code, res.Body.String())
	}
	if !reloadUser(t, app, user.Id).ValidatePassword("BrandNewSecret123") {
		t.Fatal("new password was not saved")
	}
	if got := reloadUser(t, app, user.Id).GetDateTime("last_login_at"); !got.Equal(signedInAt) {
		t.Fatalf("password change counted as a sign-in: last_login_at %v -> %v", signedInAt, got)
	}
	carried, ok, err := oidc.SessionAuthTime(app, newToken, time.Now())
	if err != nil || !ok || !carried.Equal(authTime) {
		t.Fatalf("new token auth time = %v ok=%v err=%v, want %v", carried, ok, err, authTime)
	}
}

func TestChangeOwnPasswordRejections(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "password-guard@example.com", "PasswordGuardKey123")
	other := newUsersTestRecord(t, app, "password-other@example.com", "PasswordOtherKey123")
	disabled := newUsersTestRecord(t, app, "password-disabled@example.com", "PasswordDisabledKey1")
	disabled.Set("status", "disabled")
	if err := app.Save(disabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	mux := newPasswordTestMux(t, app)

	cases := []struct {
		name     string
		as       *core.Record
		targetID string
		body     string
		want     int
		message  string
	}{
		{"other user", user, other.Id, passwordChangeBody("BrandNewSecret123", "BrandNewSecret123"), http.StatusForbidden, "Self access required."},
		{"disabled user", disabled, disabled.Id, passwordChangeBody("BrandNewSecret123", "BrandNewSecret123"), http.StatusForbidden, "Active account required."},
		{"empty password", user, user.Id, passwordChangeBody("", ""), http.StatusBadRequest, "Password is required."},
		{"mismatch", user, user.Id, passwordChangeBody("BrandNewSecret123", "BrandNewSecret124"), http.StatusBadRequest, "Passwords do not match."},
		{"too short", user, user.Id, passwordChangeBody("short", "short"), http.StatusBadRequest, "Password is too short."},
		{"too long", user, user.Id, passwordChangeBody(strings.Repeat("a", 72), strings.Repeat("a", 72)), http.StatusBadRequest, "Password is too long."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := tc.as.NewAuthToken()
			if err != nil {
				t.Fatalf("token: %v", err)
			}
			res := servePasswordRequest(mux, http.MethodPost, "/api/panel/users/"+tc.targetID+"/password", tc.body, token)
			if res.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", res.Code, tc.want, res.Body.String())
			}
			var body struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || body.Message != tc.message {
				t.Fatalf("message = %q err=%v, want %q", body.Message, err, tc.message)
			}
		})
	}
	for _, rec := range []*core.Record{user, other, disabled} {
		if !reloadUser(t, app, rec.Id).ValidatePassword("password12345") {
			t.Fatalf("rejected request changed the password of %s", rec.Email())
		}
	}
}
