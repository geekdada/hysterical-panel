package oidc

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IssuerPath is appended to PANEL_BACKEND_URL_BASE. The deployed nginx /api/
// rule already routes it to the backend.
const IssuerPath = "/api/oidc"

var (
	ErrBackendURLMissing  = errors.New("PANEL_BACKEND_URL_BASE is not set")
	ErrBackendURLInsecure = errors.New("PANEL_BACKEND_URL_BASE must use https unless it points to a loopback host")
)

// Issuer derives the provider issuer from the public backend origin. OIDC
// requires an https issuer; plain http is accepted on loopback hosts so the
// provider works in local development.
func Issuer(backendURL string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(backendURL), "/")
	if base == "" {
		return "", ErrBackendURLMissing
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: %q is not an absolute URL", ErrBackendURLInsecure, base)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return "", ErrBackendURLInsecure
	}
	return base + IssuerPath, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
