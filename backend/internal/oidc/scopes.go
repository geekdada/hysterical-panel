package oidc

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// knownScopes is the canonical order scopes are stored and returned in.
var knownScopes = []string{ScopeOpenID, ScopeEmail, ScopeOfflineAccess}

// orderScopes keeps the known scopes of values once each, in canonical order.
func orderScopes(values []string) []string {
	out := []string{}
	for _, scope := range knownScopes {
		if slices.Contains(values, scope) {
			out = append(out, scope)
		}
	}
	return out
}

func unionScopes(a, b []string) []string {
	return orderScopes(append(slices.Clone(a), b...))
}

func isSubset(subset, set []string) bool {
	for _, value := range subset {
		if !slices.Contains(set, value) {
			return false
		}
	}
	return true
}

// grantableScopes drops what the app may not receive. RFC 6749 3.3 lets the
// server grant less than requested, and most OIDC clients ask for profile
// by default.
func grantableScopes(client *core.Record, requested []string) []string {
	allowed := ClientScopes(client)
	if client.GetBool("refresh_enabled") {
		allowed = append(allowed, ScopeOfflineAccess)
	}
	out := []string{}
	for _, scope := range orderScopes(requested) {
		if slices.Contains(allowed, scope) {
			out = append(out, scope)
		}
	}
	return out
}
