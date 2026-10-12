package oidc

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/token"
)

const testMasterKey = "oidc-test-master-key"

// applyOverrides replaces form values. An empty override removes the
// parameter.
func applyOverrides(form url.Values, overrides map[string]string) {
	for key, value := range overrides {
		if value == "" {
			form.Del(key)
		} else {
			form.Set(key, value)
		}
	}
}

// approvedCode approves a request for user, who signed in a minute earlier,
// and returns the code sent back to the app.
func approvedCode(t *testing.T, app core.App, client, user *core.Record, overrides map[string]string) string {
	t.Helper()
	handle := newPendingHandle(t, app, client, overrides, testNow)
	redirect, err := Approve(app, handle, user, signedInAt(testNow.Add(-time.Minute)), testIssuer, testNow)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return redirectQuery(t, redirect).Get("code")
}

// codeRequest is a valid exchange of code one second after approval.
func codeRequest(client *core.Record, box *cryptobox.Box, code string, overrides map[string]string) TokenRequest {
	form := url.Values{
		"grant_type":    {GrantTypeAuthorizationCode},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"code_verifier": {testVerifier},
	}
	applyOverrides(form, overrides)
	return TokenRequest{Client: client, Form: form, Issuer: testIssuer, Box: box, Now: testNow.Add(time.Second)}
}

// exchangedTokens approves a request and exchanges its code.
func exchangedTokens(t *testing.T, app core.App, box *cryptobox.Box, client, user *core.Record, overrides map[string]string) *TokenResponse {
	t.Helper()
	resp, oauthErr := ExchangeCode(app, codeRequest(client, box, approvedCode(t, app, client, user, overrides), nil))
	if oauthErr != nil {
		t.Fatalf("exchange: %v", oauthErr)
	}
	return resp
}

// parseIDToken verifies an ID token with the active signing key at now.
func parseIDToken(t *testing.T, app core.App, box *cryptobox.Box, client *core.Record, raw string, now time.Time) jwt.MapClaims {
	t.Helper()
	key, err := ActiveSigningKey(app, box, now)
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	parsed, err := jwt.Parse(raw, func(tok *jwt.Token) (any, error) {
		if tok.Header["kid"] != key.ID {
			return nil, errors.New("unexpected kid")
		}
		return &key.Private.PublicKey, nil
	}, jwt.WithValidMethods([]string{SigningAlgorithm}), jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithIssuer(testIssuer), jwt.WithAudience(client.GetString("client_id")))
	if err != nil {
		t.Fatalf("parse id token: %v", err)
	}
	return parsed.Claims.(jwt.MapClaims)
}

// tokenRecord loads a stored access or refresh token by its raw value.
func tokenRecord(t *testing.T, app core.App, raw string) *core.Record {
	t.Helper()
	rec, err := app.FindFirstRecordByFilter(TokensCollection, "token_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(raw)})
	if err != nil {
		t.Fatalf("find token: %v", err)
	}
	return rec
}

func tokenRevoked(t *testing.T, app core.App, raw string) bool {
	t.Helper()
	return !tokenRecord(t, app, raw).GetDateTime("revoked_at").IsZero()
}

func TestExchangeCodeIssuesTokensAndASignedIDToken(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	user := newTestUser(t, app, "exchange@example.com")
	issuedAt := testNow.Add(time.Second)

	resp := exchangedTokens(t, app, box, client, user, map[string]string{"scope": "openid email offline_access"})
	if resp.TokenType != "Bearer" || resp.ExpiresIn != 600 || resp.AccessToken == "" || resp.RefreshToken == "" ||
		resp.Scope != "openid email offline_access" {
		t.Fatalf("response = %+v", resp)
	}
	claims := parseIDToken(t, app, box, client, resp.IDToken, issuedAt)
	want := map[string]any{
		"sub":            user.Id,
		"email":          "exchange@example.com",
		"email_verified": true,
		"nonce":          "nonce-1",
		"auth_time":      float64(testNow.Add(-time.Minute).Unix()),
		"iat":            float64(issuedAt.Unix()),
		"exp":            float64(issuedAt.Add(IDTokenTTL).Unix()),
		"at_hash":        atHash(resp.AccessToken),
	}
	for name, value := range want {
		if claims[name] != value {
			t.Errorf("claim %s = %v, want %v", name, claims[name], value)
		}
	}

	access, refresh := tokenRecord(t, app, resp.AccessToken), tokenRecord(t, app, resp.RefreshToken)
	if access.GetString("kind") != "access" || refresh.GetString("kind") != "refresh" ||
		access.GetString("family") == "" || access.GetString("family") != refresh.GetString("family") ||
		!access.GetDateTime("expires_at").Time().Equal(issuedAt.Add(AccessTokenTTL)) ||
		!refresh.GetDateTime("expires_at").Time().Equal(issuedAt.Add(RefreshTokenIdleTTL)) ||
		!refresh.GetDateTime("absolute_expires_at").Time().Equal(issuedAt.Add(RefreshTokenMaxTTL)) {
		t.Fatalf("stored tokens = %v / %v", access.PublicExport(), refresh.PublicExport())
	}

	grants, err := app.FindAllRecords(GrantsCollection)
	if err != nil || len(grants) != 1 {
		t.Fatalf("grants = %d, %v", len(grants), err)
	}
	fresh, err := app.FindRecordById(ClientsCollection, client.Id)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	for _, rec := range []*core.Record{grants[0], fresh} {
		if !rec.GetDateTime("last_used_at").Time().Equal(issuedAt) {
			t.Errorf("%s last_used_at = %v", rec.Collection().Name, rec.GetDateTime("last_used_at"))
		}
	}
}

func TestExchangeCodeIssuesARefreshTokenOnlyForOfflineAccess(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	user := newTestUser(t, app, "offline@example.com")
	refreshing, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	plain, _ := newTestClient(t, app, nil)
	cases := []struct {
		name   string
		client *core.Record
		scope  string
	}{
		{"without offline_access", refreshing, "openid email"},
		{"refresh turned off", plain, "openid email offline_access"},
	}
	for _, c := range cases {
		resp := exchangedTokens(t, app, box, c.client, user, map[string]string{"scope": c.scope})
		if resp.RefreshToken != "" || resp.Scope != "openid email" {
			t.Errorf("%s: response = %+v", c.name, resp)
		}
	}
}

func TestExchangeCodeRejectsMismatchedRequests(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, nil)
	other, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "mismatch@example.com")
	code := approvedCode(t, app, client, user, nil)

	cases := []struct {
		name      string
		client    *core.Record
		overrides map[string]string
		want      string
	}{
		{"missing code", client, map[string]string{"code": ""}, InvalidRequest},
		{"missing redirect_uri", client, map[string]string{"redirect_uri": ""}, InvalidRequest},
		{"missing code_verifier", client, map[string]string{"code_verifier": ""}, InvalidRequest},
		{"unknown code", client, map[string]string{"code": "unknown"}, InvalidGrant},
		{"wrong code_verifier", client, map[string]string{"code_verifier": strings.Repeat("a", 43)}, InvalidGrant},
		{"short code_verifier", client, map[string]string{"code_verifier": testVerifier[:42]}, InvalidGrant},
		{"other redirect_uri", client, map[string]string{"redirect_uri": "https://app.example.com/other"}, InvalidGrant},
		{"another app", other, nil, InvalidGrant},
	}
	for _, c := range cases {
		resp, oauthErr := ExchangeCode(app, codeRequest(c.client, box, code, c.overrides))
		if resp != nil || oauthErr == nil || oauthErr.Code != c.want {
			t.Errorf("%s: %+v, %v; want %s", c.name, resp, oauthErr, c.want)
		}
	}
	if _, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil)); oauthErr != nil {
		t.Fatalf("failed attempts used up the code: %v", oauthErr)
	}

	expired := codeRequest(client, box, approvedCode(t, app, client, user, nil), nil)
	expired.Now = testNow.Add(CodeTTL)
	if _, oauthErr := ExchangeCode(app, expired); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("expired code: %v", oauthErr)
	}
}

func TestExchangeCodeReplayRevokesOnlyItsFamily(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	user := newTestUser(t, app, "replay@example.com")
	offline := map[string]string{"scope": "openid email offline_access"}

	code := approvedCode(t, app, client, user, offline)
	first, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil))
	if oauthErr != nil {
		t.Fatalf("first exchange: %v", oauthErr)
	}
	other := exchangedTokens(t, app, box, client, user, offline)

	if _, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil)); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("replay: %v", oauthErr)
	}
	if !tokenRevoked(t, app, first.AccessToken) || !tokenRevoked(t, app, first.RefreshToken) {
		t.Fatal("the replayed code's tokens still work")
	}
	if tokenRevoked(t, app, other.AccessToken) || tokenRevoked(t, app, other.RefreshToken) {
		t.Fatal("the replay revoked another family")
	}
	active, err := app.FindRecordsByFilter(GrantsCollection, "revoked_at = ''", "", 0, 0)
	if err != nil || len(active) != 1 {
		t.Fatalf("active grants = %d, %v; the remembered consent must stay", len(active), err)
	}
}

func TestExchangeCodeRejectsAUserWhoCannotSignIn(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "disabled-exchange@example.com")
	code := approvedCode(t, app, client, user, nil)

	user.Set("status", "disabled")
	if err := app.Save(user); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil)); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("disabled user: %v", oauthErr)
	}
}
