package api

import (
	"strconv"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/ipmeta"
)

func TestClientIPFromHysteriaAddr(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "ipv4 with port", in: "1.2.3.4:5678", want: "1.2.3.4", ok: true},
		{name: "bracketed ipv6 with port", in: "[2001:db8::1]:443", want: "2001:db8::1", ok: true},
		{name: "bare ipv6", in: "2001:db8::1", want: "2001:db8::1", ok: true},
		{name: "bad host", in: "bad host:abc", ok: false},
		{name: "empty", in: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := clientIPFromHysteriaAddr(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("ip = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateRecentConnections(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 34, 56, 0, time.UTC)

	t.Run("new ip creates one entry", func(t *testing.T) {
		rec := newRecentConnectionsRecord()
		if err := updateRecentConnections(rec, "8.8.8.8", now); err != nil {
			t.Fatalf("updateRecentConnections() error = %v", err)
		}
		got := readStoredRecentConnections(t, rec)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].IP != "8.8.8.8" || got[0].LastSeenAt != now.Format(time.RFC3339) {
			t.Fatalf("entry = %#v", got[0])
		}
	})

	t.Run("repeated ip updates timestamp and order", func(t *testing.T) {
		rec := newRecentConnectionsRecord()
		rec.Set("recent_connections", []storedRecentConnection{
			{IP: "1.1.1.1", LastSeenAt: "old-1"},
			{IP: "8.8.8.8", LastSeenAt: "old-2"},
		})

		if err := updateRecentConnections(rec, "8.8.8.8", now); err != nil {
			t.Fatalf("updateRecentConnections() error = %v", err)
		}
		got := readStoredRecentConnections(t, rec)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].IP != "8.8.8.8" || got[0].LastSeenAt != now.Format(time.RFC3339) {
			t.Fatalf("first entry = %#v", got[0])
		}
		if got[1].IP != "1.1.1.1" {
			t.Fatalf("second entry = %#v", got[1])
		}
	})

	t.Run("keeps only ten unique ips", func(t *testing.T) {
		rec := newRecentConnectionsRecord()
		for i := 1; i <= 11; i++ {
			if err := updateRecentConnections(rec, "192.0.2."+strconv.Itoa(i), now.Add(time.Duration(i)*time.Second)); err != nil {
				t.Fatalf("updateRecentConnections(%d) error = %v", i, err)
			}
		}
		got := readStoredRecentConnections(t, rec)
		if len(got) != recentConnectionLimit {
			t.Fatalf("len = %d, want %d", len(got), recentConnectionLimit)
		}
		if got[0].IP != "192.0.2.11" {
			t.Fatalf("first ip = %q, want 192.0.2.11", got[0].IP)
		}
		for _, entry := range got {
			if entry.IP == "192.0.2.1" {
				t.Fatalf("oldest ip was not truncated: %#v", got)
			}
		}
	})

	t.Run("malformed existing value and entries are repaired", func(t *testing.T) {
		rec := newRecentConnectionsRecord()
		rec.Set("recent_connections", "not an array")
		if err := updateRecentConnections(rec, "8.8.8.8", now); err != nil {
			t.Fatalf("updateRecentConnections() error = %v", err)
		}
		got := readStoredRecentConnections(t, rec)
		if len(got) != 1 || got[0].IP != "8.8.8.8" {
			t.Fatalf("got %#v, want only 8.8.8.8", got)
		}

		rec.Set("recent_connections", []storedRecentConnection{
			{IP: "bad"},
			{IP: "2001:db8::1"},
		})
		if err := updateRecentConnections(rec, "8.8.8.8", now); err != nil {
			t.Fatalf("updateRecentConnections() error = %v", err)
		}
		got = readStoredRecentConnections(t, rec)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].IP != "8.8.8.8" || got[1].IP != "2001:db8::1" {
			t.Fatalf("got %#v", got)
		}
	})
}

func TestRecentConnectionsFromRecord(t *testing.T) {
	rec := newRecentConnectionsRecord()
	rec.Set("recent_connections", []storedRecentConnection{
		{IP: "8.8.8.8", LastSeenAt: "2026-06-22T12:34:56Z"},
		{IP: "1.1.1.1", LastSeenAt: "2026-06-22T12:00:00Z"},
	})

	rows := recentConnectionsFromRecord(rec, recentConnectionFakeLookup{
		"8.8.8.8": &ipmeta.Info{
			IP:          "8.8.8.8",
			ASN:         "AS15169",
			CountryCode: "US",
			CountryName: "United States",
			IPInfoURL:   "https://ipinfo.io/8.8.8.8",
		},
	})

	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	if rows[0].IP != "8.8.8.8" || rows[1].IP != "1.1.1.1" {
		t.Fatalf("order = %#v", rows)
	}
	if rows[0].IPMeta == nil || rows[0].IPMeta.ASN != "AS15169" || rows[0].IPMeta.CountryCode != "US" {
		t.Fatalf("first ip_meta = %#v", rows[0].IPMeta)
	}
	if rows[1].IPMeta != nil {
		t.Fatalf("second ip_meta = %#v, want nil", rows[1].IPMeta)
	}
}

func newRecentConnectionsRecord() *core.Record {
	collection := core.NewBaseCollection("users")
	collection.Fields.Add(&core.JSONField{Name: "recent_connections"})
	return core.NewRecord(collection)
}

func readStoredRecentConnections(t *testing.T, rec *core.Record) []storedRecentConnection {
	t.Helper()
	var got []storedRecentConnection
	if err := rec.UnmarshalJSONField("recent_connections", &got); err != nil {
		t.Fatalf("UnmarshalJSONField() error = %v", err)
	}
	return got
}

type recentConnectionFakeLookup map[string]*ipmeta.Info

func (f recentConnectionFakeLookup) LookupHost(host string) *ipmeta.Info {
	return f[host]
}
