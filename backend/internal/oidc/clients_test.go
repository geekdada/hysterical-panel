package oidc

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"hysterical-panel/internal/token"
)

func TestNormalizeRedirectURIsAcceptsHTTPSAndLoopbackHTTP(t *testing.T) {
	valid := []string{
		"https://app.example/cb",
		"https://app.example/cb?tenant=1",
		"https://app.example:8443/auth/callback",
		"http://localhost:3000/cb",
		"http://127.0.0.1/cb",
		"http://[::1]:8080/cb",
		"https://xn--exmple-cua.com/cb",
		"https://app.example/c%20b",
	}
	got, err := NormalizeRedirectURIs(append([]string{"  https://trim.example/cb  "}, valid...))
	if err != nil {
		t.Fatalf("valid redirect URIs rejected: %v", err)
	}
	if want := append([]string{"https://trim.example/cb"}, valid...); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Exact matching keeps a trailing slash meaningful, so both spellings stay.
	if got, err := NormalizeRedirectURIs([]string{"https://a.example/cb", "https://a.example/cb/"}); err != nil || len(got) != 2 {
		t.Fatalf("distinct spellings: %q, %v", got, err)
	}
}

func TestNormalizeRedirectURIsRejectsUnsafeValues(t *testing.T) {
	for _, bad := range []string{
		"",
		"/cb",
		"app.example/cb",
		"http://app.example/cb",
		"http://localhost.example/cb",
		"https://app.example/cb#fragment",
		"https://user:pass@app.example/cb",
		"ftp://app.example/cb",
		"javascript:alert(1)",
		"https:///cb",
		"https://app.example/" + strings.Repeat("a", 2048),
		// These parse, but /authorize compares bytes, so no client could ever
		// send a matching value, and look-alike hosts would reach the UI.
		"HTTPS://app.example/cb",
		"https://App.Example/cb",
		"https://app.example/c b",
		"https://app.example/c\tb",
		"https://exаmple.com/cb",
		"https://app.example:65536/cb",
		"https://app.example:0/cb",
		"https://:443/cb",
	} {
		if _, err := NormalizeRedirectURIs([]string{bad}); !errors.Is(err, ErrRedirectURIInvalid) {
			t.Errorf("%q: err = %v, want ErrRedirectURIInvalid", bad, err)
		}
	}
	eleven := make([]string, MaxRedirectURIs+1)
	for i := range eleven {
		eleven[i] = fmt.Sprintf("https://app.example/cb%d", i)
	}
	for _, list := range [][]string{nil, {}, eleven} {
		if _, err := NormalizeRedirectURIs(list); !errors.Is(err, ErrRedirectURICount) {
			t.Errorf("%d entries: err = %v, want ErrRedirectURICount", len(list), err)
		}
	}
	if _, err := NormalizeRedirectURIs([]string{"https://a.example/cb", " https://a.example/cb"}); !errors.Is(err, ErrRedirectURIDuplicate) {
		t.Errorf("duplicate: err = %v, want ErrRedirectURIDuplicate", err)
	}
}

func TestNormalizeScopes(t *testing.T) {
	if got, err := NormalizeScopes([]string{"email", "openid", "email"}); err != nil || !slices.Equal(got, []string{"openid", "email"}) {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := NormalizeScopes([]string{"openid"}); err != nil || !slices.Equal(got, []string{"openid"}) {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range [][]string{nil, {"email"}, {"openid", "profile"}, {"openid", "offline_access"}} {
		if _, err := NormalizeScopes(bad); !errors.Is(err, ErrScopesInvalid) {
			t.Errorf("%q: err = %v, want ErrScopesInvalid", bad, err)
		}
	}
}

func TestNewClientCredentials(t *testing.T) {
	id, err := NewClientID()
	if err != nil || len(id) < 20 {
		t.Fatalf("client id %q, %v", id, err)
	}
	secret, hash, err := NewClientSecret()
	if err != nil || len(secret) < 43 || hash != token.Sha256Hex(secret) {
		t.Fatalf("secret %q hash %q err %v", secret, hash, err)
	}
	other, _, _ := NewClientSecret()
	if other == secret {
		t.Fatal("two secrets are equal")
	}
}
