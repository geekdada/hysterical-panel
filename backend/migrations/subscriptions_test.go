package migrations_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestSubscriptionMigrationGrandfathersExistingUsers(t *testing.T) {
	app := migratedApp(t)
	runner := migrationRunnerThrough(t, app, "1730000025_create_subscriptions.go")
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

func TestSplitSubscriptionUsageMigrationRollsBackToCombinedUsage(t *testing.T) {
	app := migratedApp(t)
	grants, err := app.FindCollectionByNameOrId("user_subscriptions")
	if err != nil {
		t.Fatal(err)
	}
	if grants.Fields.GetByName("used_bytes") != nil {
		t.Fatal("combined used_bytes survived the split")
	}
	users, _ := app.FindCollectionByNameOrId("users")
	user := core.NewRecord(users)
	user.SetEmail("split-usage@example.com")
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	user.SetVerified(true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", 100)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grant := core.NewRecord(grants)
	start := time.Now().UTC().Truncate(time.Millisecond)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	grant.Set("used_tx_bytes", 30)
	grant.Set("used_rx_bytes", 12)
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}

	runner := migrationRunnerThrough(t, app, "1730000026_split_subscription_usage.go")
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert split usage migration: reverted=%v err=%v", reverted, err)
	}
	stored, err := app.FindRecordById("user_subscriptions", grant.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.GetString("used_bytes"); got != "42" {
		t.Fatalf("rolled-back used_bytes = %q; want 42", got)
	}
	for _, name := range []string{"used_tx_bytes", "used_rx_bytes", "grant_tx_bytes", "grant_rx_bytes"} {
		if stored.Collection().Fields.GetByName(name) != nil {
			t.Fatalf("rollback left %s behind", name)
		}
	}
}
