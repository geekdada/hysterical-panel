package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestRecordLogin(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)

	t.Run("first sign-in sets time, ip and list", func(t *testing.T) {
		rec := newRecentLoginsRecord()
		recordLogin(rec, "8.8.8.8", now)
		if got := rec.GetDateTime("last_login_at").Time(); !got.Equal(now) {
			t.Fatalf("last_login_at = %v, want %v", got, now)
		}
		if got := rec.GetString("last_login_ip"); got != "8.8.8.8" {
			t.Fatalf("last_login_ip = %q", got)
		}
		assertStoredLoginIPs(t, rec, "8.8.8.8")
	})

	t.Run("repeated ip moves to the front", func(t *testing.T) {
		rec := newRecentLoginsRecord()
		rec.Set("recent_login_ips", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"})
		recordLogin(rec, "3.3.3.3", now)
		assertStoredLoginIPs(t, rec, "3.3.3.3", "1.1.1.1", "2.2.2.2")
	})

	t.Run("list keeps the five newest unique ips", func(t *testing.T) {
		rec := newRecentLoginsRecord()
		rec.Set("recent_login_ips", []string{"1.1.1.1", "2.2.2.2", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5"})
		recordLogin(rec, "6.6.6.6", now)
		assertStoredLoginIPs(t, rec, "6.6.6.6", "1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4")
	})

	t.Run("ipv6 is normalized", func(t *testing.T) {
		rec := newRecentLoginsRecord()
		recordLogin(rec, "2001:DB8:0::1", now)
		if got := rec.GetString("last_login_ip"); got != "2001:db8::1" {
			t.Fatalf("last_login_ip = %q", got)
		}
		assertStoredLoginIPs(t, rec, "2001:db8::1")
	})

	t.Run("unparseable ip keeps the time and clears the last ip", func(t *testing.T) {
		rec := newRecentLoginsRecord()
		rec.Set("last_login_ip", "1.1.1.1")
		rec.Set("recent_login_ips", []string{"1.1.1.1"})
		recordLogin(rec, "not-an-ip", now)
		if got := rec.GetDateTime("last_login_at").Time(); !got.Equal(now) {
			t.Fatalf("last_login_at = %v, want %v", got, now)
		}
		if got := rec.GetString("last_login_ip"); got != "" {
			t.Fatalf("last_login_ip = %q, want empty", got)
		}
		assertStoredLoginIPs(t, rec, "1.1.1.1")
	})
}

func TestLoginIPsFromRecord(t *testing.T) {
	lookup := recentConnectionFakeLookup{
		"8.8.8.8": {IP: "8.8.8.8", ASN: "AS15169 Google LLC", CountryCode: "US"},
	}

	t.Run("adds metadata and skips invalid entries", func(t *testing.T) {
		rec := newRecentLoginsRecord()
		rec.Set("last_login_ip", "8.8.8.8")
		rec.Set("recent_login_ips", []string{"8.8.8.8", "bad", "1.1.1.1", "8.8.8.8"})
		last, recent := loginIPsFromRecord(rec, lookup)
		if last == nil || last.IP != "8.8.8.8" || last.IPMeta == nil || last.IPMeta.ASN != "AS15169 Google LLC" {
			t.Fatalf("last = %#v", last)
		}
		if len(recent) != 2 || recent[0].IP != "8.8.8.8" || recent[1].IP != "1.1.1.1" {
			t.Fatalf("recent = %#v", recent)
		}
		if recent[0].IPMeta == nil || recent[1].IPMeta != nil {
			t.Fatalf("recent metadata = %#v / %#v", recent[0].IPMeta, recent[1].IPMeta)
		}
	})

	t.Run("never signed in returns nil and an empty list", func(t *testing.T) {
		last, recent := loginIPsFromRecord(newRecentLoginsRecord(), lookup)
		if last != nil {
			t.Fatalf("last = %#v, want nil", last)
		}
		if recent == nil || len(recent) != 0 {
			t.Fatalf("recent = %#v, want empty slice", recent)
		}
	})
}

func TestLoginTrackingRecordsSignInButNotRefresh(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "login-track@example.com", "LoginTrackSecret123")
	(&Handlers{app: app}).bindLoginTracking()

	login := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-with-password",
		`{"identity":"login-track@example.com","password":"password12345"}`, "")
	loginToken := responseToken(t, login)

	signedIn := reloadUser(t, app, user.Id)
	signedInAt := signedIn.GetDateTime("last_login_at")
	if signedInAt.IsZero() {
		t.Fatal("password sign-in did not set last_login_at")
	}
	// httptest requests come from 192.0.2.1.
	if got := signedIn.GetString("last_login_ip"); got != "192.0.2.1" {
		t.Fatalf("last_login_ip = %q", got)
	}
	assertStoredLoginIPs(t, signedIn, "192.0.2.1")

	time.Sleep(1100 * time.Millisecond)
	refresh := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-refresh", "", "Bearer "+loginToken)
	responseToken(t, refresh)
	if got := reloadUser(t, app, user.Id).GetDateTime("last_login_at"); !got.Equal(signedInAt) {
		t.Fatalf("refresh changed last_login_at from %v to %v", signedInAt, got)
	}
}

func TestLoginTrackingSkipsRejectedSignIn(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "login-reject@example.com", "LoginRejectSecret123")
	user.Set("status", "disabled")
	if err := app.Save(user); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	h := &Handlers{app: app}
	h.bindAuthGate()
	h.bindLoginTracking()

	rec := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-with-password",
		`{"identity":"login-reject@example.com","password":"password12345"}`, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := reloadUser(t, app, user.Id).GetDateTime("last_login_at"); !got.IsZero() {
		t.Fatalf("rejected sign-in set last_login_at = %v", got)
	}
}

func newRecentLoginsRecord() *core.Record {
	collection := core.NewBaseCollection("users")
	collection.Fields.Add(&core.DateField{Name: "last_login_at"})
	collection.Fields.Add(&core.TextField{Name: "last_login_ip"})
	collection.Fields.Add(&core.JSONField{Name: "recent_login_ips"})
	return core.NewRecord(collection)
}

func assertStoredLoginIPs(t *testing.T, rec *core.Record, want ...string) {
	t.Helper()
	var got []string
	if err := rec.UnmarshalJSONField("recent_login_ips", &got); err != nil {
		t.Fatalf("UnmarshalJSONField() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("recent_login_ips = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("recent_login_ips = %v, want %v", got, want)
		}
	}
}

func reloadUser(t *testing.T, app core.App, id string) *core.Record {
	t.Helper()
	rec, err := app.FindRecordById("users", id)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	return rec
}
