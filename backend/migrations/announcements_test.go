package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

const announcementsMigration = "1730000040_create_announcements.go"

func TestAnnouncementsMigrationCreatesLockedCollections(t *testing.T) {
	app := migratedApp(t)
	for _, name := range []string{"announcements", "files"} {
		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		rules := []*string{collection.ListRule, collection.ViewRule, collection.CreateRule, collection.UpdateRule, collection.DeleteRule}
		for _, rule := range rules {
			if rule != nil {
				t.Errorf("%s has a non-nil API rule %q", name, *rule)
			}
		}
	}
	assertPublicFileField(t, app)
}

func assertPublicFileField(t *testing.T, app core.App) {
	t.Helper()
	files, err := app.FindCollectionByNameOrId("files")
	if err != nil {
		t.Fatal(err)
	}
	field, ok := files.Fields.GetByName("file").(*core.FileField)
	if !ok {
		t.Fatal("files.file is not a file field")
	}
	if field.Protected {
		t.Error("files.file is protected, CDN caching needs it public")
	}
	if field.MaxSize != 1<<30 || field.MaxSelect != 1 {
		t.Errorf("files.file MaxSize = %d, MaxSelect = %d, want 1 GiB and 1", field.MaxSize, field.MaxSelect)
	}
}

func TestAnnouncementsMigrationRollsBack(t *testing.T) {
	app := migratedApp(t)
	runner := migrationRunnerThrough(t, app, announcementsMigration)
	if _, err := runner.Down(1); err != nil {
		t.Fatalf("revert: %v", err)
	}
	for _, name := range []string{"announcements", "files"} {
		if _, err := app.FindCollectionByNameOrId(name); err == nil {
			t.Errorf("%s still exists after rollback", name)
		}
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}
