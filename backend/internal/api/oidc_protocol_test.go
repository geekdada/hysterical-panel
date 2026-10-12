package api

import (
	"encoding/json"
	"hysterical-panel/internal/token"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// oidcTestVerifier and oidcTestChallenge are the PKCE example of RFC 7636
// appendix B.
const (
	oidcTestVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	oidcTestChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

// newOIDCTestEnv switches the provider on and adds a Connected App for
// helpCenterCallback and a User signed in to the panel.
func newOIDCTestEnv(t *testing.T, clientBody map[string]any) *oidcTestEnv {
	t.Helper()
	env := newOIDCRouterEnv(t)
	env.setEnabled(true)
	body := map[string]any{"name": "Help center", "redirect_uris": []string{helpCenterCallback}}
	maps.Copy(body, clientBody)
	created := createTestOIDCClient(t, env.h, nil, body)
	env.client, env.secret = created.Client, created.ClientSecret
	env.user = newUsersTestRecord(t, env.app, "oidc-user@example.com", "OIDCUserSecret123")
	session, err := issueSessionToken(env.app, env.user)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	env.session = session
	return env
}

// authorizeQuery is a valid authorization request for the env's app. An
// override replaces a value, and an empty override removes the parameter.
func (env *oidcTestEnv) authorizeQuery(overrides map[string]string) url.Values {
	q := url.Values{
		"client_id":             {env.client.ClientID},
		"redirect_uri":          {helpCenterCallback},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {"rp-state"},
		"nonce":                 {"rp-nonce"},
		"code_challenge":        {oidcTestChallenge},
		"code_challenge_method": {"S256"},
	}
	for key, value := range overrides {
		if value == "" {
			q.Del(key)
		} else {
			q.Set(key, value)
		}
	}
	return q
}

// startAuthorization runs /authorize and returns the handle the consent page
// receives.
func (env *oidcTestEnv) startAuthorization(overrides map[string]string) string {
	env.t.Helper()
	res := env.get("/api/oidc/authorize?" + env.authorizeQuery(overrides).Encode())
	location, err := url.Parse(res.Header().Get("Location"))
	if res.Code != http.StatusSeeOther || err != nil || location.Path != "/oidc/authorize" {
		env.t.Fatalf("authorize = %d %q", res.Code, res.Header().Get("Location"))
	}
	return location.Query().Get("request")
}

func TestOIDCAuthorizeHandsTheBrowserToTheConsentPage(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	const consentPrefix = oidcTestFrontend + "/oidc/authorize?request="
	res := env.get("/api/oidc/authorize?" + env.authorizeQuery(nil).Encode())
	location := res.Header().Get("Location")
	if res.Code != http.StatusSeeOther || !strings.HasPrefix(location, consentPrefix) {
		t.Fatalf("GET = %d %q", res.Code, location)
	}
	stored, err := env.app.FindAllRecords(oidc.AuthRequestsCollection)
	if err != nil || len(stored) != 1 || stored[0].GetString("request_hash") != token.Sha256Hex(strings.TrimPrefix(location, consentPrefix)) {
		t.Fatalf("stored requests = %d, err %v", len(stored), err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/oidc/authorize", strings.NewReader(env.authorizeQuery(nil).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if res := env.do(req); res.Code != http.StatusSeeOther || !strings.HasPrefix(res.Header().Get("Location"), consentPrefix) {
		t.Fatalf("POST = %d %q", res.Code, res.Header().Get("Location"))
	}
}

func TestOIDCAuthorizeErrorsNeverRedirectToUnregisteredURIs(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	res := env.get("/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"redirect_uri": "https://evil.example/cb"}).Encode())
	if res.Code != http.StatusSeeOther || res.Header().Get("Location") != oidcTestFrontend+"/oidc/error?reason=invalid_redirect_uri" {
		t.Fatalf("bad redirect_uri = %d %q", res.Code, res.Header().Get("Location"))
	}

	res = env.get("/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"code_challenge_method": "plain"}).Encode())
	location, err := url.Parse(res.Header().Get("Location"))
	if err != nil || res.Code != http.StatusSeeOther || !strings.HasPrefix(location.String(), helpCenterCallback+"?") {
		t.Fatalf("plain pkce = %d %q", res.Code, res.Header().Get("Location"))
	}
	if q := location.Query(); q.Get("error") != oidc.InvalidRequest || q.Get("state") != "rp-state" || q.Get("iss") != oidcTestIssuer {
		t.Fatalf("error query = %v", q)
	}
}

func TestOIDCAuthorizeIsRateLimited(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	path := "/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"client_id": "unknown"}).Encode()
	for range oidcAuthorizeRateMax {
		env.get(path)
	}
	if res := env.get(path); res.Header().Get("Location") != oidcTestFrontend+"/oidc/error?reason=rate_limited" {
		t.Fatalf("over the limit = %d %q", res.Code, res.Header().Get("Location"))
	}
}
