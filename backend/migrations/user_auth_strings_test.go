package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/token"
	_ "hysterical-panel/migrations"
)

func migratedApp(t *testing.T) core.App {
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

// authStringsMigrationRunner scopes Down/Up to the user_auth_strings migration
// so tests keep working when newer migrations are appended after it.
func authStringsMigrationRunner(t *testing.T, app core.App) *core.MigrationsRunner {
	t.Helper()
	const target = "1730000023_create_user_auth_strings.go"
	var list core.MigrationsList
	for _, m := range core.AppMigrations.Items() {
		list.Add(m)
		if m.File == target {
			return core.NewMigrationsRunner(app, list)
		}
	}
	t.Fatalf("migration %s not registered", target)
	return nil
}

func TestUserAuthStringsMigrationBackfillsAndRollsBackCurrentCredential(t *testing.T) {
	app := migratedApp(t)
	runner := authStringsMigrationRunner(t, app)
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert auth string migration: reverted=%v err=%v", reverted, err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	user := core.NewRecord(users)
	user.SetEmail("legacy@example.com")
	user.SetPassword("password12345")
	user.SetVerified(true)
	user.Set("role", "user")
	user.Set("status", "active")
	user.Set("auth_string", "LegacySecret")
	user.Set("auth_string_anytls_hash", "0000000000000000000000000000000000000000000000000000000000000000")
	if err := app.Save(user); err != nil {
		t.Fatalf("save legacy user: %v", err)
	}

	if applied, err := runner.Up(); err != nil || len(applied) != 1 {
		t.Fatalf("apply auth string migration: applied=%v err=%v", applied, err)
	}
	current, err := authstrings.CurrentForUser(app, user.Id)
	if err != nil {
		t.Fatalf("find backfilled current credential: %v", err)
	}
	if current.GetString("auth_string") != "LegacySecret" || current.GetString("auth_string_anytls_hash") != token.Sha256Hex("LegacySecret") {
		t.Fatalf("backfilled credential = auth:%q hash:%q", current.GetString("auth_string"), current.GetString("auth_string_anytls_hash"))
	}

	if _, err := runner.Down(1); err != nil {
		t.Fatalf("rollback auth string migration: %v", err)
	}
	reloaded, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatalf("reload rolled-back user: %v", err)
	}
	if got := reloaded.GetString("auth_string"); got != "LegacySecret" {
		t.Fatalf("restored auth_string = %q, want LegacySecret", got)
	}
}

func TestUserAuthStringsMigrationRejectsAuthStringUserIDCollision(t *testing.T) {
	app := migratedApp(t)
	runner := authStringsMigrationRunner(t, app)
	if _, err := runner.Down(1); err != nil {
		t.Fatalf("revert auth string migration: %v", err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	first := legacyUserRecord(users, "first-collision@example.com", "FirstLegacySecret")
	if err := app.Save(first); err != nil {
		t.Fatalf("save first user: %v", err)
	}
	second := legacyUserRecord(users, "second-collision@example.com", first.Id)
	if err := app.Save(second); err != nil {
		t.Fatalf("save second user: %v", err)
	}
	if _, err := runner.Up(); err == nil {
		t.Fatal("migration accepted an Auth String equal to a User ID")
	}
}

func legacyUserRecord(users *core.Collection, email, authString string) *core.Record {
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("password12345")
	user.SetVerified(true)
	user.Set("role", "user")
	user.Set("status", "active")
	user.Set("auth_string", authString)
	user.Set("auth_string_anytls_hash", token.Sha256Hex(authString))
	return user
}

func TestUserAuthStringsRejectsSecondCurrentAndHistoricalReuse(t *testing.T) {
	app := migratedApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	user := core.NewRecord(users)
	user.SetEmail("constraints@example.com")
	user.SetPassword("password12345")
	user.SetVerified(true)
	user.Set("role", "user")
	user.Set("status", "active")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	if _, err := authstrings.CreateCurrent(app, user.Id, "FirstSecret"); err != nil {
		t.Fatalf("create first current: %v", err)
	}
	if _, err := authstrings.CreateCurrent(app, user.Id, "SecondSecret"); err == nil {
		t.Fatal("second Current Auth String was accepted")
	}
	other := core.NewRecord(users)
	other.SetEmail("other-constraints@example.com")
	other.SetPassword("password12345")
	other.SetVerified(true)
	other.Set("role", "user")
	other.Set("status", "active")
	if err := app.Save(other); err != nil {
		t.Fatalf("save other user: %v", err)
	}
	if _, err := authstrings.CreateCurrent(app, other.Id, user.Id); err == nil {
		t.Fatal("Auth String equal to a User ID was accepted")
	}
	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "NextSecret")
		return err
	}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "FirstSecret")
		return err
	}); err == nil {
		t.Fatal("retired Auth String was reused")
	}
}

func TestUserAuthStringsOwnNodeCredentials(t *testing.T) {
	app := migratedApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	if users.Fields.GetByName("auth_string") != nil || users.Fields.GetByName("auth_string_anytls_hash") != nil {
		t.Fatal("users still owns node credential fields")
	}

	authStrings, err := app.FindCollectionByNameOrId("user_auth_strings")
	if err != nil {
		t.Fatalf("find user_auth_strings: %v", err)
	}
	for _, field := range []string{"user", "auth_string", "auth_string_anytls_hash", "state"} {
		if authStrings.Fields.GetByName(field) == nil {
			t.Fatalf("user_auth_strings.%s is missing", field)
		}
	}
}
