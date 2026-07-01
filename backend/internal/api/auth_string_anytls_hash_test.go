package api

import (
	"testing"

	"hysterical-panel/internal/token"
	_ "hysterical-panel/migrations" // register the panel migrations for the test app

	"github.com/pocketbase/pocketbase/core"
)

// newMigratedTestApp spins up a PocketBase app on a throwaway data dir and applies
// every registered migration (PocketBase's base schema plus the panel's, including
// the auth_string_anytls_hash field). A fresh DB has no demo users, so the unique
// indexes our migrations add apply cleanly — unlike tests.NewTestApp's bundled fixture.
func newMigratedTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

// newUsersTestRecord builds a valid users record. auth_string_anytls_hash is left
// unset on purpose — the sync hook is expected to populate it (it's a required
// field, so a save would otherwise fail validation, which is exactly the invariant
// under test).
func newUsersTestRecord(t *testing.T, app core.App, email, authString string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("auth_string", authString)
	rec.Set("role", "user")
	rec.Set("status", "active")
	rec.Set("verified", true)
	return rec
}

// TestUserAnytlsHashSyncHook verifies that bindUserAnytlsHashSync keeps
// auth_string_anytls_hash = hex(sha256(auth_string)) on both create and update,
// and that the resulting hash is matchable the way the anytls /auth callback
// matches it.
func TestUserAnytlsHashSyncHook(t *testing.T) {
	app := newMigratedTestApp(t)

	h := &Handlers{app: app}
	h.bindUserAnytlsHashSync()

	const authString = "Abc123XyZ789"
	rec := newUsersTestRecord(t, app, "sync@example.com", authString)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save (create): %v", err)
	}

	want := token.Sha256Hex(authString)
	if got := rec.GetString("auth_string_anytls_hash"); got != want {
		t.Fatalf("after create: auth_string_anytls_hash = %q, want %q", got, want)
	}

	// The anytls callback looks up the lowercased incoming hash against
	// auth_string_anytls_hash; the stored value must be findable that way.
	found, err := app.FindFirstRecordByFilter("users", "auth_string_anytls_hash = {:a}", map[string]any{"a": want})
	if err != nil || found == nil || found.Id != rec.Id {
		t.Fatalf("lookup by auth_string_anytls_hash failed: rec=%v err=%v", found, err)
	}

	// Rotating auth_string must re-derive the hash on update.
	const rotated = "NEWkey0000"
	rec.Set("auth_string", rotated)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save (update): %v", err)
	}
	if got, want := rec.GetString("auth_string_anytls_hash"), token.Sha256Hex(rotated); got != want {
		t.Fatalf("after update: auth_string_anytls_hash = %q, want %q", got, want)
	}

	// Reload from the database to confirm the new hash was persisted, not just
	// set on the in-memory record.
	reloaded, err := app.FindRecordById("users", rec.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, want := reloaded.GetString("auth_string_anytls_hash"), token.Sha256Hex(rotated); got != want {
		t.Fatalf("persisted auth_string_anytls_hash = %q, want %q", got, want)
	}
}
