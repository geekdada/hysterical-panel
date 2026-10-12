package oidc

import (
	"errors"
	"net/url"
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
	ErrRedirectURIInvalid   = errors.New("redirect URI must be an absolute https URL, or http on localhost, without a fragment or credentials")
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
	if uri == "" || len(uri) > maxRedirectURILength || strings.Contains(uri, "#") {
		return false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return isLoopbackHost(u.Hostname())
	}
	return false
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
