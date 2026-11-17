package announcements

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

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

func ptr[T any](v T) *T { return &v }

var validContent = json.RawMessage(`{"type":"doc","content":[]}`)

func validInput(title string) Input {
	return Input{Title: title, Body: Body{HTML: "<p>Hello</p>", Content: validContent}}
}

func TestResolveFlags(t *testing.T) {
	cases := []struct {
		name           string
		current        Flags
		pinned, hidden *bool
		want           Flags
		wantErr        error
	}{
		{name: "pin visible", pinned: ptr(true), want: Flags{Pinned: true}},
		{name: "hide unpins", current: Flags{Pinned: true}, hidden: ptr(true), want: Flags{Hidden: true}},
		{name: "pin hidden refused", current: Flags{Hidden: true}, pinned: ptr(true), wantErr: ErrPinnedHidden},
		{name: "pin and hide together refused", pinned: ptr(true), hidden: ptr(true), wantErr: ErrPinnedHidden},
		{name: "unhide does not repin", current: Flags{Hidden: true}, hidden: ptr(false), want: Flags{}},
		{name: "unhide and pin together", current: Flags{Hidden: true}, pinned: ptr(true), hidden: ptr(false), want: Flags{Pinned: true}},
		{name: "unpin hidden allowed", current: Flags{Hidden: true}, pinned: ptr(false), want: Flags{Hidden: true}},
		{name: "no change", current: Flags{Pinned: true}, want: Flags{Pinned: true}},
	}
	for _, tc := range cases {
		got, err := ResolveFlags(tc.current, tc.pinned, tc.hidden)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: flags = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestCreateValidates(t *testing.T) {
	app := newTestApp(t)
	cases := map[string]struct {
		in   Input
		want error
	}{
		"blank title":   {Input{Title: "  ", Body: Body{HTML: "<p>x</p>", Content: validContent}}, ErrTitleRequired},
		"long title":    {Input{Title: strings.Repeat("字", MaxTitleRunes+1), Body: Body{HTML: "<p>x</p>", Content: validContent}}, ErrTitleTooLong},
		"empty body":    {Input{Title: "t", Body: Body{HTML: "<p><script>x</script></p>", Content: validContent}}, ErrBodyEmpty},
		"huge body":     {Input{Title: "t", Body: Body{HTML: "<p>" + strings.Repeat("a", MaxHTMLBytes) + "</p>", Content: validContent}}, ErrBodyTooLarge},
		"array content": {Input{Title: "t", Body: Body{HTML: "<p>x</p>", Content: json.RawMessage(`[]`)}}, ErrContentInvalid},
		"no content":    {Input{Title: "t", Body: Body{HTML: "<p>x</p>"}}, ErrContentInvalid},
		"pinned hidden": {Input{Title: "t", Body: Body{HTML: "<p>x</p>", Content: validContent}, Flags: Flags{Pinned: true, Hidden: true}}, ErrPinnedHidden},
	}
	for name, tc := range cases {
		if _, err := Create(app, "", tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestCreateStoresSanitizedBody(t *testing.T) {
	app := newTestApp(t)
	in := validInput("  Maintenance  ")
	in.Body.HTML = `<p onclick="x()">Hello</p><script>alert(1)</script>`
	record, err := Create(app, "", in)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := app.FindRecordById(Collection, record.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.GetString("title"); got != "Maintenance" {
		t.Errorf("title = %q", got)
	}
	if got := stored.GetString("html"); got != "<p>Hello</p>" {
		t.Errorf("html = %q", got)
	}
	if got, _ := stored.Get("content").(types.JSONRaw); string(got) != string(validContent) {
		t.Errorf("content = %s", got)
	}
}

func TestUpdateHidingUnpins(t *testing.T) {
	app := newTestApp(t)
	in := validInput("Pinned")
	in.Flags.Pinned = true
	record, err := Create(app, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if err := Update(app, record, Patch{Hidden: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	stored, _ := app.FindRecordById(Collection, record.Id)
	if got := FlagsOf(stored); got != (Flags{Hidden: true}) {
		t.Fatalf("flags = %+v, want hidden and unpinned", got)
	}
	if err := Update(app, stored, Patch{Pinned: ptr(true)}); !errors.Is(err, ErrPinnedHidden) {
		t.Fatalf("pin hidden err = %v", err)
	}
}

func TestUpdateRejectsInvalidPatchWithoutSaving(t *testing.T) {
	app := newTestApp(t)
	record, err := Create(app, "", validInput("Original"))
	if err != nil {
		t.Fatal(err)
	}
	err = Update(app, record, Patch{Title: ptr("Changed"), Body: &Body{HTML: "<p></p>", Content: validContent}})
	if !errors.Is(err, ErrBodyEmpty) {
		t.Fatalf("err = %v, want ErrBodyEmpty", err)
	}
	stored, _ := app.FindRecordById(Collection, record.Id)
	if got := stored.GetString("title"); got != "Original" {
		t.Fatalf("title saved despite error: %q", got)
	}
}

func TestReaderQueriesHideHiddenAndSortNewestFirst(t *testing.T) {
	app := newTestApp(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := createAt(t, app, "older", Flags{Pinned: true}, base)
	newer := createAt(t, app, "newer", Flags{Pinned: true}, base.Add(time.Hour))
	hidden := createAt(t, app, "hidden", Flags{Hidden: true}, base.Add(2*time.Hour))
	plain := createAt(t, app, "plain", Flags{}, base.Add(3*time.Hour))

	pinned, err := ListPinned(app)
	if err != nil {
		t.Fatal(err)
	}
	assertIDs(t, "pinned", pinned, newer, older)

	page, total, err := ListPublished(app, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	assertIDs(t, "page 1", page, plain, newer)
	page, _, _ = ListPublished(app, 2, 2)
	assertIDs(t, "page 2", page, older)

	all, _ := ListAll(app)
	assertIDs(t, "all", all, plain, hidden, newer, older)

	if _, err := FindPublished(app, hidden.Id); err == nil {
		t.Error("hidden announcement found by reader lookup")
	}
	if _, err := FindPublished(app, plain.Id); err != nil {
		t.Errorf("visible announcement not found: %v", err)
	}
}

func TestHasPublished(t *testing.T) {
	app := newTestApp(t)
	createAt(t, app, "hidden", Flags{Hidden: true}, time.Now().UTC())
	if ok, err := HasPublished(app); err != nil || ok {
		t.Fatalf("HasPublished = %v, %v with only hidden announcements", ok, err)
	}
	createAt(t, app, "visible", Flags{}, time.Now().UTC())
	if ok, err := HasPublished(app); err != nil || !ok {
		t.Fatalf("HasPublished = %v, %v with a visible announcement", ok, err)
	}
}

func createAt(t *testing.T, app core.App, title string, flags Flags, created time.Time) *core.Record {
	t.Helper()
	in := validInput(title)
	in.Flags = flags
	record, err := Create(app, "", in)
	if err != nil {
		t.Fatalf("create %s: %v", title, err)
	}
	// Autodate fields ignore Set on save, so write the timestamp directly.
	stamp, _ := types.ParseDateTime(created)
	if _, err := app.DB().NewQuery("UPDATE announcements SET created = {:c} WHERE id = {:id}").
		Bind(map[string]any{"c": stamp.String(), "id": record.Id}).Execute(); err != nil {
		t.Fatalf("set created: %v", err)
	}
	return record
}

func assertIDs(t *testing.T, label string, got []*core.Record, want ...*core.Record) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d records, want %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i].Id != want[i].Id {
			t.Errorf("%s[%d] = %s, want %s", label, i, got[i].GetString("title"), want[i].GetString("title"))
		}
	}
}
