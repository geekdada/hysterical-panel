package api

import (
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/ipmeta"
)

const recentConnectionLimit = 10

type storedRecentConnection struct {
	IP         string `json:"ip"`
	LastSeenAt string `json:"last_seen_at"`
}

func clientIPFromHysteriaAddr(addr string) (string, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", false
	}

	host := addr
	if splitHost, _, err := net.SplitHostPort(addr); err == nil {
		host = splitHost
	}
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "" {
		return "", false
	}

	parsed, err := netip.ParseAddr(host)
	if err != nil {
		return "", false
	}
	return parsed.String(), true
}

func updateRecentConnections(u *core.Record, ip string, now time.Time) error {
	parsed, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return err
	}
	ip = parsed.String()

	var existing []storedRecentConnection
	if err := u.UnmarshalJSONField("recent_connections", &existing); err != nil {
		existing = nil
	}

	seenAt := now.UTC().Format(time.RFC3339)
	out := make([]storedRecentConnection, 0, recentConnectionLimit)
	out = append(out, storedRecentConnection{IP: ip, LastSeenAt: seenAt})
	seen := map[string]struct{}{ip: {}}

	for _, entry := range existing {
		entryIP, ok := normalizeStoredConnectionIP(entry.IP)
		if !ok || entryIP == ip {
			continue
		}
		if _, exists := seen[entryIP]; exists {
			continue
		}
		entry.IP = entryIP
		out = append(out, entry)
		seen[entryIP] = struct{}{}
		if len(out) == recentConnectionLimit {
			break
		}
	}

	u.Set("recent_connections", out)
	return nil
}

func recentConnectionsFromRecord(u *core.Record, lookup ipMetadataLookup, ignored map[string]struct{}) []RecentConnection {
	var stored []storedRecentConnection
	if err := u.UnmarshalJSONField("recent_connections", &stored); err != nil {
		return []RecentConnection{}
	}

	out := make([]RecentConnection, 0, min(len(stored), recentConnectionLimit))
	seen := map[string]struct{}{}
	for _, entry := range stored {
		ip, ok := normalizeStoredConnectionIP(entry.IP)
		if !ok {
			continue
		}
		if _, exists := seen[ip]; exists {
			continue
		}
		if ignored != nil {
			if _, skip := ignored[ip]; skip {
				continue
			}
		}
		row := RecentConnection{
			IP:         ip,
			LastSeenAt: entry.LastSeenAt,
		}
		if lookup != nil {
			row.IPMeta = ipMetaDTO(lookup.LookupHost(ip))
		}
		out = append(out, row)
		seen[ip] = struct{}{}
		if len(out) == recentConnectionLimit {
			break
		}
	}
	return out
}

func removeIPFromRecentConnections(u *core.Record, ip string) bool {
	var stored []storedRecentConnection
	if err := u.UnmarshalJSONField("recent_connections", &stored); err != nil || len(stored) == 0 {
		return false
	}
	out := make([]storedRecentConnection, 0, len(stored))
	removed := false
	for _, entry := range stored {
		entryIP, ok := normalizeStoredConnectionIP(entry.IP)
		if !ok || entryIP == ip {
			if ok && entryIP == ip {
				removed = true
			}
			continue
		}
		entry.IP = entryIP
		out = append(out, entry)
	}
	if !removed {
		return false
	}
	u.Set("recent_connections", out)
	return true
}

func normalizeStoredConnectionIP(ip string) (string, bool) {
	parsed, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return "", false
	}
	return parsed.String(), true
}

func ipMetaDTO(info *ipmeta.Info) *IPMeta {
	if info == nil {
		return nil
	}
	return &IPMeta{
		IP:          info.IP,
		ASN:         info.ASN,
		CountryCode: info.CountryCode,
		CountryName: info.CountryName,
		IPInfoURL:   info.IPInfoURL,
	}
}
