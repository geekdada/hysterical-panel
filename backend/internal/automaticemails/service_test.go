package automaticemails

import (
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/sendouts"
	_ "hysterical-panel/migrations"
)

var scanStart = time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

type countingNotifier struct{ calls int }

func (n *countingNotifier) Notify() { n.calls++ }

type fixture struct {
	t        *testing.T
	app      core.App
	service  *Service
	notifier *countingNotifier
	user     *core.Record
	grant    *core.Record
}

func newFixture(t *testing.T, allowance int64) *fixture {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	notifier := &countingNotifier{}
	f := &fixture{t: t, app: app, notifier: notifier, service: New(app, notifier, "https://panel.example")}
	f.user = newUser(t, app, "a@example.com", "active", true, true)
	f.grant = newGrant(t, app, f.user, newType(t, app, allowance), scanStart)
	for _, kind := range Kinds {
		f.setTemplate(kind, true, "["+string(kind)+"] {{email}} {{remaining_percent}}", "<p>{{email}}</p>", "{{email}}")
	}
	return f
}

func (f *fixture) setTemplate(kind Kind, enabled bool, subject, html, text string) {
	f.t.Helper()
	row, err := f.app.FindFirstRecordByData("automatic_email_templates", "kind", string(kind))
	if err != nil {
		f.t.Fatalf("find template: %v", err)
	}
	row.Set("enabled", enabled)
	row.Set("subject", subject)
	row.Set("html", html)
	row.Set("text", text)
	if err := f.app.Save(row); err != nil {
		f.t.Fatalf("save template: %v", err)
	}
}

func (f *fixture) setEnabled(kind Kind, enabled bool) {
	f.t.Helper()
	row, err := f.app.FindFirstRecordByData("automatic_email_templates", "kind", string(kind))
	if err != nil {
		f.t.Fatalf("find template: %v", err)
	}
	row.Set("enabled", enabled)
	if err := f.app.Save(row); err != nil {
		f.t.Fatalf("save template: %v", err)
	}
}

func (f *fixture) useWindow(usedTx, extra int64) {
	f.t.Helper()
	f.grant.Set("window_index", 0)
	f.grant.Set("used_tx_bytes", usedTx)
	f.grant.Set("used_rx_bytes", 0)
	f.grant.Set("extra_bytes", extra)
	if err := f.app.Save(f.grant); err != nil {
		f.t.Fatalf("save window usage: %v", err)
	}
}

func (f *fixture) scan(at time.Time) {
	f.t.Helper()
	if err := f.service.Scan(at); err != nil {
		f.t.Fatalf("scan: %v", err)
	}
}

func (f *fixture) queuedKinds() []string {
	f.t.Helper()
	rows, err := f.app.FindAllRecords(sendouts.SendoutsCollection)
	if err != nil {
		f.t.Fatalf("find sendouts: %v", err)
	}
	kinds := []string{}
	for _, row := range rows {
		kinds = append(kinds, row.GetString("automatic_kind"))
	}
	sort.Strings(kinds)
	return kinds
}

func (f *fixture) wantKinds(want ...string) {
	f.t.Helper()
	got := f.queuedKinds()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		f.t.Fatalf("queued kinds = %v, want %v", got, want)
	}
}

func newUser(t *testing.T, app core.App, email, status string, verified, subscriptionRequired bool) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", status)
	user.Set("subscription_required", subscriptionRequired)
	user.SetVerified(verified)
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

func newType(t *testing.T, app core.App, allowance int64) *core.Record {
	t.Helper()
	types, err := app.FindCollectionByNameOrId("subscription_types")
	if err != nil {
		t.Fatalf("find subscription types: %v", err)
	}
	typ := core.NewRecord(types)
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", allowance)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatalf("save subscription type: %v", err)
	}
	return typ
}

func newGrant(t *testing.T, app core.App, user, typ *core.Record, start time.Time) *core.Record {
	t.Helper()
	grants, err := app.FindCollectionByNameOrId("user_subscriptions")
	if err != nil {
		t.Fatalf("find user subscriptions: %v", err)
	}
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	return grant
}

func TestScanSendsLowOnceUntilRecovered(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	f.useWindow(95, 0)
	f.scan(now)
	f.wantKinds("low_allowance")
	f.scan(now.Add(time.Minute))
	f.wantKinds("low_allowance")

	f.useWindow(95, 15)
	f.scan(now.Add(2 * time.Minute))
	f.wantKinds("low_allowance")

	f.useWindow(110, 15)
	f.scan(now.Add(3 * time.Minute))
	f.wantKinds("low_allowance", "low_allowance")
}

func TestScanSendsOnlyExhaustedWhenBothCross(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	f.useWindow(50, 0)
	f.scan(now)
	f.wantKinds()
	f.useWindow(100, 0)
	f.scan(now.Add(time.Minute))
	f.wantKinds("allowance_exhausted")
}

func TestScanFallsBackToLowWhenExhaustedDisabled(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	f.setEnabled(KindAllowanceExhausted, false)
	f.useWindow(50, 0)
	f.scan(now)
	f.useWindow(100, 0)
	f.scan(now.Add(time.Minute))
	f.wantKinds("low_allowance")

	f.setEnabled(KindLowAllowance, false)
	f.useWindow(0, 0)
	f.scan(now.Add(2 * time.Minute))
	f.useWindow(100, 0)
	f.scan(now.Add(3 * time.Minute))
	f.wantKinds("allowance_restored", "low_allowance")
}

func TestScanSendsRestoredAfterTopUp(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	f.useWindow(100, 0)
	f.scan(now)
	f.wantKinds("allowance_exhausted")
	f.useWindow(100, 100)
	f.scan(now.Add(time.Minute))
	f.wantKinds("allowance_exhausted", "allowance_restored")
}

func TestScanSendsRestoredAtNewWindow(t *testing.T) {
	f := newFixture(t, 100)
	f.useWindow(100, 0)
	f.scan(scanStart.Add(time.Hour))
	f.wantKinds("allowance_exhausted")
	f.scan(scanStart.Add(31 * 24 * time.Hour))
	f.wantKinds("allowance_exhausted", "allowance_restored")
}

func TestScanSkipsDisabledKindAndUnusableUsers(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	typ := newType(t, f.app, 100)
	for _, u := range []*core.Record{
		newUser(t, f.app, "disabled@example.com", "disabled", true, true),
		newUser(t, f.app, "unverified@example.com", "active", false, true),
		newUser(t, f.app, "legacy@example.com", "active", true, false),
	} {
		grant := newGrant(t, f.app, u, typ, scanStart)
		grant.Set("used_tx_bytes", 100)
		if err := f.app.Save(grant); err != nil {
			t.Fatalf("save grant: %v", err)
		}
	}
	f.setEnabled(KindAllowanceExhausted, false)
	f.setEnabled(KindLowAllowance, false)
	f.useWindow(100, 0)
	f.scan(now)
	f.wantKinds()
	if f.notifier.calls != 0 {
		t.Fatalf("notify calls = %d, want 0", f.notifier.calls)
	}
	count, err := f.app.CountRecords("automatic_email_states")
	if err != nil || count != 1 {
		t.Fatalf("state rows = %d (%v), want 1", count, err)
	}
}

func TestScanTracksStateWhileKindDisabled(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	f.setEnabled(KindLowAllowance, false)
	f.useWindow(95, 0)
	f.scan(now)
	f.setEnabled(KindLowAllowance, true)
	f.scan(now.Add(time.Minute))
	f.wantKinds()
}

func TestScanRollsBackStateWhenSendoutFails(t *testing.T) {
	f := newFixture(t, 100)
	now := scanStart.Add(time.Hour)
	f.setTemplate(KindLowAllowance, true, "s", strings.Repeat("{{email}}", 100_000), strings.Repeat("{{email}}", 10))
	f.useWindow(95, 0)
	f.scan(now)
	f.wantKinds()
	if _, err := f.app.FindFirstRecordByData("automatic_email_states", "user", f.user.Id); err == nil {
		t.Fatal("state row exists after failed sendout")
	}

	f.setTemplate(KindLowAllowance, true, "s", "<p>{{email}}</p>", "{{email}}")
	f.scan(now.Add(time.Minute))
	f.wantKinds("low_allowance")
}

func TestScanRendersRecipientAndSource(t *testing.T) {
	f := newFixture(t, 100)
	f.useWindow(95, 0)
	f.scan(scanStart.Add(time.Hour))
	rows, err := f.app.FindAllRecords(sendouts.SendoutsCollection)
	if err != nil || len(rows) != 1 {
		t.Fatalf("sendouts = %d (%v), want 1", len(rows), err)
	}
	sendout := rows[0]
	if sendout.GetString("source") != "automatic" || sendout.GetString("language") != "bilingual" || sendout.GetString("audience") != "single" {
		t.Fatalf("source=%q language=%q audience=%q", sendout.GetString("source"), sendout.GetString("language"), sendout.GetString("audience"))
	}
	if sendout.GetString("subject") != "[low_allowance] a@example.com 5%" {
		t.Fatalf("subject = %q", sendout.GetString("subject"))
	}
	recipients, err := f.app.FindRecordsByFilter(sendouts.RecipientsCollection, "sendout = {:id}", "", 0, 0, dbx.Params{"id": sendout.Id})
	if err != nil || len(recipients) != 1 || recipients[0].GetString("email") != "a@example.com" {
		t.Fatalf("recipients = %v (%v)", recipients, err)
	}
	if f.notifier.calls != 1 {
		t.Fatalf("notify calls = %d, want 1", f.notifier.calls)
	}
}

func TestScanTruncatesRenderedSubjectToSendoutLimit(t *testing.T) {
	f := newFixture(t, 100)
	f.user.SetEmail(strings.Repeat("a", 60) + "@example.com")
	if err := f.app.Save(f.user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	f.setTemplate(KindLowAllowance, true, strings.Repeat("é", 190)+"{{email}}", "<p>{{email}}</p>", "{{email}}")
	f.useWindow(95, 0)
	f.scan(scanStart.Add(time.Hour))
	f.wantKinds("low_allowance")
	rows, err := f.app.FindAllRecords(sendouts.SendoutsCollection)
	if err != nil || len(rows) != 1 {
		t.Fatalf("sendouts = %d (%v)", len(rows), err)
	}
	if got := utf8.RuneCountInString(rows[0].GetString("subject")); got != sendouts.MaxSubjectRunes {
		t.Fatalf("subject runes = %d, want %d", got, sendouts.MaxSubjectRunes)
	}
}
