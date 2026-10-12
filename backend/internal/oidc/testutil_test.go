package oidc

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
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

func newTestUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("role", "user")
	rec.Set("status", "active")
	rec.Set("verified", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return rec
}

func newTestBox(t *testing.T, masterKey string) *cryptobox.Box {
	t.Helper()
	box, err := cryptobox.New(masterKey)
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	return box
}

func saveTestRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("find %s: %v", collection, err)
	}
	rec := core.NewRecord(col)
	for key, value := range fields {
		rec.Set(key, value)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return rec
}

func countRecords(t *testing.T, app core.App, collection string) int {
	t.Helper()
	recs, err := app.FindAllRecords(collection)
	if err != nil {
		t.Fatalf("list %s: %v", collection, err)
	}
	return len(recs)
}
