package oidc

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/dbx"

	"hysterical-panel/internal/token"
)

func TestUserMayUse(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	disabledClient, _ := newTestClient(t, app, map[string]any{"enabled": false})
	user := newTestUser(t, app, "may-use@example.com")
	if !UserMayUse(client, user) {
		t.Fatal("active verified user rejected")
	}
	if UserMayUse(disabledClient, user) || UserMayUse(nil, user) || UserMayUse(client, nil) {
		t.Fatal("disabled or missing app accepted")
	}
	for field, value := range map[string]any{"status": "disabled", "verified": false} {
		changed := user.Clone()
		changed.Set(field, value)
		if UserMayUse(client, changed) {
			t.Errorf("user with %s=%v accepted", field, value)
		}
	}
}

func TestApproveIssuesASingleUseCode(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "approve@example.com")
	handle := newPendingHandle(t, app, client, nil, testNow)

	redirect, err := Approve(app, handle, user, signedInAt(testNow.Add(-time.Minute)), testIssuer, testNow)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	q := redirectQuery(t, redirect)
	if q.Get("code") == "" || q.Get("state") != "state-1" || q.Get("iss") != testIssuer {
		t.Fatalf("redirect = %s", redirect)
	}
	code, err := app.FindFirstRecordByFilter(CodesCollection, "code_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(q.Get("code"))})
	if err != nil {
		t.Fatalf("stored code: %v", err)
	}
	if code.GetString("nonce") != "nonce-1" || code.GetString("scope") != "openid email" || code.GetString("family") == "" ||
		!code.GetDateTime("expires_at").Time().Equal(testNow.Add(CodeTTL)) ||
		!code.GetDateTime("auth_time").Time().Equal(testNow.Add(-time.Minute)) {
		t.Fatalf("stored code = %v", code.PublicExport())
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("second approve: %v, want ErrRequestNotFound", err)
	}
}

func TestConsentIsRememberedUntilScopesGrow(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "remember@example.com")
	approve := func(scope string) {
		t.Helper()
		handle := newPendingHandle(t, app, client, map[string]string{"scope": scope}, testNow)
		if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); err != nil {
			t.Fatalf("approve %q: %v", scope, err)
		}
	}
	approve("openid")

	cases := map[string]struct {
		overrides map[string]string
		want      bool
	}{
		"same scope":            {map[string]string{"scope": "openid"}, false},
		"wider scope":           {map[string]string{"scope": "openid email"}, true},
		"prompt=consent":        {map[string]string{"scope": "openid", "prompt": "consent"}, true},
		"prompt=select_account": {map[string]string{"scope": "openid", "prompt": "select_account"}, true},
	}
	for name, c := range cases {
		p, err := FindPendingRequest(app, newPendingHandle(t, app, client, c.overrides, testNow), testNow)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if needs, err := p.NeedsConsent(app, user.Id); err != nil || needs != c.want {
			t.Errorf("%s: NeedsConsent = %v, %v; want %v", name, needs, err, c.want)
		}
	}

	approve("openid email")
	grants, err := app.FindAllRecords(GrantsCollection)
	if err != nil || len(grants) != 1 || !slices.Equal(grantScopes(grants[0]), []string{"openid", "email"}) {
		t.Fatalf("grants = %d, err %v", len(grants), err)
	}

	skipping, _ := newTestClient(t, app, map[string]any{"skip_consent": true})
	p, err := FindPendingRequest(app, newPendingHandle(t, app, skipping, nil, testNow), testNow)
	if err != nil {
		t.Fatalf("skip_consent request: %v", err)
	}
	if needs, _ := p.NeedsConsent(app, user.Id); needs {
		t.Fatal("an app marked skip_consent asked for consent")
	}
}

func TestNeedsReauthentication(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	cases := []struct {
		name      string
		overrides map[string]string
		session   Session
		want      bool
	}{
		{"no prompt, unknown session", nil, Session{}, false},
		{"login, signed in before the request", map[string]string{"prompt": "login"}, signedInAt(testNow.Add(-time.Second)), true},
		{"login, signed in after the request", map[string]string{"prompt": "login"}, signedInAt(testNow.Add(time.Second)), false},
		{"login, unknown session", map[string]string{"prompt": "login"}, Session{}, true},
		{"max_age, recent sign-in", map[string]string{"max_age": "300"}, signedInAt(testNow.Add(-299 * time.Second)), false},
		{"max_age, old sign-in", map[string]string{"max_age": "300"}, signedInAt(testNow.Add(-301 * time.Second)), true},
		{"max_age, unknown session", map[string]string{"max_age": "300"}, Session{}, true},
	}
	for _, c := range cases {
		p, err := FindPendingRequest(app, newPendingHandle(t, app, client, c.overrides, testNow), testNow)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := p.NeedsReauthentication(c.session, testNow); got != c.want {
			t.Errorf("%s: NeedsReauthentication = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestApproveHandlesStaleAndSilentRequests(t *testing.T) {
	app := newTestApp(t)
	remembered, _ := newTestClient(t, app, nil)
	unknown, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "silent@example.com")

	stale := newPendingHandle(t, app, remembered, map[string]string{"prompt": "login"}, testNow)
	if _, err := Approve(app, stale, user, signedInAt(testNow.Add(-time.Hour)), testIssuer, testNow); !errors.Is(err, ErrReauthenticationRequired) {
		t.Fatalf("stale session: %v, want ErrReauthenticationRequired", err)
	}
	if _, err := Approve(app, stale, user, signedInAt(testNow.Add(time.Second)), testIssuer, testNow.Add(2*time.Second)); err != nil {
		t.Fatalf("after signing in again: %v", err)
	}

	silent := newPendingHandle(t, app, remembered, map[string]string{"prompt": "none"}, testNow)
	redirect, err := Approve(app, silent, user, Session{}, testIssuer, testNow)
	if err != nil || redirectQuery(t, redirect).Get("code") == "" {
		t.Fatalf("silent request with remembered consent = %q, %v", redirect, err)
	}

	for overrides, want := range map[string]string{"": ConsentRequired, "60": LoginRequired} {
		handle := newPendingHandle(t, app, unknown, map[string]string{"prompt": "none", "max_age": overrides}, testNow)
		redirect, err := Approve(app, handle, user, Session{}, testIssuer, testNow)
		if err != nil || redirectQuery(t, redirect).Get("error") != want {
			t.Errorf("max_age=%q: redirect %q, err %v; want %s", overrides, redirect, err, want)
		}
		if _, err := FindPendingRequest(app, handle, testNow); !errors.Is(err, ErrRequestNotFound) {
			t.Errorf("max_age=%q: a silent failure left the request pending", overrides)
		}
	}
}

func TestApproveRejectsUnusableUsersAppsAndExpiredRequests(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "blocked@example.com")
	handle := newPendingHandle(t, app, client, nil, testNow)

	// The caller's copy of the User predates the disable, as the panel's
	// session record does when the disable commits mid-request.
	stored, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	stored.Set("status", "disabled")
	if err := app.Save(stored); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); !errors.Is(err, ErrUserNotAllowed) {
		t.Fatalf("disabled user: %v", err)
	}
	stored.Set("status", "active")
	if err := app.Save(stored); err != nil {
		t.Fatalf("enable user: %v", err)
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow.Add(AuthRequestTTL)); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("expired request: %v", err)
	}
	client.Set("enabled", false)
	if err := app.Save(client); err != nil {
		t.Fatalf("disable client: %v", err)
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("disabled app: %v", err)
	}
}

func TestDenyAndAbortEndTheRequest(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)

	handle := newPendingHandle(t, app, client, nil, testNow)
	redirect, err := Deny(app, handle, testIssuer, testNow)
	if q := redirectQuery(t, redirect); err != nil || q.Get("error") != AccessDenied || q.Get("state") != "state-1" || q.Get("iss") != testIssuer {
		t.Fatalf("deny = %q, %v", redirect, err)
	}
	if _, err := Deny(app, handle, testIssuer, testNow); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("second deny: %v", err)
	}

	for prompt, want := range map[string]string{"none": LoginRequired, "": AccessDenied} {
		handle := newPendingHandle(t, app, client, map[string]string{"prompt": prompt}, testNow)
		redirect, err := Abort(app, handle, testIssuer, testNow)
		if err != nil || redirectQuery(t, redirect).Get("error") != want {
			t.Errorf("abort with prompt=%q = %q, %v; want %s", prompt, redirect, err, want)
		}
	}
}
