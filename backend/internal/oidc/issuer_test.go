package oidc

import (
	"errors"
	"testing"
)

func TestIssuer(t *testing.T) {
	cases := []struct {
		backend, want string
		err           error
	}{
		{"https://panel.example.com", "https://panel.example.com/api/oidc", nil},
		{"https://panel.example.com/", "https://panel.example.com/api/oidc", nil},
		{"http://localhost:8090", "http://localhost:8090/api/oidc", nil},
		{"http://127.0.0.1:8090", "http://127.0.0.1:8090/api/oidc", nil},
		{"http://[::1]:8090", "http://[::1]:8090/api/oidc", nil},
		{"", "", ErrBackendURLMissing},
		{"http://panel.example.com", "", ErrBackendURLInsecure},
		{"http://localhost.example.com", "", ErrBackendURLInsecure},
	}
	for _, c := range cases {
		got, err := Issuer(c.backend)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("Issuer(%q) = %q, %v; want %q, %v", c.backend, got, err, c.want, c.err)
		}
	}
}
