package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/oidc"
)

func newOIDCHandlers(t *testing.T, app core.App, apiURL, frontendURL string) *Handlers {
	t.Helper()
	box, err := cryptobox.New("oidc-test-master-key")
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	return &Handlers{app: app, box: box, publicConfig: PanelConfigResponse{APIURL: apiURL, FrontendURL: frontendURL}}
}

// panelEvent builds a handler event the way the router would, with an
// optional path id and authenticated record.
func panelEvent(t *testing.T, app core.App, method, path, id string, body any, auth *core.Record) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	raw := []byte{}
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if id != "" {
		req.SetPathValue("id", id)
	}
	return &core.RequestEvent{App: app, Auth: auth, Event: router.Event{Request: req, Response: response}}, response
}

// assertAPIError compares the message the way the frontend does: PocketBase
// capitalises it and appends a period.
func assertAPIError(t *testing.T, err error, status int, message string) {
	t.Helper()
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an API error", err)
	}
	got := strings.ToLower(strings.TrimSuffix(apiErr.Message, "."))
	if apiErr.Status != status || got != message {
		t.Fatalf("API error = %d %q, want %d %q", apiErr.Status, got, status, message)
	}
}

func TestEnablingOIDCRequiresIssuerAndConsentOrigins(t *testing.T) {
	cases := map[string]struct{ api, frontend, reason string }{
		"no backend url":     {"", "https://panel.example.com", "backend_url_missing"},
		"plain http backend": {"http://api.example.com", "https://panel.example.com", "backend_url_insecure"},
		"no frontend url":    {"https://api.example.com", "", "frontend_url_missing"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			app := newMigratedTestApp(t)
			h := newOIDCHandlers(t, app, c.api, c.frontend)
			e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
			assertAPIError(t, h.updateSettings(e), http.StatusBadRequest,
				"oidc provider requires an https panel_backend_url_base and panel_frontend_url_base")
			if h.loadSettings().OIDCEnabled {
				t.Fatal("the switch was stored")
			}
			if got := h.settingsResponse(h.loadSettings()).OIDCUnavailableReason; got != c.reason {
				t.Fatalf("reason = %q, want %q", got, c.reason)
			}
		})
	}
}

func TestEnablingOIDCCreatesSigningKeyAndReportsIssuer(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")
	e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("enable: %v", err)
	}
	var resp SettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OIDCEnabled || resp.OIDCIssuer != "https://api.example.com/api/oidc" || resp.OIDCUnavailableReason != "" {
		t.Fatalf("settings response = %+v", resp)
	}
	if jwks, err := oidc.PublicJWKS(app, time.Now()); err != nil || len(jwks.Keys) != 1 {
		t.Fatalf("jwks after enabling = %+v, err=%v", jwks, err)
	}

	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": false}, nil)
	if err := h.updateSettings(e); err != nil || h.loadSettings().OIDCEnabled {
		t.Fatalf("disable: err=%v enabled=%v", err, h.loadSettings().OIDCEnabled)
	}
}

func TestReEnablingOIDCIsANoOpAfterTheEnvironmentChanges(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")
	e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("enable: %v", err)
	}

	h.publicConfig.FrontendURL = ""
	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if !h.loadSettings().OIDCEnabled {
		t.Fatal("the switch was turned off")
	}
}

func TestEnablingOIDCReportsSigningKeyFailureAsServerError(t *testing.T) {
	app := newMigratedTestApp(t)
	keys, err := app.FindCollectionByNameOrId(oidc.SigningKeysCollection)
	if err != nil {
		t.Fatalf("find keys: %v", err)
	}
	if err := app.Delete(keys); err != nil {
		t.Fatalf("drop keys: %v", err)
	}
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")
	e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
	assertAPIError(t, h.updateSettings(e), http.StatusInternalServerError, "failed to prepare the oidc signing key")
	if h.loadSettings().OIDCEnabled {
		t.Fatal("the switch was stored without a signing key")
	}
}
func TestOIDCSwitchPinsAndClearsIssuer(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")
	for _, step := range []struct {
		enabled bool
		pinned  string
	}{{true, "https://api.example.com/api/oidc"}, {false, ""}} {
		e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": step.enabled}, nil)
		if err := h.updateSettings(e); err != nil {
			t.Fatalf("oidc_enabled=%v: %v", step.enabled, err)
		}
		if got := h.loadSettings().OIDCIssuer; got != step.pinned {
			t.Fatalf("oidc_enabled=%v pinned %q, want %q", step.enabled, got, step.pinned)
		}
	}
}
