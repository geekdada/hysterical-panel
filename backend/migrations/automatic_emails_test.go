package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestAutomaticEmailsMigration(t *testing.T) {
	app := migratedApp(t)
	runner := migrationRunnerThrough(t, app, "1730000039_create_automatic_emails.go")
	if _, err := runner.Down(1); err != nil {
		t.Fatalf("revert: %v", err)
	}
	legacy := saveLegacySendout(t, app)
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply: %v", err)
	}

	assertSeededTemplates(t, app)
	legacy, err := app.FindRecordById("email_sendouts", legacy.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := legacy.GetString("source"); got != "manual" {
		t.Fatalf("legacy source = %q, want manual", got)
	}
	assertOneStatePerUser(t, app)
}

func saveLegacySendout(t *testing.T, app core.App) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("email_sendouts")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(collection)
	rec.Set("subject", "Legacy")
	rec.Set("language", "en")
	rec.Set("audience", "all")
	rec.Set("html", "<p>x</p>")
	rec.Set("text", "x")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save legacy sendout: %v", err)
	}
	return rec
}

func assertSeededTemplates(t *testing.T, app core.App) {
	t.Helper()
	rows, err := app.FindAllRecords("automatic_email_templates")
	if err != nil || len(rows) != 3 {
		t.Fatalf("templates = %d, err = %v, want 3", len(rows), err)
	}
	for _, row := range rows {
		kind := row.GetString("kind")
		wantThreshold := 0
		if kind == "low_allowance" {
			wantThreshold = 10
		}
		if row.GetBool("enabled") || row.GetInt("threshold_percent") != wantThreshold {
			t.Fatalf("%s: enabled=%v threshold=%d", kind, row.GetBool("enabled"), row.GetInt("threshold_percent"))
		}
	}
}

func assertOneStatePerUser(t *testing.T, app core.App) {
	t.Helper()
	users, _ := app.FindCollectionByNameOrId("users")
	user := core.NewRecord(users)
	user.SetEmail("state@example.com")
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	states, _ := app.FindCollectionByNameOrId("automatic_email_states")
	for i, wantErr := range []bool{false, true} {
		state := core.NewRecord(states)
		state.Set("user", user.Id)
		if err := app.Save(state); (err != nil) != wantErr {
			t.Fatalf("state save %d: err = %v, wantErr %v", i, err, wantErr)
		}
	}
}
