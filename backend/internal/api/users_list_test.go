package api

import (
	"testing"
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
	}
	for _, c := range cases {
		if got := normalizeUserListSort(c.raw); got != c.want {
			t.Errorf("normalizeUserListSort(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestBuildUserListFilter(t *testing.T) {
	filter, params := buildUserListFilter("")
	if filter != "" || params != nil {
		t.Fatalf("empty search: filter=%q params=%v", filter, params)
	}

	filter, params = buildUserListFilter("  alice  ")
	wantFilter := "(email ~ {:like} || role ~ {:like} || status ~ {:like} || auth_string = {:auth})"
	if filter != wantFilter {
		t.Fatalf("filter = %q, want %q", filter, wantFilter)
	}
	if params["like"] != "%alice%" || params["auth"] != "alice" {
		t.Fatalf("params = %#v, want like=%%alice%% auth=alice", params)
	}
}

func TestParseUserListQuery(t *testing.T) {
	q := parseUserListQuery("2", "50", " foo ", "-email")
	if q.Page != 2 || q.PerPage != 50 || q.Search != "foo" || q.Sort != "-email" {
		t.Fatalf("parseUserListQuery() = %+v", q)
	}
}

func TestBuildUserListFilterParamsType(t *testing.T) {
	_, params := buildUserListFilter("x")
	if params == nil || params["like"] == nil || params["auth"] == nil {
		t.Fatalf("params = %#v", params)
	}
}
