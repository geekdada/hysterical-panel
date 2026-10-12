package oidc

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// tokenHTTPRequest builds a parsed token endpoint request. Basic credentials
// are form-encoded first, as RFC 6749 2.3.1 requires.
func tokenHTTPRequest(t *testing.T, form url.Values, basicID, basicSecret string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/oidc/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basicID != "" {
		r.SetBasicAuth(url.QueryEscape(basicID), url.QueryEscape(basicSecret))
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	return r
}

func TestAuthenticateClientAcceptsBasicAndPost(t *testing.T) {
	app := newTestApp(t)
	client, secret := newTestClient(t, app, nil)
	id := client.GetString("client_id")
	for name, r := range map[string]*http.Request{
		"basic":                         tokenHTTPRequest(t, url.Values{}, id, secret),
		"basic with matching client_id": tokenHTTPRequest(t, url.Values{"client_id": {id}}, id, secret),
		"post":                          tokenHTTPRequest(t, url.Values{"client_id": {id}, "client_secret": {secret}}, "", ""),
	} {
		got, err := AuthenticateClient(app, r)
		if err != nil || got.Id != client.Id {
			t.Errorf("%s: client=%v err=%v", name, got, err)
		}
	}
}

func TestAuthenticateClientRejectsBadCredentials(t *testing.T) {
	app := newTestApp(t)
	client, secret := newTestClient(t, app, nil)
	disabled, disabledSecret := newTestClient(t, app, map[string]any{"enabled": false})
	id := client.GetString("client_id")
	cases := map[string]struct {
		r    *http.Request
		code string
	}{
		"no credentials":       {tokenHTTPRequest(t, url.Values{}, "", ""), InvalidClient},
		"client_id only":       {tokenHTTPRequest(t, url.Values{"client_id": {id}}, "", ""), InvalidClient},
		"wrong secret":         {tokenHTTPRequest(t, url.Values{}, id, secret+"x"), InvalidClient},
		"unknown client":       {tokenHTTPRequest(t, url.Values{}, "unknown", secret), InvalidClient},
		"disabled client":      {tokenHTTPRequest(t, url.Values{}, disabled.GetString("client_id"), disabledSecret), InvalidClient},
		"both methods":         {tokenHTTPRequest(t, url.Values{"client_id": {id}, "client_secret": {secret}}, id, secret), InvalidRequest},
		"mismatched client_id": {tokenHTTPRequest(t, url.Values{"client_id": {"other"}}, id, secret), InvalidRequest},
	}
	for name, c := range cases {
		if _, err := AuthenticateClient(app, c.r); err == nil || err.Code != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}
}

func TestHasRepeatedParams(t *testing.T) {
	if HasRepeatedParams(url.Values{"a": {"1"}, "b": {"2"}}) {
		t.Fatal("distinct parameters reported as repeated")
	}
	if !HasRepeatedParams(url.Values{"code": {"1", "2"}}) {
		t.Fatal("repeated code not reported")
	}
}
