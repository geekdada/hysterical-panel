package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

var oidcHiddenFields = map[string][]string{
	"oidc_clients":            {"secret_hash"},
	"oidc_auth_requests":      {"request_hash"},
	"oidc_codes":              {"code_hash"},
	"oidc_grants":             nil,
	"oidc_tokens":             {"token_hash"},
	"oidc_signing_keys":       {"private_key"},
	"oidc_session_auth_times": {"token_hash"},
}

func TestOIDCProviderMigrationCreatesLockedCollections(t *testing.T) {
	app := migratedApp(t)
	for name, hidden := range oidcHiddenFields {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		for rule, value := range map[string]*string{
			"list": c.ListRule, "view": c.ViewRule, "create": c.CreateRule,
			"update": c.UpdateRule, "delete": c.DeleteRule,
		} {
			if value != nil {
				t.Fatalf("%s %s rule = %q, want superuser-only", name, rule, *value)
			}
		}
		for _, field := range hidden {
			f := c.Fields.GetByName(field)
			if f == nil || !f.GetHidden() {
				t.Fatalf("%s.%s must exist and be hidden", name, field)
			}
		}
	}

	runner := migrationRunnerThrough(t, app, "1730000032_create_oidc_provider.go")
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert oidc migration: reverted=%v err=%v", reverted, err)
	}
	for name := range oidcHiddenFields {
		if _, err := app.FindCollectionByNameOrId(name); err == nil {
			t.Fatalf("%s still exists after rollback", name)
		}
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply oidc migration: %v", err)
	}
}

func TestOIDCGrantsAllowOneActiveGrantPerUserAndClient(t *testing.T) {
	app := migratedApp(t)
	user := newOIDCMigrationUser(t, app)
	client := saveOIDCRecord(t, app, "oidc_clients", map[string]any{
		"client_id": "client-a", "name": "App", "secret_hash": "x",
		"redirect_uris": []string{"https://app.example/cb"}, "allowed_scopes": []string{"openid"},
	})
	grant := map[string]any{"user": user.Id, "client": client.Id, "scopes": []string{"openid"}}
	first := saveOIDCRecord(t, app, "oidc_grants", grant)
	if err := saveOIDCRecordErr(app, "oidc_grants", grant); err == nil {
		t.Fatal("second active grant for the same user and client was accepted")
	}
	first.Set("revoked_at", "2026-10-12 00:00:00.000Z")
	if err := app.Save(first); err != nil {
		t.Fatalf("revoke first grant: %v", err)
	}
	saveOIDCRecord(t, app, "oidc_grants", grant)
}

func TestOIDCSigningKeysAllowOneActiveKey(t *testing.T) {
	app := migratedApp(t)
	key := map[string]any{"alg": "RS256", "private_key": "x", "public_jwk": map[string]any{"kty": "RSA"}, "state": "active"}
	key["kid"] = "kid-1"
	saveOIDCRecord(t, app, "oidc_signing_keys", key)
	key["kid"] = "kid-2"
	if err := saveOIDCRecordErr(app, "oidc_signing_keys", key); err == nil {
		t.Fatal("a second active signing key was accepted")
	}
	key["state"] = "retired"
	saveOIDCRecord(t, app, "oidc_signing_keys", key)
}

func newOIDCMigrationUser(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(col)
	user.Set("email", "oidc-migration@example.com")
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

func saveOIDCRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	rec, err := buildOIDCRecord(app, collection, fields)
	if err == nil {
		err = app.Save(rec)
	}
	if err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return rec
}

func saveOIDCRecordErr(app core.App, collection string, fields map[string]any) error {
	rec, err := buildOIDCRecord(app, collection, fields)
	if err != nil {
		return err
	}
	return app.Save(rec)
}

func buildOIDCRecord(app core.App, collection string, fields map[string]any) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	for key, value := range fields {
		rec.Set(key, value)
	}
	return rec, nil
}
