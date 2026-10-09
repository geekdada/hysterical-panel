package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestUserMonitorsMigrationRollsBackAndReapplies(t *testing.T) {
	app := migratedApp(t)
	alerts, err := app.FindCollectionByNameOrId("alerts")
	if err != nil {
		t.Fatal(err)
	}
	if alerts.Fields.GetByName("user") == nil || alerts.Fields.GetByName("user_email_snapshot") == nil {
		t.Fatal("alerts lack the user subject fields")
	}
	if alerts.Fields.GetByName("node").(*core.RelationField).Required {
		t.Fatal("alerts.node must be optional for User Alerts")
	}

	runner := migrationRunnerThrough(t, app, "1730000029_add_user_monitors.go")
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert user monitors migration: reverted=%v err=%v", reverted, err)
	}
	alerts, _ = app.FindCollectionByNameOrId("alerts")
	if alerts.Fields.GetByName("user") != nil || !alerts.Fields.GetByName("node").(*core.RelationField).Required {
		t.Fatal("rollback did not restore Node-only alerts")
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply user monitors migration: %v", err)
	}
}
