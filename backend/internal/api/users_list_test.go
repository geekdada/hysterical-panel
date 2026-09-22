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

func TestUserListItemOmitsLifetimeTraffic(t *testing.T) {
	raw, err := json.Marshal(UserListItem{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"used_tx", "used_rx"} {
		if _, ok := fields[name]; ok {
			t.Fatalf("list row exposes lifetime %s", name)
		}
	}
	if _, ok := fields["current_subscription"]; !ok {
		t.Fatal("list row lost current_subscription")
	}
}
