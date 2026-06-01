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
	if cfg.PanelCorsMaxAge != 7200 {
		t.Fatalf("PanelCorsMaxAge = %d, want 7200", cfg.PanelCorsMaxAge)
	}
}

func TestLoad_rejectsNegativeCorsMaxAge(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_CORS_MAX_AGE", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
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

func TestServeAllowedOrigins_defaultWildcard(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_FRONTEND_URL_BASE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	origins, err := cfg.ServeAllowedOrigins()
	if err != nil {
		t.Fatalf("ServeAllowedOrigins: %v", err)
	}
	if len(origins) != 1 || origins[0] != "*" {
		t.Fatalf("origins = %v, want [*]", origins)
	}
}

func TestServeAllowedOrigins_parsesURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://localhost:3000", "http://localhost:3000"},
		{"https://panel.example.com/", "https://panel.example.com"},
		{"http://127.0.0.1:3000", "http://127.0.0.1:3000"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Setenv("PANEL_MASTER_KEY", "test-secret")
			t.Setenv("PANEL_FRONTEND_URL_BASE", tc.in)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			origins, err := cfg.ServeAllowedOrigins()
			if err != nil {
				t.Fatalf("ServeAllowedOrigins: %v", err)
			}
			if len(origins) != 1 || origins[0] != tc.want {
				t.Fatalf("origins = %v, want [%s]", origins, tc.want)
			}
		})
	}
}

func TestServeAllowedOrigins_rejectsInvalid(t *testing.T) {
	tests := []string{
		"ftp://localhost:3000",
		"http://localhost:3000/app",
		"not-a-url",
		"http://",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			t.Setenv("PANEL_MASTER_KEY", "test-secret")
			t.Setenv("PANEL_FRONTEND_URL_BASE", in)

			_, err := Load()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
