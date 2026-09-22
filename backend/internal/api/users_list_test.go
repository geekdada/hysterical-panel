package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/subscriptions"
)

func TestClampUserListPage(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 1},
		{"1", 1},
		{"3", 3},
		{"0", 1},
		{"-1", 1},
		{"abc", 1},
	}
	for _, c := range cases {
		if got := clampUserListPage(c.raw); got != c.want {
			t.Errorf("clampUserListPage(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}

func TestClampUserListPerPage(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 25},
		{"25", 25},
		{"50", 50},
		{"100", 100},
		{"30", 25},
		{"40", 50},
		{"75", 50},
		{"90", 100},
		{"abc", 25},
	}
	for _, c := range cases {
		if got := clampUserListPerPage(c.raw); got != c.want {
			t.Errorf("clampUserListPerPage(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}

func TestNormalizeUserListSort(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", "created"},
		{"created", "created"},
		{"-email", "-email"},
		{"bad", "created"},
		// Lifetime usage is not part of list rows, so it cannot order them.
		{"used_tx", "created"},
		{"-used_rx", "created"},
		{"-subscription_used", "-subscription_used"},
		{"subscription_rx", "subscription_rx"},
	}
	for _, c := range cases {
		if got := normalizeUserListSort(c.raw); got != c.want {
			t.Errorf("normalizeUserListSort(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestBuildUserListFilter(t *testing.T) {
	filter, params := buildUserListFilter("", "")
	if filter != "" || params != nil {
		t.Fatalf("empty search: filter=%q params=%v", filter, params)
	}

	filter, params = buildUserListFilter("  alice  ", "user123")
	wantFilter := "(email ~ {:like} || role ~ {:like} || status ~ {:like} || id = {:search} || id = {:auth_user})"
	if filter != wantFilter {
		t.Fatalf("filter = %q, want %q", filter, wantFilter)
	}
	if params["like"] != "%alice%" || params["search"] != "alice" || params["auth_user"] != "user123" {
		t.Fatalf("params = %#v, want like=%%alice%% search=alice auth_user=user123", params)
	}
}

func TestBuildUserListFilterExactID(t *testing.T) {
	filter, params := buildUserListFilter("abc123xyz", "")
	wantFilter := "(email ~ {:like} || role ~ {:like} || status ~ {:like} || id = {:search} || id = {:auth_user})"
	if filter != wantFilter {
		t.Fatalf("filter = %q, want %q", filter, wantFilter)
	}
	if params["search"] != "abc123xyz" || params["auth_user"] != "" {
		t.Fatalf("params = %#v, want search=abc123xyz auth_user=''", params)
	}
}

func TestParseUserListQuery(t *testing.T) {
	q := parseUserListQuery("2", "50", " foo ", "-email")
	if q.Page != 2 || q.PerPage != 50 || q.Search != "foo" || q.Sort != "-email" {
		t.Fatalf("parseUserListQuery() = %+v", q)
	}
}

func TestBuildUserListFilterParamsType(t *testing.T) {
	_, params := buildUserListFilter("x", "")
	if params == nil || params["like"] == nil || params["auth_user"] == nil {
		t.Fatalf("params = %#v", params)
	}
}

func TestCurrentSubscriptionsByUserKeepsOnlyGrantCoveringNow(t *testing.T) {
	app := newMigratedTestApp(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	day := 24 * time.Hour
	current := newUsersTestRecord(t, app, "current@example.com", "CurrentGrantKey")
	queuedOnly := newUsersTestRecord(t, app, "queued@example.com", "QueuedGrantKey")
	terminated := newUsersTestRecord(t, app, "terminated@example.com", "TerminatedGrantKey")
	none := newUsersTestRecord(t, app, "none@example.com", "NoGrantKey")

	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", 100)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	saveGrant := func(user *core.Record, start time.Time, terminatedAt time.Time) *core.Record {
		grant := core.NewRecord(grants)
		grant.Set("user", user.Id)
		grant.Set("subscription_type", typ.Id)
		grant.Set("starts_at", start)
		grant.Set("ends_at", start.Add(360*day))
		if !terminatedAt.IsZero() {
			grant.Set("terminated_at", terminatedAt)
		}
		if err := app.Save(grant); err != nil {
			t.Fatal(err)
		}
		return grant
	}
	used := saveGrant(current, now.Add(-day), time.Time{})
	subscriptions.SetWindow(used, 0, subscriptions.Usage{Tx: 30, Rx: 12}, 0)
	if err := app.Save(used); err != nil {
		t.Fatal(err)
	}
	saveGrant(current, now.Add(359*day), time.Time{})
	saveGrant(queuedOnly, now.Add(day), time.Time{})
	saveGrant(terminated, now.Add(-day), now.Add(-time.Hour))

	got, err := currentSubscriptionsByUser(app, []*core.Record{current, queuedOnly, terminated, none}, now)
	if err != nil {
		t.Fatal(err)
	}
	view := got[current.Id]
	if view == nil || view.Status != "current" || view.AllowanceBytes != 100 || view.UsedTxBytes != 30 || view.UsedRxBytes != 12 || view.UsedBytes != 42 || view.RemainingBytes != 58 {
		t.Fatalf("current user subscription = %+v", view)
	}
	for _, user := range []*core.Record{queuedOnly, terminated, none} {
		if got[user.Id] != nil {
			t.Fatalf("%s subscription = %+v; want none", user.GetString("email"), got[user.Id])
		}
	}
}

func TestUserListItemShowsLifetimeTrafficOnlyForLegacyUsers(t *testing.T) {
	app := newMigratedTestApp(t)
	legacy := newUsersTestRecord(t, app, "legacy-list@example.com", "LegacyListKey")
	metered := newUsersTestRecord(t, app, "metered-list@example.com", "MeteredListKey")
	for _, u := range []*core.Record{legacy, metered} {
		u.Set("used_tx", 700)
		u.Set("used_rx", 300)
	}
	legacy.Set("subscription_required", false)
	metered.Set("subscription_required", true)

	item := userListItem(legacy, UserProfile{}, nil)
	if item.UsedTx == nil || *item.UsedTx != 700 || item.UsedRx == nil || *item.UsedRx != 300 {
		t.Fatalf("legacy row usage = %v/%v; want 700/300", item.UsedTx, item.UsedRx)
	}
	item = userListItem(metered, UserProfile{}, nil)
	if item.UsedTx != nil || item.UsedRx != nil {
		t.Fatalf("metered row exposes lifetime usage %v/%v", *item.UsedTx, *item.UsedRx)
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["used_tx"] != nil || fields["used_rx"] != nil {
		t.Fatalf("metered row JSON = %s; want null lifetime usage", raw)
	}
}

func TestFindUsersBySubscriptionUsageRanksOnlyCurrentGrants(t *testing.T) {
	app := newMigratedTestApp(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	day := 24 * time.Hour
	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", 1000)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	grantWith := func(user *core.Record, start time.Time, window int, tx, rx int64) {
		grant := core.NewRecord(grants)
		grant.Set("user", user.Id)
		grant.Set("subscription_type", typ.Id)
		grant.Set("starts_at", start)
		grant.Set("ends_at", start.Add(360*day))
		subscriptions.SetWindow(grant, window, subscriptions.Usage{Tx: tx, Rx: rx}, 0)
		if err := app.Save(grant); err != nil {
			t.Fatal(err)
		}
	}
	legacy := newUsersTestRecord(t, app, "legacy-sort@example.com", "LegacySortKey")
	heavyRx := newUsersTestRecord(t, app, "heavy-rx@example.com", "HeavyRxKey")
	heavyTx := newUsersTestRecord(t, app, "heavy-tx@example.com", "HeavyTxKey")
	staleWindow := newUsersTestRecord(t, app, "stale-window@example.com", "StaleWindowKey")
	expired := newUsersTestRecord(t, app, "expired-sort@example.com", "ExpiredSortKey")
	grantWith(heavyRx, now.Add(-day), 0, 10, 50)
	grantWith(heavyTx, now.Add(-day), 0, 40, 5)
	// Usage stored for window 0 of a grant now in window 1 counts as empty.
	grantWith(staleWindow, now.Add(-31*day), 0, 900, 900)
	grantWith(expired, now.Add(-400*day), 11, 800, 800)

	unsubscribed := map[string]bool{legacy.Id: true, expired.Id: true}
	for _, tt := range []struct {
		column string
		desc   bool
		want   []*core.Record
	}{
		{"used", true, []*core.Record{heavyRx, heavyTx, staleWindow}},
		{"used", false, []*core.Record{staleWindow, heavyTx, heavyRx}},
		{"tx", true, []*core.Record{heavyTx, heavyRx, staleWindow}},
		{"rx", false, []*core.Record{staleWindow, heavyTx, heavyRx}},
	} {
		got, err := findUsersBySubscriptionUsage(app, "", nil, tt.column, tt.desc, 25, 0, now)
		if err != nil {
			t.Fatalf("%s desc=%t: %v", tt.column, tt.desc, err)
		}
		if len(got) != 5 {
			t.Fatalf("%s desc=%t returned %d users; want 5", tt.column, tt.desc, len(got))
		}
		for i, want := range tt.want {
			if got[i].Id != want.Id {
				t.Fatalf("%s desc=%t position %d = %s; want %s", tt.column, tt.desc, i, got[i].GetString("email"), want.GetString("email"))
			}
		}
		for _, user := range got[3:] {
			if !unsubscribed[user.Id] {
				t.Fatalf("%s desc=%t placed %s after unsubscribed users", tt.column, tt.desc, user.GetString("email"))
			}
		}
	}

	filter, params := buildUserListFilter("heavy", "")
	got, err := findUsersBySubscriptionUsage(app, filter, params, "used", true, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Id != heavyTx.Id {
		t.Fatalf("filtered second page = %v; want heavy-tx only", got)
	}
}
