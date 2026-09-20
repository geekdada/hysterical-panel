package subscriptions_test

import (
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

func TestGrantAllowanceResetsAtWindowBoundary(t *testing.T) {
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
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", 100)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	grant := core.NewRecord(grants)
	start := time.Date(2026, 9, 20, 15, 37, 0, 0, time.UTC)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}

	if exhausted, err := subscriptions.AddUsage(app, user.Id, start.Add(time.Hour), 120); err != nil || !exhausted {
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
	if exhausted, err := subscriptions.AddUsage(app, user.Id, start.Add(30*24*time.Hour), 7); err != nil || exhausted {
		t.Fatalf("next window poll exhausted = %t, err = %v", exhausted, err)
	}
	state, err = subscriptions.Current(app, user.Id, start.Add(30*24*time.Hour))
	if err != nil || state == nil || state.Used != 7 || state.Remaining != 93 {
		t.Fatalf("next window state = %v, err = %v", state, err)
	}
	allowed, err = subscriptions.Allowed(app, user, start.Add(360*24*time.Hour))
	if err != nil || allowed {
		t.Fatalf("expired access = %t, err = %v", allowed, err)
	}
}

func TestSubscriptionUsagePreservesInt64BytePrecision(t *testing.T) {
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
	user.SetEmail("exact-allowance@example.com")
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
	typ.Set("name", "Exact")
	typ.Set("allowance_bytes", int64(1<<53)+3)
	typ.Set("reset_days", 360)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	grant := core.NewRecord(grants)
	start := time.Now().UTC().Truncate(time.Millisecond)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	if exhausted, err := subscriptions.AddUsage(app, user.Id, start, 1<<53); err != nil || exhausted {
		t.Fatalf("large usage = %t, err = %v", exhausted, err)
	}
	state, err := subscriptions.Current(app, user.Id, start)
	if err != nil || state == nil || state.Used != 1<<53 || state.Remaining != 3 {
		t.Fatalf("large usage state = %v, err = %v", state, err)
	}
	stored, err := app.FindRecordById("user_subscriptions", grant.Id)
	if err != nil || stored.GetString("used_bytes") != "9007199254740992" {
		t.Fatalf("stored usage = %v, err = %v", stored, err)
	}
}
