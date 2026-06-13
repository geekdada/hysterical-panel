package api

import (
	"testing"
	"time"
)

func TestRegistrationDecision(t *testing.T) {
	cases := []struct {
		name             string
		s                settings
		wantAllowed      bool
		wantCodeRequired bool
	}{
		{"all off → closed", settings{}, false, false},
		{"invitations only → invite-only", settings{InvitationsEnabled: true}, true, true},
		{"open, no code", settings{OpenRegistration: true}, true, false},
		{"open + require invite", settings{OpenRegistration: true, RequireInviteForOpen: true}, true, true},
		{"open wins over invitations off", settings{OpenRegistration: true, InvitationsEnabled: false}, true, false},
		{"open + require + invitations on", settings{OpenRegistration: true, RequireInviteForOpen: true, InvitationsEnabled: true}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allowed, codeRequired := registrationDecision(c.s)
			if allowed != c.wantAllowed || codeRequired != c.wantCodeRequired {
				t.Errorf("registrationDecision(%+v) = (%v, %v), want (%v, %v)",
					c.s, allowed, codeRequired, c.wantAllowed, c.wantCodeRequired)
			}
		})
	}
}

func TestInvalidInviteReason(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name      string
		revoked   bool
		expiresAt time.Time
		maxUses   int
		usedCount int
		want      string
	}{
		{"fresh unlimited", false, time.Time{}, 0, 0, ""},
		{"fresh with future expiry and remaining uses", false, future, 5, 2, ""},
		{"revoked", true, time.Time{}, 0, 0, "revoked"},
		{"expired", false, past, 0, 0, "expired"},
		{"exhausted at limit", false, time.Time{}, 3, 3, "exhausted"},
		{"exhausted over limit", false, time.Time{}, 1, 2, "exhausted"},
		{"unlimited never exhausts", false, time.Time{}, 0, 999, ""},
		{"revoked takes precedence over expiry", true, past, 1, 1, "revoked"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := invalidInviteReason(c.revoked, c.expiresAt, c.maxUses, c.usedCount, now)
			if got != c.want {
				t.Errorf("invalidInviteReason() = %q, want %q", got, c.want)
			}
		})
	}
}
