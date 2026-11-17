package uploads

import (
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

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

func newFile(t *testing.T, content, name string) *filesystem.File {
	t.Helper()
	file, err := filesystem.NewFileFromBytes([]byte(content), name)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

var storedPath = regexp.MustCompile(`^/api/files/files/[a-z0-9]{15}/report_[a-z0-9]{10}\.pdf$`)

func TestSaveStoresPublicFile(t *testing.T) {
	app := newTestApp(t)
	stored, err := Save(app, "", newFile(t, "%PDF-1.4", "Report.pdf"), "application/pdf; charset=binary")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Report.pdf" || stored.MIME != "application/pdf" || stored.Size != 8 {
		t.Errorf("stored = %+v", stored)
	}
	if !storedPath.MatchString(stored.Path) {
		t.Errorf("path = %q", stored.Path)
	}
	record, err := app.FindRecordById(Collection, stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := Path(record.Id, record.GetString("file")); got != stored.Path {
		t.Errorf("record path = %q, want %q", got, stored.Path)
	}
	assertReadable(t, app, record)
}

func assertReadable(t *testing.T, app core.App, record *core.Record) {
	t.Helper()
	fsys, err := app.NewFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	reader, err := fsys.GetReader(record.BaseFilesPath() + "/" + record.GetString("file"))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if string(body) != "%PDF-1.4" {
		t.Errorf("stored bytes = %q", body)
	}
}

func TestSaveFallsBackForUnknownMIMEAndBlankName(t *testing.T) {
	app := newTestApp(t)
	file := newFile(t, "x", "notes.txt")
	file.OriginalName = "   "
	stored, err := Save(app, "", file, "not a type")
	if err != nil {
		t.Fatal(err)
	}
	if stored.MIME != fallbackMIME {
		t.Errorf("mime = %q", stored.MIME)
	}
	if stored.Name != file.Name {
		t.Errorf("name = %q, want stored name %q", stored.Name, file.Name)
	}
}

func TestSaveCapsLongNames(t *testing.T) {
	app := newTestApp(t)
	stored, err := Save(app, "", newFile(t, "x", strings.Repeat("名", 300)+".txt"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(stored.Name)); got != maxNameRunes {
		t.Errorf("name runes = %d, want %d", got, maxNameRunes)
	}
}

func TestSaveRejectsOversizedFile(t *testing.T) {
	app := newTestApp(t)
	file := newFile(t, "x", "big.bin")
	file.Size = MaxBytes + 1
	if _, err := Save(app, "", file, ""); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestPathEscapesStoredName(t *testing.T) {
	if got := Path("abcdefghij12345", "a b.c++"); got != "/api/files/files/abcdefghij12345/a%20b.c++" {
		t.Errorf("path = %q", got)
	}
}
