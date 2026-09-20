package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestSubscriptionMigrationGrandfathersExistingUsers(t *testing.T) {
	app := migratedApp(t)
	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	if _, err := runner.Down(1); err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	legacy := core.NewRecord(users)
	legacy.SetEmail("before-subscriptions@example.com")
	legacy.SetPassword("password12345")
	legacy.Set("role", "user")
	legacy.Set("status", "active")
	legacy.SetVerified(true)
	if err := app.Save(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	stored, err := app.FindRecordById("users", legacy.Id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetBool("subscription_required") {
		t.Fatal("existing User lost legacy exemption")
	}
	if stored.Collection().Fields.GetByName("quota_bytes") != nil {
		t.Fatal("unused quota_bytes field survived migration")
	}
}
