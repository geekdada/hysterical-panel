package api

import (
	"log"
	"slices"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const recentLoginIPLimit = 5

// bindLoginTracking records the time and client IP of every successful panel
// sign-in. Token refresh also fires this hook with an empty auth method, and it
// does not count. A failed write only loses one history entry, so sign-in
// proceeds.
func (h *Handlers) bindLoginTracking() {
	h.app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if e.AuthMethod == "" {
			return nil
		}
		saveLogin(e.App, e.Record.Id, e.RealIP())
		return nil
	})
}

// saveLogin reloads the User so a sign-in never overwrites fields that the
// auth flow itself just saved, such as verified after an OTP sign-in.
func saveLogin(app core.App, userID, ip string) {
	u, err := app.FindRecordById("users", userID)
	if err != nil {
		log.Printf("[login] user %s not found: %v", userID, err)
		return
	}
	recordLogin(u, ip, time.Now())
	if err := app.Save(u); err != nil {
		log.Printf("[login] record sign-in for user %s: %v", userID, err)
	}
}

func recordLogin(u *core.Record, ip string, now time.Time) {
	u.Set("last_login_at", now.UTC())
	normalized, ok := normalizeStoredConnectionIP(ip)
	if !ok {
		u.Set("last_login_ip", "")
		return
	}
	u.Set("last_login_ip", normalized)

	out := make([]string, 0, recentLoginIPLimit)
	out = append(out, normalized)
	for _, stored := range storedLoginIPs(u) {
		if len(out) == recentLoginIPLimit {
			break
		}
		if !slices.Contains(out, stored) {
			out = append(out, stored)
		}
	}
	u.Set("recent_login_ips", out)
}

// loginIPsFromRecord returns the last sign-in IP (nil before the first
// sign-in) and the recent unique sign-in IPs, each with MMDB metadata.
func loginIPsFromRecord(u *core.Record, lookup ipMetadataLookup) (*LoginIP, []LoginIP) {
	var last *LoginIP
	if ip, ok := normalizeStoredConnectionIP(u.GetString("last_login_ip")); ok {
		row := loginIP(ip, lookup)
		last = &row
	}

	recent := make([]LoginIP, 0, recentLoginIPLimit)
	seen := make([]string, 0, recentLoginIPLimit)
	for _, ip := range storedLoginIPs(u) {
		if len(recent) == recentLoginIPLimit {
			break
		}
		if slices.Contains(seen, ip) {
			continue
		}
		seen = append(seen, ip)
		recent = append(recent, loginIP(ip, lookup))
	}
	return last, recent
}

// storedLoginIPs drops entries that are not valid IP literals.
func storedLoginIPs(u *core.Record) []string {
	var stored []string
	if err := u.UnmarshalJSONField("recent_login_ips", &stored); err != nil {
		return nil
	}
	out := make([]string, 0, len(stored))
	for _, entry := range stored {
		if ip, ok := normalizeStoredConnectionIP(entry); ok {
			out = append(out, ip)
		}
	}
	return out
}

func loginIP(ip string, lookup ipMetadataLookup) LoginIP {
	row := LoginIP{IP: ip}
	if lookup != nil {
		row.IPMeta = ipMetaDTO(lookup.LookupHost(ip))
	}
	return row
}
