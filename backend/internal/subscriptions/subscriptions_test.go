package subscriptions_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"hysterical-panel/internal/subscriptions"
	_ "hysterical-panel/migrations"
)

func TestWindowAtExactThirtyDayBoundaries(t *testing.T) {
	start := time.Date(2026, 9, 20, 15, 37, 0, 0, time.UTC)
	for _, tt := range []struct {
		at   time.Time
		want int
	}{
		{start, 0},
		{start.Add(30*24*time.Hour - time.Nanosecond), 0},
		{start.Add(30 * 24 * time.Hour), 1},
		{start.Add(359 * 24 * time.Hour), 11},
	} {
		got, ok := subscriptions.WindowAt(start, tt.at, 30)
		if !ok || got != tt.want {
			t.Errorf("WindowAt(%s) = (%d, %t), want (%d, true)", tt.at, got, ok, tt.want)
		}
	}
	if _, ok := subscriptions.WindowAt(start, start.Add(360*24*time.Hour), 30); ok {
		t.Fatal("expiry instant belongs to a subscription window")
	}
	if _, ok := subscriptions.WindowAt(start, start.Add(-time.Second), 30); ok {
		t.Fatal("time before grant belongs to a subscription window")
	}
	if got, ok := subscriptions.WindowAt(start, start.Add(359*24*time.Hour), 360); !ok || got != 0 {
		t.Fatalf("annual window = (%d, %t), want (0, true)", got, ok)
	}
}

// newGrantFixture creates a metered User with one grant of a new Type.
func newGrantFixture(t *testing.T, allowance int64, resetDays int, start time.Time) (core.App, *core.Record, *core.Record) {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	users, _ := app.FindCollectionByNameOrId("users")
	user := core.NewRecord(users)
	user.SetEmail("subscribed@example.com")
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	user.SetVerified(true)
	user.Set("subscription_required", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Fixture")
	typ.Set("allowance_bytes", allowance)
	typ.Set("reset_days", resetDays)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	return app, user, grant
}

func TestGrantAllowanceResetsAtWindowBoundary(t *testing.T) {
	start := time.Date(2026, 9, 20, 15, 37, 0, 0, time.UTC)
	app, user, _ := newGrantFixture(t, 100, 30, start)

	if exhausted, err := subscriptions.AddUsage(app, user.Id, start.Add(time.Hour), 80, 40); err != nil || !exhausted {
		t.Fatalf("first poll exhausted = %t, err = %v", exhausted, err)
	}
	state, err := subscriptions.Current(app, user.Id, start.Add(2*time.Hour))
	if err != nil || state == nil || state.Remaining != -20 {
		t.Fatalf("remaining = %v, err = %v; want -20", state, err)
	}
	allowed, err := subscriptions.Allowed(app, user, start.Add(2*time.Hour))
	if err != nil || allowed {
		t.Fatalf("exhausted access = %t, err = %v", allowed, err)
	}
	allowed, err = subscriptions.Allowed(app, user, start.Add(30*24*time.Hour))
	if err != nil || !allowed {
		t.Fatalf("next window access = %t, err = %v", allowed, err)
	}
	if exhausted, err := subscriptions.AddUsage(app, user.Id, start.Add(30*24*time.Hour), 3, 4); err != nil || exhausted {
		t.Fatalf("next window poll exhausted = %t, err = %v", exhausted, err)
	}
	state, err = subscriptions.Current(app, user.Id, start.Add(30*24*time.Hour))
	if err != nil || state == nil || state.Used != (subscriptions.Usage{Tx: 3, Rx: 4}) || state.Remaining != 93 {
		t.Fatalf("next window state = %+v, err = %v", state, err)
	}
	allowed, err = subscriptions.Allowed(app, user, start.Add(360*24*time.Hour))
	if err != nil || allowed {
		t.Fatalf("expired access = %t, err = %v", allowed, err)
	}
}

func TestGrantUsageAccumulatesAcrossWindows(t *testing.T) {
	start := time.Date(2026, 9, 20, 15, 37, 0, 0, time.UTC)
	app, user, grant := newGrantFixture(t, 100, 30, start)

	for i, at := range []time.Time{start, start.Add(30 * 24 * time.Hour), start.Add(95 * 24 * time.Hour)} {
		if _, err := subscriptions.AddUsage(app, user.Id, at, int64(10*(i+1)), int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := app.FindRecordById("user_subscriptions", grant.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := subscriptions.GrantUsage(stored); got != (subscriptions.Usage{Tx: 60, Rx: 6}) {
		t.Fatalf("grant usage = %+v; want 60/6", got)
	}
	used, _ := subscriptions.WindowUsage(stored, 3)
	if used != (subscriptions.Usage{Tx: 30, Rx: 3}) {
		t.Fatalf("window 3 usage = %+v; want 30/3", used)
	}
	if used, extra := subscriptions.WindowUsage(stored, 4); used != (subscriptions.Usage{}) || extra != 0 {
		t.Fatalf("unwritten window 4 = %+v extra %d; want empty", used, extra)
	}
}

func TestAddUsageRejectsDirectionSumOverflow(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Millisecond)
	app, user, grant := newGrantFixture(t, math.MaxInt64, 360, start)

	if _, err := subscriptions.AddUsage(app, user.Id, start, math.MaxInt64-1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptions.AddUsage(app, user.Id, start, 0, 2); err == nil {
		t.Fatal("usage whose tx+rx overflows int64 was accepted")
	}
	stored, err := app.FindRecordById("user_subscriptions", grant.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := subscriptions.GrantUsage(stored); got != (subscriptions.Usage{Tx: math.MaxInt64 - 1}) {
		t.Fatalf("grant usage after rejected poll = %+v", got)
	}
}

func TestSubscriptionUsagePreservesInt64BytePrecision(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Millisecond)
	app, user, grant := newGrantFixture(t, int64(1<<53)+3, 360, start)

	if exhausted, err := subscriptions.AddUsage(app, user.Id, start, 1<<53, 1); err != nil || exhausted {
		t.Fatalf("large usage = %t, err = %v", exhausted, err)
	}
	state, err := subscriptions.Current(app, user.Id, start)
	if err != nil || state == nil || state.Used.Tx != 1<<53 || state.Remaining != 2 {
		t.Fatalf("large usage state = %+v, err = %v", state, err)
	}
	stored, err := app.FindRecordById("user_subscriptions", grant.Id)
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"used_tx_bytes": "9007199254740992", "used_rx_bytes": "1", "grant_tx_bytes": "9007199254740992", "grant_rx_bytes": "1"} {
		if got := stored.GetString(field); got != want {
			t.Fatalf("stored %s = %q; want %q", field, got, want)
		}
	}
}

func addGrant(t *testing.T, app core.App, user *core.Record, typeID string, start, terminated time.Time) *core.Record {
	t.Helper()
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typeID)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	if !terminated.IsZero() {
		grant.Set("terminated_at", terminated)
	}
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	return grant
}

func reloadGrant(t *testing.T, app core.App, id string) *core.Record {
	t.Helper()
	grant, err := app.FindRecordById("user_subscriptions", id)
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func TestRescheduleMovesQueuedGrantAndKeepsWindowUsage(t *testing.T) {
	day := 24 * time.Hour
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	start := now.Add(-10 * day)
	app, user, current := newGrantFixture(t, 100, 30, start)
	queued := addGrant(t, app, user, current.GetString("subscription_type"), start.Add(360*day), time.Time{})
	if _, err := subscriptions.AddUsage(app, user.Id, now, 30, 10); err != nil {
		t.Fatal(err)
	}
	current = reloadGrant(t, app, current.Id)
	subscriptions.SetWindow(current, 0, subscriptions.Usage{Tx: 30, Rx: 10}, 25)
	if err := app.Save(current); err != nil {
		t.Fatal(err)
	}

	// 35 days earlier puts now in window 1 of the rescheduled grant.
	newStart := start.Add(-35 * day)
	if err := subscriptions.Reschedule(app, user.Id, current.Id, newStart, now); err != nil {
		t.Fatal(err)
	}
	state, err := subscriptions.Current(app, user.Id, now)
	if err != nil || state == nil {
		t.Fatalf("current after reschedule = %v, err = %v", state, err)
	}
	if !state.Grant.GetDateTime("starts_at").Time().Equal(newStart) || !state.Grant.GetDateTime("ends_at").Time().Equal(newStart.Add(360*day)) {
		t.Fatalf("rescheduled interval = %s..%s", state.Grant.GetString("starts_at"), state.Grant.GetString("ends_at"))
	}
	if state.Window != 1 || state.Used != (subscriptions.Usage{Tx: 30, Rx: 10}) || state.Allowance != 125 {
		t.Fatalf("rescheduled window = %d used %+v allowance %d; want window 1, 30/10, 125", state.Window, state.Used, state.Allowance)
	}
	if got := subscriptions.GrantUsage(state.Grant); got != (subscriptions.Usage{Tx: 30, Rx: 10}) {
		t.Fatalf("grant usage = %+v; want 30/10", got)
	}
	queued = reloadGrant(t, app, queued.Id)
	if !queued.GetDateTime("starts_at").Time().Equal(newStart.Add(360 * day)) {
		t.Fatalf("queued start = %s; want back to back with rescheduled grant", queued.GetString("starts_at"))
	}
}

func TestRescheduleRejectsStartsOutsideCurrentCoverage(t *testing.T) {
	day := 24 * time.Hour
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	start := now.Add(-10 * day)
	app, user, current := newGrantFixture(t, 100, 30, start)
	queued := addGrant(t, app, user, current.GetString("subscription_type"), start.Add(360*day), time.Time{})

	for _, tt := range []struct {
		name    string
		grantID string
		start   time.Time
		want    error
	}{
		{"future start", current.Id, now.Add(time.Millisecond), subscriptions.ErrRescheduleFuture},
		{"ends at now", current.Id, now.Add(-360 * day), subscriptions.ErrRescheduleEnded},
		{"queued grant", queued.Id, now, subscriptions.ErrRescheduleNotCurrent},
	} {
		if err := subscriptions.Reschedule(app, user.Id, tt.grantID, tt.start, now); !errors.Is(err, tt.want) {
			t.Fatalf("%s: err = %v; want %v", tt.name, err, tt.want)
		}
	}
	if err := subscriptions.Reschedule(app, user.Id, current.Id, now, now); err != nil {
		t.Fatalf("start at now: %v", err)
	}
	if err := subscriptions.Reschedule(app, user.Id, current.Id, now.Add(-360*day+time.Millisecond), now); err != nil {
		t.Fatalf("start just inside coverage: %v", err)
	}
}

func TestRescheduleCannotOverlapPreviousExpiredGrant(t *testing.T) {
	day := 24 * time.Hour
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	previousStart := now.Add(-400 * day)
	app, user, previous := newGrantFixture(t, 100, 30, previousStart)
	typeID := previous.GetString("subscription_type")
	previousEnd := previousStart.Add(360 * day)
	current := addGrant(t, app, user, typeID, now.Add(-day), time.Time{})

	if err := subscriptions.Reschedule(app, user.Id, current.Id, previousEnd.Add(-time.Millisecond), now); !errors.Is(err, subscriptions.ErrRescheduleOverlap) {
		t.Fatalf("overlap with expired grant err = %v", err)
	}
	if err := subscriptions.Reschedule(app, user.Id, current.Id, previousEnd, now); err != nil {
		t.Fatalf("start at previous expiry: %v", err)
	}
}

func TestRescheduleMayOverlapTerminatedGrants(t *testing.T) {
	day := 24 * time.Hour
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	// Granted by mistake 20 days ago and terminated yesterday.
	app, user, mistaken := newGrantFixture(t, 100, 30, now.Add(-20*day))
	mistaken.Set("terminated_at", now.Add(-day))
	if err := app.Save(mistaken); err != nil {
		t.Fatal(err)
	}
	typeID := mistaken.GetString("subscription_type")
	// Cancelled while queued, so it never covered any time.
	addGrant(t, app, user, typeID, now.Add(-10*day), now.Add(-15*day))
	current := addGrant(t, app, user, typeID, now.Add(-time.Hour), time.Time{})

	if err := subscriptions.Reschedule(app, user.Id, current.Id, now.Add(-30*day), now); err != nil {
		t.Fatalf("start before terminated grants: %v", err)
	}
}
