package oidc

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

var signInTime = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

func TestRecordSessionAuthStoresSignInTime(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "signin@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, ok, err := SessionAuthTime(app, "token-a", signInTime.Add(time.Minute))
	if err != nil || !ok || !got.Equal(signInTime) {
		t.Fatalf("auth time = %v ok=%v err=%v, want %v", got, ok, err, signInTime)
	}
}

func TestRecordSessionAuthCarriesTimeAcrossRefresh(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "refresh@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "passkey", "", signInTime); err != nil {
		t.Fatalf("record sign-in: %v", err)
	}
	later := signInTime.Add(2 * time.Hour)
	if err := RecordSessionAuth(app, user, "token-b", "", "token-a", later); err != nil {
		t.Fatalf("record refresh: %v", err)
	}
	got, ok, err := SessionAuthTime(app, "token-b", later)
	if err != nil || !ok || !got.Equal(signInTime) {
		t.Fatalf("refreshed auth time = %v ok=%v err=%v, want %v", got, ok, err, signInTime)
	}
}

func TestRecordSessionAuthSkipsRefreshOfUnknownToken(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "unknown@example.com")
	if err := RecordSessionAuth(app, user, "token-b", "", "token-never-seen", signInTime); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, ok, err := SessionAuthTime(app, "token-b", signInTime); err != nil || ok {
		t.Fatalf("unknown refresh recorded an auth time: ok=%v err=%v", ok, err)
	}
}

func TestRecordSessionAuthIsIdempotentForTheSameToken(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "same@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime.Add(time.Second)); err != nil {
		t.Fatalf("same token signed in again: %v", err)
	}
	if err := RecordSessionAuth(app, user, "token-a", "", "token-a", signInTime.Add(time.Hour)); err != nil {
		t.Fatalf("non-refreshable token refreshed: %v", err)
	}
	if n := countRecords(t, app, SessionAuthTimesCollection); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	got, _, _ := SessionAuthTime(app, "token-a", signInTime.Add(time.Hour))
	if !got.Equal(signInTime) {
		t.Fatalf("auth time = %v, want the first sign-in %v", got, signInTime)
	}
}

func TestSessionAuthTimeIgnoresExpiredRows(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "expired@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime); err != nil {
		t.Fatalf("record: %v", err)
	}
	afterExpiry := signInTime.Add(user.Collection().AuthToken.DurationTime() + time.Second)
	if _, ok, err := SessionAuthTime(app, "token-a", afterExpiry); err != nil || ok {
		t.Fatalf("expired row still answered: ok=%v err=%v", ok, err)
	}
}

func TestSessionAuthTimeOfEmptyTokenIsUnknown(t *testing.T) {
	app := newTestApp(t)
	if _, ok, err := SessionAuthTime(app, "", signInTime); err != nil || ok {
		t.Fatalf("empty token: ok=%v err=%v", ok, err)
	}
}

// Tokens minted for one user in the same second are byte-identical, so a
// refresh of an old session can produce a token a fresh sign-in recorded.
func TestRecordSessionAuthKeepsOlderTimeWhenRefreshCollidesWithSignIn(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "collide@example.com")
	fresh := signInTime.Add(time.Hour)
	mustRecord(t, app, user, "stale", "password", "", signInTime)
	mustRecord(t, app, user, "shared", "password", "", fresh)
	mustRecord(t, app, user, "shared", "", "stale", fresh)
	assertAuthTime(t, app, "shared", fresh, signInTime)
}

func TestRecordSessionAuthKeepsOlderTimeWhenSignInCollidesWithRefresh(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "collide-late@example.com")
	fresh := signInTime.Add(time.Hour)
	mustRecord(t, app, user, "stale", "password", "", signInTime)
	mustRecord(t, app, user, "shared", "", "stale", fresh)
	mustRecord(t, app, user, "shared", "password", "", fresh)
	assertAuthTime(t, app, "shared", fresh, signInTime)
}

func TestRecordSessionAuthForgetsTimeWhenUnknownSessionCollides(t *testing.T) {
	for name, order := range map[string][2]bool{"refresh last": {true, false}, "refresh first": {false, true}} {
		t.Run(name, func(t *testing.T) {
			app := newTestApp(t)
			user := newTestUser(t, app, "collide-unknown@example.com")
			for _, signIn := range order {
				if signIn {
					mustRecord(t, app, user, "shared", "password", "", signInTime)
				} else {
					mustRecord(t, app, user, "shared", "", "never-recorded", signInTime)
				}
			}
			if _, ok, err := SessionAuthTime(app, "shared", signInTime); err != nil || ok {
				t.Fatalf("collision with an unknown session kept an auth time: ok=%v err=%v", ok, err)
			}
		})
	}
}

func mustRecord(t *testing.T, app core.App, user *core.Record, sessionToken, authMethod, previousToken string, now time.Time) {
	t.Helper()
	if err := RecordSessionAuth(app, user, sessionToken, authMethod, previousToken, now); err != nil {
		t.Fatalf("record %s: %v", sessionToken, err)
	}
}

func assertAuthTime(t *testing.T, app core.App, sessionToken string, now, want time.Time) {
	t.Helper()
	got, ok, err := SessionAuthTime(app, sessionToken, now)
	if err != nil || !ok || !got.Equal(want) {
		t.Fatalf("auth time of %s = %v ok=%v err=%v, want %v", sessionToken, got, ok, err, want)
	}
}
