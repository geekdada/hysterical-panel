package migrations_test

import "testing"

const defaultAppNameMigration = "1730000031_default_app_name.go"

func TestDefaultAppNameMigrationReplacesPocketBasePlaceholder(t *testing.T) {
	app := migratedApp(t)
	if got := app.Settings().Meta.AppName; got != "Hysterical Panel" {
		t.Fatalf("app name = %q, want Hysterical Panel", got)
	}

	runner := migrationRunnerThrough(t, app, defaultAppNameMigration)
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert app name migration: reverted=%v err=%v", reverted, err)
	}
	if got := app.Settings().Meta.AppName; got != "Acme" {
		t.Fatalf("rollback app name = %q, want Acme", got)
	}
}

func TestDefaultAppNameMigrationKeepsCustomName(t *testing.T) {
	app := migratedApp(t)
	runner := migrationRunnerThrough(t, app, defaultAppNameMigration)
	if _, err := runner.Down(1); err != nil {
		t.Fatalf("revert app name migration: %v", err)
	}
	settings := app.Settings()
	settings.Meta.AppName = "Acme VPN"
	if err := app.Save(settings); err != nil {
		t.Fatalf("save custom app name: %v", err)
	}

	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply app name migration: %v", err)
	}
	if got := app.Settings().Meta.AppName; got != "Acme VPN" {
		t.Fatalf("app name = %q, want custom name kept", got)
	}
}
