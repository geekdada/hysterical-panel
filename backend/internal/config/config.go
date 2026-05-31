// Package config loads process environment variables for the panel backend.
// Values must already be exported (e.g. direnv, systemd, or make include .env).
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds required and optional settings parsed from the environment.
type Config struct {
	// PanelMasterKey encrypts node api_secret values (AES-GCM via SHA-256 derived key).
	PanelMasterKey string `env:"PANEL_MASTER_KEY,notEmpty"`
}

// Load parses the current process environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
