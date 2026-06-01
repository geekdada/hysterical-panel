package api

import (
	"testing"
	"time"

	"hysterical-panel/internal/hysteria"
	"hysterical-panel/internal/ipmeta"
)

func TestHostOf(t *testing.T) {
	tests := map[string]string{
		"example.com:443":            "example.com",
		"1.1.1.1:443":                "1.1.1.1",
		"[2606:4700:4700::1111]:443": "2606:4700:4700::1111",
		"2606:4700:4700::1111":       "2606:4700:4700::1111",
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := hostOf(input); got != want {
				t.Fatalf("hostOf(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestLiveAggregatorTopDomainsIPMeta(t *testing.T) {
	lookup := fakeIPLookup{
		"8.8.8.8": &ipmeta.Info{
			IP:          "8.8.8.8",
			ASN:         "GOOGLE",
			CountryCode: "US",
			CountryName: "United States",
			IPInfoURL:   "https://ipinfo.io/8.8.8.8",
		},
	}
	agg := newLiveAggregator(lookup)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	agg.add(hysteria.Stream{
		Connection:    1,
		Stream:        1,
		HookedReqAddr: "8.8.8.8:443",
		Tx:            100,
		Rx:            200,
	}, now)
	agg.add(hysteria.Stream{
		Connection:    2,
		Stream:        1,
		HookedReqAddr: "example.com:443",
		Tx:            1,
		Rx:            2,
	}, now)

	rows := agg.topDomains()
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	ipRow := rows[0]
	if ipRow["domain"] != "8.8.8.8" {
		t.Fatalf("domain = %v, want 8.8.8.8", ipRow["domain"])
	}
	meta, ok := ipRow["ip_meta"].(*ipmeta.Info)
	if !ok {
		t.Fatalf("ip_meta = %#v, want *ipmeta.Info", ipRow["ip_meta"])
	}
	if meta.ASN != "GOOGLE" || meta.CountryCode != "US" || meta.IPInfoURL != "https://ipinfo.io/8.8.8.8" {
		t.Fatalf("ip_meta = %#v", meta)
	}

	domainRow := rows[1]
	if domainRow["domain"] != "example.com" {
		t.Fatalf("domain = %v, want example.com", domainRow["domain"])
	}
	if _, ok := domainRow["ip_meta"]; ok {
		t.Fatalf("domain row has ip_meta: %#v", domainRow["ip_meta"])
	}
}

type fakeIPLookup map[string]*ipmeta.Info

func (f fakeIPLookup) LookupHost(host string) *ipmeta.Info {
	return f[host]
}
