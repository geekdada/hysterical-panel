package sendouts

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	_ "hysterical-panel/migrations"
)

func newTestApp(t *testing.T) core.App {
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

func newTestUser(t *testing.T, app core.App, email, status string, verified bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("role", "user")
	rec.Set("status", status)
	rec.Set("verified", verified)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return rec
}

func recipientsOf(t *testing.T, app core.App, sendoutID string) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter(RecipientsCollection, "sendout = {:id}", "queued_at,id", 0, 0, dbx.Params{"id": sendoutID})
	if err != nil {
		t.Fatalf("find recipients: %v", err)
	}
	return rows
}

func setRecipientStatus(t *testing.T, app core.App, row *core.Record, status, reason string) {
	t.Helper()
	row.Set("status", status)
	row.Set("reason", reason)
	if err := app.Save(row); err != nil {
		t.Fatalf("set recipient status: %v", err)
	}
}

func reload(t *testing.T, app core.App, collection, id string) *core.Record {
	t.Helper()
	rec, err := app.FindRecordById(collection, id)
	if err != nil {
		t.Fatalf("reload %s/%s: %v", collection, id, err)
	}
	return rec
}

func testDraft() Draft {
	return Draft{
		Subject:  "Scheduled maintenance",
		Language: "en",
		Audience: AudienceAll,
		HTML:     "<!DOCTYPE html><html><body><p>Maintenance tonight.</p></body></html>",
		Text:     "Maintenance tonight.",
		Content:  map[string]any{"type": "doc"},
	}
}

func newTestSendout(t *testing.T, app core.App, queuedAt time.Time, users ...*core.Record) *core.Record {
	t.Helper()
	rec, err := Create(app, testDraft(), users, nil, queuedAt)
	if err != nil {
		t.Fatalf("create sendout: %v", err)
	}
	return rec
}
