package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/oidc"
)

const (
	oidcTestAPI      = "https://api.example.com"
	oidcTestFrontend = "https://panel.example.com"
	oidcTestIssuer   = oidcTestAPI + "/api/oidc"
)

// oidcTestEnv serves the real router, so tests cover the serving gate, the
// panel's auth middleware and the handlers together. The Connected App and
// User fields are filled by newOIDCTestEnv.
type oidcTestEnv struct {
	t       *testing.T
	app     core.App
	h       *Handlers
	mux     http.Handler
	client  OIDCClient
	secret  string
	user    *core.Record
	session string
}

func newOIDCRouterEnv(t *testing.T) *oidcTestEnv {
	t.Helper()
	app := newMigratedTestApp(t)
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	box, err := cryptobox.New("oidc-test-master-key")
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	h := Register(&core.ServeEvent{App: app, Router: r}, app, box, nil, nil, nil, nil,
		PanelConfigResponse{APIURL: oidcTestAPI, FrontendURL: oidcTestFrontend})
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("mux: %v", err)
	}
	return &oidcTestEnv{t: t, app: app, h: h, mux: mux}
}

func (env *oidcTestEnv) setEnabled(enabled bool) {
	env.t.Helper()
	e, _ := panelEvent(env.t, env.app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": enabled}, nil)
	if err := env.h.updateSettings(e); err != nil {
		env.t.Fatalf("set oidc_enabled=%v: %v", enabled, err)
	}
}

func (env *oidcTestEnv) do(req *http.Request) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	env.mux.ServeHTTP(res, req)
	return res
}

func (env *oidcTestEnv) get(path string) *httptest.ResponseRecorder {
	return env.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func TestOIDCEndpointsReturn404UnlessServing(t *testing.T) {
	env := newOIDCRouterEnv(t)
	paths := []string{"/api/oidc/.well-known/openid-configuration", "/api/oidc/jwks"}
	for _, path := range paths {
		if res := env.get(path); res.Code != http.StatusNotFound {
			t.Fatalf("off: %s = %d", path, res.Code)
		}
	}
	env.setEnabled(true)
	for _, path := range paths {
		if res := env.get(path); res.Code != http.StatusOK {
			t.Fatalf("on: %s = %d %s", path, res.Code, res.Body.String())
		}
	}
	env.h.publicConfig.FrontendURL = ""
	for _, path := range paths {
		if res := env.get(path); res.Code != http.StatusNotFound {
			t.Fatalf("unavailable: %s = %d", path, res.Code)
		}
	}
}

func TestOIDCEndpointsReturn404AfterIssuerChange(t *testing.T) {
	env := newOIDCRouterEnv(t)
	env.setEnabled(true)
	env.h.publicConfig.APIURL = "https://moved.example.com"
	if res := env.get("/api/oidc/.well-known/openid-configuration"); res.Code != http.StatusNotFound {
		t.Fatalf("after the move = %d", res.Code)
	}
	if reason := env.h.settingsResponse(env.h.loadSettings()).OIDCUnavailableReason; reason != "issuer_changed" {
		t.Fatalf("reason = %q, want issuer_changed", reason)
	}

	env.setEnabled(false)
	env.setEnabled(true)
	res := env.get("/api/oidc/.well-known/openid-configuration")
	var meta oidc.ProviderMetadata
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &meta) != nil || meta.Issuer != "https://moved.example.com/api/oidc" {
		t.Fatalf("after re-enabling = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCOutOfBandEnablePinsIssuerAndCreatesKey(t *testing.T) {
	env := newOIDCRouterEnv(t)
	rec, err := env.h.settingsRecord()
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	rec.Set("oidc_enabled", true)
	if err := env.app.Save(rec); err != nil {
		t.Fatalf("enable in place: %v", err)
	}

	res := env.get("/api/oidc/jwks")
	var jwks oidc.JWKS
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &jwks) != nil || len(jwks.Keys) != 1 {
		t.Fatalf("jwks = %d %s", res.Code, res.Body.String())
	}
	if pinned := env.h.loadSettings().OIDCIssuer; pinned != oidcTestIssuer {
		t.Fatalf("pinned issuer = %q", pinned)
	}
}

func TestRateLimiterBlockedPeeksWithoutCounting(t *testing.T) {
	limiter := newPasskeyRateLimiter(2, time.Minute)
	now := time.Now()
	for hits := 0; hits < 2; hits++ {
		if limiter.blocked("192.0.2.1", now) {
			t.Fatalf("blocked after %d hits", hits)
		}
		limiter.allow("192.0.2.1", now)
	}
	if !limiter.blocked("192.0.2.1", now) {
		t.Fatal("not blocked at the limit")
	}
	if limiter.blocked("192.0.2.1", now.Add(2*time.Minute)) {
		t.Fatal("still blocked after the window")
	}
}
