// Package config loads process environment variables for the panel backend.
// Values must already be exported (e.g. direnv, systemd, or make include .env).
package config

import (
	"fmt"
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

	// PanelCorsMaxAge is Access-Control-Max-Age for preflight caching (seconds).
	// Browsers may cap (e.g. Chrome at 7200). 0 disables the header.
	PanelCorsMaxAge int `env:"PANEL_CORS_MAX_AGE" envDefault:"7200"`

	// DataDir is PocketBase's data directory. Empty falls back to ./pb_data.
	DataDir string `env:"PB_DATA_DIR"`

	// MMDBDir contains the bundled MMDB files used for live IP metadata.
	MMDBDir string `env:"MMDB_DIR" envDefault:"mmdb"`
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
	origin, err := parseFrontendOrigin(raw)
	if err != nil {
		return nil, fmt.Errorf("config: PANEL_FRONTEND_URL_BASE: %w", err)
	}
	return []string{origin}, nil
}

func parseFrontendOrigin(raw string) (string, error) {
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
