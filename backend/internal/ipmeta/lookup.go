// Package ipmeta enriches IP literals with metadata from local MMDB files.
package ipmeta

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/oschwald/maxminddb-golang/v2"
)

const (
	asnDBName     = "Country-asn.mmdb"
	countryDBName = "Country-without-asn.mmdb"
)

// Info is the metadata returned for an IP literal.
type Info struct {
	IP          string `json:"ip"`
	ASN         string `json:"asn,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	IPInfoURL   string `json:"ipinfo_url,omitempty"`
}

// Lookup holds open MMDB readers. It is safe to reuse across requests.
type Lookup struct {
	asn     *maxminddb.Reader
	country *maxminddb.Reader
}

// New opens the bundled MMDB files from dir.
func New(dir string) (*Lookup, error) {
	asn, err := maxminddb.Open(filepath.Join(dir, asnDBName))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", asnDBName, err)
	}

	country, err := maxminddb.Open(filepath.Join(dir, countryDBName))
	if err != nil {
		_ = asn.Close()
		return nil, fmt.Errorf("open %s: %w", countryDBName, err)
	}

	return &Lookup{asn: asn, country: country}, nil
}

// Close releases both MMDB readers.
func (l *Lookup) Close() error {
	if l == nil {
		return nil
	}
	return errors.Join(closeReader(l.asn), closeReader(l.country))
}

func closeReader(r *maxminddb.Reader) error {
	if r == nil {
		return nil
	}
	return r.Close()
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

	if l == nil {
		return info
	}

	if l.asn != nil {
		var record struct {
			Country struct {
				ISOCode string `maxminddb:"iso_code"`
			} `maxminddb:"country"`
		}
		result := l.asn.Lookup(addr)
		if result.Found() && result.Decode(&record) == nil {
			info.ASN = record.Country.ISOCode
		}
	}

	if l.country != nil {
		var record struct {
			Country struct {
				ISOCode string            `maxminddb:"iso_code"`
				Names   map[string]string `maxminddb:"names"`
			} `maxminddb:"country"`
		}
		result := l.country.Lookup(addr)
		if result.Found() && result.Decode(&record) == nil {
			info.CountryCode = record.Country.ISOCode
			info.CountryName = record.Country.Names["en"]
		}
	}

	return info
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
