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
	return cfg, nil
}
