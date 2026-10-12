package oidc

import (
	"testing"
	"time"
)

func TestCleanupRemovesOnlyExpiredRows(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	user := newTestUser(t, app, "cleanup@example.com")
	client := saveTestRecord(t, app, ClientsCollection, map[string]any{
		"client_id": "cleanup", "name": "Cleanup", "secret_hash": "x",
		"redirect_uris": []string{"https://app.example/cb"}, "allowed_scopes": []string{"openid"},
	})
	grant := saveTestRecord(t, app, GrantsCollection, map[string]any{"user": user.Id, "client": client.Id, "scopes": []string{"openid"}})

	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	for i, expires := range []time.Time{past, future} {
		suffix := string(rune('a' + i))
		saveTestRecord(t, app, AuthRequestsCollection, map[string]any{
			"request_hash": "req-" + suffix, "client": client.Id, "redirect_uri": "https://app.example/cb",
			"scope": "openid", "code_challenge": "c", "expires_at": expires,
		})
		saveTestRecord(t, app, CodesCollection, map[string]any{
			"code_hash": "code-" + suffix, "grant": grant.Id, "family": "f", "redirect_uri": "https://app.example/cb",
			"scope": "openid", "code_challenge": "c", "expires_at": expires,
		})
		saveTestRecord(t, app, SessionAuthTimesCollection, map[string]any{
			"token_hash": "session-" + suffix, "user": user.Id, "auth_time": past, "expires_at": expires,
		})
	}
	tokenExpiries := []time.Time{now.Add(-expiredTokenRetention - time.Minute), now.Add(-time.Hour)}
	for i, expires := range tokenExpiries {
		saveTestRecord(t, app, TokensCollection, map[string]any{
			"token_hash": "token-" + string(rune('a'+i)), "kind": "refresh", "grant": grant.Id,
			"family": "f", "scope": "openid", "expires_at": expires,
		})
	}
	for i, retired := range []time.Time{now.Add(-RetiredKeyRetention - time.Minute), now.Add(-time.Hour)} {
		saveTestRecord(t, app, SigningKeysCollection, map[string]any{
			"kid": "kid-" + string(rune('a'+i)), "alg": SigningAlgorithm, "private_key": "x",
			"public_jwk": map[string]any{"kty": "RSA"}, "state": "retired", "retired_at": retired,
		})
	}

	if err := Cleanup(app, now); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, collection := range []string{AuthRequestsCollection, CodesCollection, SessionAuthTimesCollection, TokensCollection, SigningKeysCollection} {
		if n := countRecords(t, app, collection); n != 1 {
			t.Fatalf("%s rows = %d after cleanup, want 1", collection, n)
		}
	}
}
