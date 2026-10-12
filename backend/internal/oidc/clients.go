package oidc

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"hysterical-panel/internal/token"
)

const (
	ScopeOpenID = "openid"
	ScopeEmail  = "email"

	// MaxRedirectURIs bounds the admin form and the exact-match lookup.
	MaxRedirectURIs      = 10
	maxRedirectURILength = 2048
)

// The messages are API error strings that the frontend localizes.
var (
	ErrRedirectURICount     = errors.New("redirect_uris must contain between 1 and 10 entries")
	ErrRedirectURIInvalid   = errors.New("redirect URI must be an absolute https URL, or http on a loopback host, with a lowercase ASCII host and no spaces, fragment or credentials")
	ErrRedirectURIDuplicate = errors.New("redirect URIs must be unique")
	ErrScopesInvalid        = errors.New("allowed_scopes must include openid and only contain openid or email")
)

// NormalizeRedirectURIs trims and validates each entry. /authorize compares
// redirect_uri by exact string match (RFC 9700 2.1), so values are otherwise
// stored as entered.
func NormalizeRedirectURIs(raw []string) ([]string, error) {
	if len(raw) == 0 || len(raw) > MaxRedirectURIs {
		return nil, ErrRedirectURICount
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, entry := range raw {
		uri := strings.TrimSpace(entry)
		if !validRedirectURI(uri) {
			return nil, ErrRedirectURIInvalid
		}
		if seen[uri] {
			return nil, ErrRedirectURIDuplicate
		}
		seen[uri] = true
		out = append(out, uri)
	}
	return out, nil
}

// RFC 6749 3.1.2 forbids fragments. Plain http is allowed only on loopback
// hosts, where the code never crosses the network.
func validRedirectURI(uri string) bool {
	if uri == "" || len(uri) > maxRedirectURILength || strings.Contains(uri, "#") || !printableASCII(uri) {
		return false
	}
	u, err := url.Parse(uri)
	if err != nil || u.User != nil || u.Opaque != "" || !canonicalHost(u) {
		return false
	}
	// url.Parse lowercases the scheme, so the prefix check rejects "HTTPS://".
	switch {
	case strings.HasPrefix(uri, "https://"):
		return true
	case strings.HasPrefix(uri, "http://"):
		return isLoopbackHost(u.Hostname())
	}
	return false
}

// Redirect URIs are compared byte for byte, so anything a client would send
// in another spelling can never match. Internationalized hosts must be
// entered as punycode, which also keeps look-alike hosts out of the UI.
func printableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

func canonicalHost(u *url.URL) bool {
	host := u.Hostname()
	if host == "" || host != strings.ToLower(host) {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		return err == nil && n >= 1 && n <= 65535
	}
	return !strings.HasSuffix(u.Host, ":")
}

// NormalizeScopes returns the allowed scopes in canonical order. Refresh
// tokens depend on the client's refresh_enabled flag, not on a scope here.
func NormalizeScopes(raw []string) ([]string, error) {
	has := map[string]bool{}
	for _, scope := range raw {
		scope = strings.TrimSpace(scope)
		if scope != ScopeOpenID && scope != ScopeEmail {
			return nil, ErrScopesInvalid
		}
		has[scope] = true
	}
	if !has[ScopeOpenID] {
		return nil, ErrScopesInvalid
	}
	out := []string{ScopeOpenID}
	if has[ScopeEmail] {
		out = append(out, ScopeEmail)
	}
	return out, nil
}

// NewClientID returns a random public client identifier.
func NewClientID() (string, error) {
	return token.New(16)
}

// NewClientSecret returns a 256-bit secret, shown to the admin once, and its
// SHA-256 hash, which is all the panel stores.
func NewClientSecret() (secret, secretHash string, err error) {
	secret, err = token.New(32)
	if err != nil {
		return "", "", err
	}
	return secret, token.Sha256Hex(secret), nil
}
