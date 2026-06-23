// Package ipmeta enriches IP literals with metadata from local MMDB files.
package ipmeta

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/oschwald/maxminddb-golang/v2"
)

const dbName = "ipinfo_lite.mmdb"

// Info is the metadata returned for an IP literal.
type Info struct {
	IP          string `json:"ip"`
	ASN         string `json:"asn,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	IPInfoURL   string `json:"ipinfo_url,omitempty"`
}

// Lookup holds an open MMDB reader. It is safe to reuse across requests.
type Lookup struct {
	reader *maxminddb.Reader
}

// New opens the bundled MMDB file from dir.
func New(dir string) (*Lookup, error) {
	reader, err := maxminddb.Open(filepath.Join(dir, dbName))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbName, err)
	}

	return &Lookup{reader: reader}, nil
}

// Close releases the MMDB reader.
func (l *Lookup) Close() error {
	if l == nil || l.reader == nil {
		return nil
	}
	return l.reader.Close()
}

// LookupHost returns metadata for an IP literal host. Non-IP hosts return nil.
func (l *Lookup) LookupHost(host string) *Info {
	addr, ok := parseIPHost(host)
	if !ok {
		return nil
	}

	info := &Info{IP: addr.String()}
	if addr.Is4() {
		info.IPInfoURL = "https://ipinfo.io/" + info.IP
	}

	if l == nil || l.reader == nil {
		return info
	}

	var record struct {
		ASN         string `maxminddb:"asn"`
		ASName      string `maxminddb:"as_name"`
		Country     string `maxminddb:"country"`
		CountryCode string `maxminddb:"country_code"`
	}
	result := l.reader.Lookup(addr)
	if result.Found() && result.Decode(&record) == nil {
		info.ASN = formatASN(record.ASN, record.ASName)
		info.CountryCode = record.CountryCode
		info.CountryName = record.Country
	}

	return info
}

// formatASN combines the AS number and org name into a single label,
// e.g. "AS4134 CHINANET BACKBONE". Either part may be empty.
func formatASN(number, name string) string {
	switch {
	case number != "" && name != "":
		return number + " " + name
	case number != "":
		return number
	default:
		return name
	}
}

func parseIPHost(host string) (netip.Addr, bool) {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "" {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}
