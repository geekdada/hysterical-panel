package oidc

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"

	"hysterical-panel/internal/token"
)

func TestParseAuthorizeRequestAcceptsAValidRequest(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{
		"scope": "profile offline_access email openid", "prompt": "consent login", "max_age": "300", "ui_locales": "zh-CN",
	}))
	if failure != nil {
		t.Fatalf("failure = %+v", failure)
	}
	if req.Client.Id != client.Id || req.RedirectURI != testRedirectURI || req.State != "state-1" || req.Nonce != "nonce-1" ||
		req.CodeChallenge != testChallenge || req.MaxAge != 300 ||
		!slices.Equal(req.Scopes, []string{"openid", "email", "offline_access"}) ||
		!slices.Equal(req.Prompt, []string{"login", "consent"}) {
		t.Fatalf("request = %+v", req)
	}
}

type authorizePageCase struct {
	form url.Values
	page string
}

func TestParseAuthorizeRequestShowsAnErrorPageForUntrustedRedirects(t *testing.T) {
	app := newTestApp(t)
	const withQuery = "https://app.example.com/cb?tenant=1"
	client, _ := newTestClient(t, app, map[string]any{"redirect_uris": []string{testRedirectURI, withQuery}})
	disabled, _ := newTestClient(t, app, map[string]any{"enabled": false})
	repeated := authorizeForm(client, nil)
	repeated.Add("redirect_uri", testRedirectURI)
	cases := map[string]authorizePageCase{
		"missing client_id":     {authorizeForm(client, map[string]string{"client_id": ""}), PageInvalidClient},
		"unknown client":        {authorizeForm(client, map[string]string{"client_id": "unknown"}), PageInvalidClient},
		"disabled client":       {authorizeForm(disabled, nil), PageInvalidClient},
		"missing redirect_uri":  {authorizeForm(client, map[string]string{"redirect_uri": ""}), PageInvalidRedirectURI},
		"repeated redirect_uri": {repeated, PageInvalidRequest},
	}
	// RFC 9700 2.1: only a byte-for-byte match counts.
	for _, uri := range []string{
		testRedirectURI + "/",
		"https://APP.example.com/callback",
		"https://app.example.com/Callback",
		testRedirectURI + "?x=1",
		testRedirectURI + "#fragment",
		"https://user@app.example.com/callback",
		"https://app.example.com:443/callback",
		"http://app.example.com/callback",
		"https://app.example.com/cb?tenant=2",
	} {
		cases["redirect "+uri] = authorizePageCase{authorizeForm(client, map[string]string{"redirect_uri": uri}), PageInvalidRedirectURI}
	}
	for name, c := range cases {
		_, failure := ParseAuthorizeRequest(app, c.form)
		if failure == nil || failure.Err != nil || failure.Page != c.page {
			t.Errorf("%s: failure = %+v, want page %s", name, failure, c.page)
		}
	}
	if _, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{"redirect_uri": withQuery})); failure != nil {
		t.Fatalf("registered URI with a query rejected: %+v", failure)
	}
}

func TestParseAuthorizeRequestRedirectsOtherErrors(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	cases := map[string]struct {
		overrides map[string]string
		code      string
	}{
		"token response type":    {map[string]string{"response_type": "token"}, UnsupportedResponseType},
		"missing response type":  {map[string]string{"response_type": ""}, InvalidRequest},
		"fragment response mode": {map[string]string{"response_mode": "fragment"}, InvalidRequest},
		"no openid":              {map[string]string{"scope": "email"}, InvalidScope},
		"plain pkce":             {map[string]string{"code_challenge_method": "plain"}, InvalidRequest},
		"missing pkce method":    {map[string]string{"code_challenge_method": ""}, InvalidRequest},
		"missing challenge":      {map[string]string{"code_challenge": ""}, InvalidRequest},
		"short challenge":        {map[string]string{"code_challenge": "abc"}, InvalidRequest},
		"unknown prompt":         {map[string]string{"prompt": "sometimes"}, InvalidRequest},
		"none with login":        {map[string]string{"prompt": "none login"}, InvalidRequest},
		"negative max_age":       {map[string]string{"max_age": "-1"}, InvalidRequest},
		"text max_age":           {map[string]string{"max_age": "soon"}, InvalidRequest},
		"none with max_age=0":    {map[string]string{"prompt": "none", "max_age": "0"}, LoginRequired},
		"request object":         {map[string]string{"request": "eyJhbGciOiJub25lIn0"}, RequestNotSupported},
		"request_uri":            {map[string]string{"request_uri": "https://evil.example/request"}, RequestURINotSupported},
		"long nonce":             {map[string]string{"nonce": strings.Repeat("n", 1025)}, InvalidRequest},
	}
	for name, c := range cases {
		_, failure := ParseAuthorizeRequest(app, authorizeForm(client, c.overrides))
		if failure == nil || failure.Err == nil || failure.Err.Code != c.code || failure.RedirectURI != testRedirectURI || failure.State != "state-1" {
			t.Errorf("%s: failure = %+v, want %s back to the client with its state", name, failure, c.code)
		}
	}

	repeated := authorizeForm(client, nil)
	repeated.Add("code_challenge", testChallenge)
	if _, failure := ParseAuthorizeRequest(app, repeated); failure == nil || failure.Err == nil || failure.Err.Code != InvalidRequest {
		t.Errorf("repeated code_challenge: failure = %+v", failure)
	}
	_, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{"state": strings.Repeat("s", 1025), "response_type": "token"}))
	if failure == nil || failure.Err == nil || failure.State != "" {
		t.Errorf("an oversized state must not be echoed: failure = %+v", failure)
	}
}

func TestParseAuthorizeRequestNarrowsScopesAndTurnsMaxAgeZeroIntoLogin(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, map[string]any{"allowed_scopes": []string{ScopeOpenID}})
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{
		"scope": "openid email profile offline_access", "max_age": "0",
	}))
	if failure != nil {
		t.Fatalf("failure = %+v", failure)
	}
	if !slices.Equal(req.Scopes, []string{ScopeOpenID}) || !slices.Equal(req.Prompt, []string{PromptLogin}) || req.MaxAge != 0 {
		t.Fatalf("request = %+v", req)
	}
}

func TestSaveAuthRequestStoresOnlyTheHandleHash(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{"prompt": "login consent"}))
	if failure != nil {
		t.Fatalf("failure = %+v", failure)
	}
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	handle, err := SaveAuthRequest(app, req, now)
	if err != nil || len(handle) < 43 {
		t.Fatalf("handle %q, err %v", handle, err)
	}
	rec, err := app.FindFirstRecordByFilter(AuthRequestsCollection, "request_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(handle)})
	if err != nil {
		t.Fatalf("stored request: %v", err)
	}
	if rec.GetString("scope") != "openid email" || rec.GetString("prompt") != "login consent" ||
		rec.GetString("code_challenge") != testChallenge || !rec.GetDateTime("expires_at").Time().Equal(now.Add(AuthRequestTTL)) {
		t.Fatalf("stored request = %v", rec.PublicExport())
	}
}

func TestErrorRedirectKeepsTheRegisteredQuery(t *testing.T) {
	got := ErrorRedirect("https://app.example.com/cb?tenant=a%20b", NewError(AccessDenied, "denied"), "s 1", testIssuer)
	if !strings.HasPrefix(got, "https://app.example.com/cb?tenant=a%20b&") {
		t.Fatalf("redirect = %s", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("error") != AccessDenied || q.Get("error_description") != "denied" || q.Get("state") != "s 1" || q.Get("iss") != testIssuer {
		t.Fatalf("query = %v", q)
	}
}
