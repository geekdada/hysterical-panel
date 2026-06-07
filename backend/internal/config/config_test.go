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

func TestPanelBackendURL_defaultEmpty(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_BACKEND_URL_BASE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := cfg.PanelBackendURL()
	if err != nil {
		t.Fatalf("PanelBackendURL: %v", err)
	}
	if got != "" {
		t.Fatalf("PanelBackendURL = %q, want empty", got)
	}
}

func TestPanelBackendURL_parsesURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://localhost:8090", "http://localhost:8090"},
		{"https://panel.example.com/", "https://panel.example.com"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Setenv("PANEL_MASTER_KEY", "test-secret")
			t.Setenv("PANEL_BACKEND_URL_BASE", tc.in)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			got, err := cfg.PanelBackendURL()
			if err != nil {
				t.Fatalf("PanelBackendURL: %v", err)
			}
			if got != tc.want {
				t.Fatalf("PanelBackendURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPanelBackendURL_rejectsInvalid(t *testing.T) {
	tests := []string{
		"ftp://localhost:8090",
		"http://localhost:8090/api",
		"not-a-url",
		"http://",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			t.Setenv("PANEL_MASTER_KEY", "test-secret")
			t.Setenv("PANEL_BACKEND_URL_BASE", in)

			_, err := Load()
			if err == nil {
				t.Fatal("expected error")
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

func TestWebAuthn_disabledByDefault(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_FRONTEND_URL_BASE", "")
	t.Setenv("PANEL_BACKEND_URL_BASE", "")
	t.Setenv("PANEL_WEBAUTHN_RP_ID", "")
	t.Setenv("PANEL_WEBAUTHN_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wa, err := cfg.WebAuthn()
	if err != nil {
		t.Fatalf("WebAuthn: %v", err)
	}
	if wa.Enabled {
		t.Fatalf("WebAuthn.Enabled = true, want false")
	}
}

func TestWebAuthn_explicitConfig(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_WEBAUTHN_RP_ID", "panel.example.com")
	t.Setenv("PANEL_WEBAUTHN_ORIGINS", "https://panel.example.com,https://panel.example.com/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wa, err := cfg.WebAuthn()
	if err != nil {
		t.Fatalf("WebAuthn: %v", err)
	}
	if !wa.Enabled || wa.RPID != "panel.example.com" {
		t.Fatalf("WebAuthn = %#v", wa)
	}
	if len(wa.Origins) != 1 || wa.Origins[0] != "https://panel.example.com" {
		t.Fatalf("Origins = %#v", wa.Origins)
	}
}

func TestWebAuthn_infersFromFrontendURL(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_FRONTEND_URL_BASE", "https://panel.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wa, err := cfg.WebAuthn()
	if err != nil {
		t.Fatalf("WebAuthn: %v", err)
	}
	if !wa.Enabled || wa.RPID != "panel.example.com" {
		t.Fatalf("WebAuthn = %#v", wa)
	}
	if len(wa.Origins) != 1 || wa.Origins[0] != "https://panel.example.com" {
		t.Fatalf("Origins = %#v", wa.Origins)
	}
}

func TestWebAuthn_allowsLocalHTTP(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_WEBAUTHN_RP_ID", "localhost")
	t.Setenv("PANEL_WEBAUTHN_ORIGINS", "http://localhost:3000")

	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestWebAuthn_rejectsInsecureRemoteHTTP(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_WEBAUTHN_RP_ID", "panel.example.com")
	t.Setenv("PANEL_WEBAUTHN_ORIGINS", "http://panel.example.com")

	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebAuthn_implicitInsecureRemoteHTTPDisablesPasskeys(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_FRONTEND_URL_BASE", "http://panel.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wa, err := cfg.WebAuthn()
	if err != nil {
		t.Fatalf("WebAuthn: %v", err)
	}
	if wa.Enabled {
		t.Fatalf("WebAuthn.Enabled = true, want false")
	}
}

func TestWebAuthn_rejectsRPIDPort(t *testing.T) {
	t.Setenv("PANEL_MASTER_KEY", "test-secret")
	t.Setenv("PANEL_WEBAUTHN_RP_ID", "panel.example.com:443")
	t.Setenv("PANEL_WEBAUTHN_ORIGINS", "https://panel.example.com")

	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}
