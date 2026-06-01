package config

import (
	"testing"
)

func TestLoad_requiresPanelMasterKey(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when PANEL_MASTER_KEY is empty")
	}
}

func TestLoad_ok(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("MMDB_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PanelMasterKey != "test-secret" {
		t.Fatalf("PanelMasterKey = %q, want test-secret", cfg.PanelMasterKey)
	}
	if cfg.MMDBDir != "mmdb" {
		t.Fatalf("MMDBDir = %q, want mmdb", cfg.MMDBDir)
	}
}

func TestLoad_allowsMMDBDirOverride(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("MMDB_DIR", "/opt/mmdb")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MMDBDir != "/opt/mmdb" {
		t.Fatalf("MMDBDir = %q, want /opt/mmdb", cfg.MMDBDir)
	}
}
