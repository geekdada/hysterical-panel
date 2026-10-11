package migrations_test

import "testing"

func TestLockUsersAPIRulesMigrationRollsBackAndReapplies(t *testing.T) {
	app := migratedApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	for name, rule := range map[string]*string{
		"list": users.ListRule, "view": users.ViewRule, "create": users.CreateRule,
		"update": users.UpdateRule, "delete": users.DeleteRule,
	} {
		if rule != nil {
			t.Fatalf("users %s rule = %q, want superuser-only", name, *rule)
		}
	}

	runner := migrationRunnerThrough(t, app, "1730000030_lock_users_api_rules.go")
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert users rules migration: reverted=%v err=%v", reverted, err)
	}
	users, _ = app.FindCollectionByNameOrId("users")
	if users.CreateRule == nil || *users.CreateRule != "" {
		t.Fatal("rollback did not restore the default public create rule")
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply users rules migration: %v", err)
	}
}
