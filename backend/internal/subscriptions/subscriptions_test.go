package subscriptions_test

import (
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
