package migrations_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestBackfillTrafficDailyRebuildsRowsDeletedByOldPrune(t *testing.T) {
	app := migratedApp(t)
	userID, nodeID := seedTrafficOwner(t, app)
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	// The old prune left these hours of the cutoff day but deleted its daily row.
	saveTrafficRow(t, app, "traffic_hourly", userID, nodeID, day.Add(13*time.Hour), 5)
	saveTrafficRow(t, app, "traffic_hourly", userID, nodeID, day.Add(20*time.Hour), 7)
	// A complete day already has its daily row and must not change.
	nextDay := day.AddDate(0, 0, 1)
	saveTrafficRow(t, app, "traffic_hourly", userID, nodeID, nextDay.Add(time.Hour), 3)
	saveTrafficRow(t, app, "traffic_daily", userID, nodeID, nextDay, 3)

	runner := migrationRunnerThrough(t, app, "1730000035_backfill_traffic_daily.go")
	if _, err := runner.Down(1); err != nil {
		t.Fatalf("revert backfill: %v", err)
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply backfill: %v", err)
	}

	rows, err := app.FindRecordsByFilter("traffic_daily", "user = {:u}", "bucket", 0, 0, map[string]any{"u": userID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].GetInt("tx") != 12 || rows[1].GetInt("tx") != 3 {
		t.Fatalf("daily rows = %d, want rebuilt 12-byte day and untouched 3-byte day", len(rows))
	}
	if got := rows[0].GetDateTime("bucket").Time(); !got.Equal(day) {
		t.Fatalf("rebuilt bucket = %v, want %v", got, day)
	}
}

func seedTrafficOwner(t *testing.T, app core.App) (string, string) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.Set("email", "backfill@example.com")
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatal(err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "backfill node")
	node.Set("api_url", "http://127.0.0.1:1")
	node.Set("api_secret", "sealed")
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return user.Id, node.Id
}

func saveTrafficRow(t *testing.T, app core.App, table, userID, nodeID string, bucket time.Time, tx int64) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(table)
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("node", nodeID)
	rec.Set("bucket", bucket)
	rec.Set("tx", tx)
	rec.Set("rx", 0)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save %s: %v", table, err)
	}
}
