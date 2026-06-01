package ipmeta

import (
	"path/filepath"
	"testing"
)

func TestLookupHost(t *testing.T) {
	lookup, err := New(filepath.Join("..", "..", "mmdb"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer lookup.Close()

	info := lookup.LookupHost("8.8.8.8")
	if info == nil {
		t.Fatal("LookupHost returned nil")
	}
	if info.IP != "8.8.8.8" {
		t.Fatalf("IP = %q, want 8.8.8.8", info.IP)
	}
	if info.ASN != "GOOGLE" {
		t.Fatalf("ASN = %q, want GOOGLE", info.ASN)
	}
	if info.CountryCode != "US" {
		t.Fatalf("CountryCode = %q, want US", info.CountryCode)
	}
	if info.CountryName != "United States" {
		t.Fatalf("CountryName = %q, want United States", info.CountryName)
	}
	if info.IPInfoURL != "https://ipinfo.io/8.8.8.8" {
		t.Fatalf("IPInfoURL = %q, want https://ipinfo.io/8.8.8.8", info.IPInfoURL)
	}
}

func TestLookupHostNonIP(t *testing.T) {
	lookup := &Lookup{}
	if info := lookup.LookupHost("example.com"); info != nil {
		t.Fatalf("LookupHost returned %#v, want nil", info)
	}
}
