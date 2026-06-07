// Package config loads process environment variables for the panel backend.
// Values must already be exported (e.g. direnv, systemd, or make include .env).
package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config holds required and optional settings parsed from the environment.
type Config struct {
	// PanelMasterKey encrypts node api_secret values (AES-GCM via SHA-256 derived key).
	PanelMasterKey string `env:"PANEL_MASTER_KEY,notEmpty"`

	// PanelFrontendURLBase is the panel UI origin for CORS (http:// or https://, no path).
	// Empty leaves CORS at "*" (PocketBase default).
	PanelFrontendURLBase string `env:"PANEL_FRONTEND_URL_BASE"`

	// PanelBackendURLBase is the public panel API origin exposed via GET /api/panel/config.
	// Empty omits api_url from the response so clients fall back to same-origin or build-time config.
	PanelBackendURLBase string `env:"PANEL_BACKEND_URL_BASE"`

	// PanelCorsMaxAge is Access-Control-Max-Age for preflight caching (seconds).
	// Browsers may cap (e.g. Chrome at 7200). 0 disables the header.
	PanelCorsMaxAge int `env:"PANEL_CORS_MAX_AGE" envDefault:"7200"`

	// DataDir is PocketBase's data directory. Empty falls back to ./pb_data.
	DataDir string `env:"PB_DATA_DIR"`

	// MMDBDir contains the bundled MMDB files used for live IP metadata.
	MMDBDir string `env:"MMDB_DIR" envDefault:"mmdb"`

	// PanelWebAuthnRPID is the stable relying party id used for passkeys.
	PanelWebAuthnRPID string `env:"PANEL_WEBAUTHN_RP_ID"`

	// PanelWebAuthnOrigins is a comma-separated list of exact frontend origins allowed for passkeys.
	PanelWebAuthnOrigins string `env:"PANEL_WEBAUTHN_ORIGINS"`
}

// WebAuthnConfig contains normalized passkey configuration.
type WebAuthnConfig struct {
	Enabled bool
	RPID    string
	Origins []string
}

// Load parses the current process environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if _, err := cfg.ServeAllowedOrigins(); err != nil {
		return Config{}, err
	}
	if _, err := cfg.PanelBackendURL(); err != nil {
		return Config{}, err
	}
	if _, err := cfg.WebAuthn(); err != nil {
		return Config{}, err
	}
	if cfg.PanelCorsMaxAge < 0 {
		return Config{}, fmt.Errorf("config: PANEL_CORS_MAX_AGE must be >= 0")
	}
	return cfg, nil
}

// ServeAllowedOrigins returns PocketBase ServeConfig.AllowedOrigins.
// Unset PANEL_FRONTEND_URL_BASE yields ["*"]; when set, a single validated origin.
func (c Config) ServeAllowedOrigins() ([]string, error) {
	raw := strings.TrimSpace(c.PanelFrontendURLBase)
	if raw == "" {
		return []string{"*"}, nil
	}
	origin, err := parseOriginBase(raw)
	if err != nil {
		return nil, fmt.Errorf("config: PANEL_FRONTEND_URL_BASE: %w", err)
	}
	return []string{origin}, nil
}

// PanelBackendURL returns the normalized public API origin, or "" when unset.
func (c Config) PanelBackendURL() (string, error) {
	raw := strings.TrimSpace(c.PanelBackendURLBase)
	if raw == "" {
		return "", nil
	}
	origin, err := parseOriginBase(raw)
	if err != nil {
		return "", fmt.Errorf("config: PANEL_BACKEND_URL_BASE: %w", err)
	}
	return origin, nil
}

// PanelFrontendURL returns the normalized frontend origin, or "" when unset.
func (c Config) PanelFrontendURL() (string, error) {
	raw := strings.TrimSpace(c.PanelFrontendURLBase)
	if raw == "" {
		return "", nil
	}
	return parseOriginBase(raw)
}

// WebAuthn returns normalized passkey config. If no WebAuthn-specific values
// and no static panel origins are configured, passkeys are disabled.
func (c Config) WebAuthn() (WebAuthnConfig, error) {
	rawRPID := strings.TrimSpace(c.PanelWebAuthnRPID)
	rawOrigins := strings.TrimSpace(c.PanelWebAuthnOrigins)
	explicit := rawRPID != "" || rawOrigins != ""

	origins, err := c.webAuthnOrigins(rawOrigins, explicit)
	if err != nil {
		return WebAuthnConfig{}, err
	}

	if rawRPID == "" && len(origins) == 0 {
		return WebAuthnConfig{}, nil
	}

	rpID := rawRPID
	if rpID == "" {
		rpID = originHost(origins[0])
	}
	if err := validateWebAuthnRPID(rpID); err != nil {
		return WebAuthnConfig{}, fmt.Errorf("config: PANEL_WEBAUTHN_RP_ID: %w", err)
	}
	if len(origins) == 0 {
		return WebAuthnConfig{}, fmt.Errorf("config: PANEL_WEBAUTHN_ORIGINS is required when PANEL_WEBAUTHN_RP_ID is set")
	}
	return WebAuthnConfig{Enabled: true, RPID: rpID, Origins: origins}, nil
}

func (c Config) webAuthnOrigins(raw string, strict bool) ([]string, error) {
	var values []string
	if raw != "" {
		values = strings.Split(raw, ",")
	} else {
		if frontend, err := c.PanelFrontendURL(); err != nil {
			return nil, err
		} else if frontend != "" {
			values = append(values, frontend)
		}
		if backend, err := c.PanelBackendURL(); err != nil {
			return nil, err
		} else if backend != "" {
			values = append(values, backend)
		}
	}

	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		origin, err := parseOriginBase(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("config: PANEL_WEBAUTHN_ORIGINS: %w", err)
		}
		if err := validateWebAuthnOrigin(origin); err != nil {
			if strict {
				return nil, fmt.Errorf("config: PANEL_WEBAUTHN_ORIGINS: %w", err)
			}
			continue
		}
		if !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	return out, nil
}

func parseOriginBase(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if u.User != nil {
		return "", fmt.Errorf("userinfo is not allowed")
	}
	if u.Host == "" {
		return "", fmt.Errorf("host is required")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("query and fragment are not allowed")
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	if path != "" {
		return "", fmt.Errorf("path is not allowed")
	}
	return u.Scheme + "://" + u.Host, nil
}

func validateWebAuthnOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLocalWebAuthnHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("origin %q must use https unless it is localhost", origin)
}

func validateWebAuthnRPID(rpID string) error {
	if rpID == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.Contains(rpID, "://") || strings.ContainsAny(rpID, "/?#") {
		return fmt.Errorf("must be a hostname without scheme or path")
	}
	if _, _, err := net.SplitHostPort(rpID); err == nil {
		return fmt.Errorf("must not include a port")
	}
	if strings.Contains(rpID, ":") {
		return fmt.Errorf("must not include a port")
	}
	return nil
}

func originHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func isLocalWebAuthnHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
