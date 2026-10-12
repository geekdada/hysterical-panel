package api

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/oidc"
	"hysterical-panel/internal/token"
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

func TestOIDCLazyPinKeepsAConcurrentSwitchOff(t *testing.T) {
	env := newOIDCRouterEnv(t)
	stale, err := env.h.settingsRecord()
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	stale.Set("oidc_enabled", true)
	if err := env.app.Save(stale); err != nil {
		t.Fatalf("enable in place: %v", err)
	}
	// An admin turns the provider off while a request still holds the
	// record it loaded with an empty pin.
	env.setEnabled(false)

	if err := env.h.pinOIDCIssuer(stale, oidcTestIssuer); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if s := env.h.loadSettings(); s.OIDCEnabled || s.OIDCIssuer != "" {
		t.Fatalf("after the pin: enabled=%v issuer=%q", s.OIDCEnabled, s.OIDCIssuer)
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

type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// postClientForm sends a back-channel request with client_secret_basic.
func (env *oidcTestEnv) postClientForm(path string, form url.Values, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(env.client.ClientID), url.QueryEscape(secret))
	return env.do(req)
}

func (env *oidcTestEnv) postToken(form url.Values) *httptest.ResponseRecorder {
	return env.postClientForm("/api/oidc/token", form, env.secret)
}

func codeForm(code string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {helpCenterCallback},
		"code_verifier": {oidcTestVerifier},
	}
}

// tokens runs the whole code flow as the env's User.
func (env *oidcTestEnv) tokens(overrides map[string]string) oidc.TokenResponse {
	env.t.Helper()
	res := env.postToken(codeForm(env.approveCode(overrides)))
	if res.Code != http.StatusOK {
		env.t.Fatalf("token = %d %s", res.Code, res.Body.String())
	}
	return decodeBody[oidc.TokenResponse](env.t, res)
}

// verifyIDToken checks an ID token the way a relying party does, with the
// served JWKS and the issuer from discovery.
func (env *oidcTestEnv) verifyIDToken(raw string) jwt.MapClaims {
	env.t.Helper()
	jwks := decodeBody[oidc.JWKS](env.t, env.get("/api/oidc/jwks"))
	issuer := decodeBody[oidc.ProviderMetadata](env.t, env.get("/api/oidc/.well-known/openid-configuration")).Issuer
	parsed, err := jwt.Parse(raw, func(tok *jwt.Token) (any, error) {
		for _, key := range jwks.Keys {
			if key.KeyID == tok.Header["kid"] {
				return rsaPublicKey(key)
			}
		}
		return nil, errors.New("kid is not in the jwks")
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(issuer), jwt.WithAudience(env.client.ClientID), jwt.WithExpirationRequired())
	if err != nil {
		env.t.Fatalf("verify id token: %v", err)
	}
	return parsed.Claims.(jwt.MapClaims)
}

func rsaPublicKey(key oidc.JWK) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(key.Modulus)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(key.Exponent)
	if err != nil {
		return nil, err
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
}

func TestOIDCCodeFlowEndToEnd(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	code := env.approveCode(nil)
	res := env.postToken(codeForm(code))
	if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" || res.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("token = %d %v %s", res.Code, res.Header(), res.Body.String())
	}
	tokens := decodeBody[oidc.TokenResponse](t, res)
	claims := env.verifyIDToken(tokens.IDToken)
	if claims["sub"] != env.user.Id || claims["email"] != "oidc-user@example.com" || claims["nonce"] != "rp-nonce" ||
		tokens.Scope != "openid email" || tokens.RefreshToken != "" {
		t.Fatalf("claims = %v, tokens = %+v", claims, tokens)
	}

	res = env.postToken(codeForm(code))
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusBadRequest || body.Error != "invalid_grant" {
		t.Fatalf("replay = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCTokenEndpointRejectsBadRequests(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	res := env.postClientForm("/api/oidc/token", codeForm("unused"), "wrong-secret")
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusUnauthorized || body.Error != "invalid_client" ||
		res.Header().Get("WWW-Authenticate") != `Basic realm="oidc"` {
		t.Fatalf("wrong secret = %d %v %s", res.Code, res.Header(), res.Body.String())
	}

	cases := map[string]struct {
		form url.Values
		want string
	}{
		"unsupported grant type": {url.Values{"grant_type": {"password"}}, "unsupported_grant_type"},
		"missing grant type":     {url.Values{"code": {"unused"}}, "invalid_request"},
		"repeated parameter":     {url.Values{"grant_type": {"authorization_code", "authorization_code"}}, "invalid_request"},
	}
	for name, c := range cases {
		res := env.postToken(c.form)
		if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusBadRequest || body.Error != c.want {
			t.Errorf("%s = %d %s", name, res.Code, res.Body.String())
		}
	}
}

func TestOIDCTokenEndpointLimitsFailuresButNotTheRightSecret(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	for range oidcClientFailureMax {
		if res := env.postClientForm("/api/oidc/token", codeForm("unused"), "wrong-secret"); res.Code != http.StatusUnauthorized {
			t.Fatalf("failure = %d %s", res.Code, res.Body.String())
		}
	}
	res := env.postClientForm("/api/oidc/token", codeForm("unused"), "wrong-secret")
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusTooManyRequests || body.Error != "temporarily_unavailable" ||
		res.Header().Get("Retry-After") == "" {
		t.Fatalf("failure past the budget = %d %v %s", res.Code, res.Header(), res.Body.String())
	}
	// Behind an untrusted proxy every caller shares one address, so failures
	// from anyone must not lock out an app that knows its secret.
	if res := env.postToken(codeForm(env.approveCode(nil))); res.Code != http.StatusOK {
		t.Fatalf("right secret after failures = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCRefreshThroughTheTokenEndpoint(t *testing.T) {
	env := newOIDCTestEnv(t, map[string]any{"refresh_enabled": true})
	first := env.tokens(map[string]string{"scope": "openid email offline_access"})
	refresh := func(raw string) *httptest.ResponseRecorder {
		return env.postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {raw}})
	}

	res := refresh(first.RefreshToken)
	next := decodeBody[oidc.TokenResponse](t, res)
	if res.Code != http.StatusOK || next.RefreshToken == "" || next.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh = %d %s", res.Code, res.Body.String())
	}
	if claims := env.verifyIDToken(next.IDToken); claims["sub"] != env.user.Id {
		t.Fatalf("refreshed claims = %v", claims)
	}

	res = refresh(first.RefreshToken)
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusBadRequest || body.Error != "invalid_grant" {
		t.Fatalf("replay = %d %s", res.Code, res.Body.String())
	}
	if res := refresh(next.RefreshToken); res.Code != http.StatusBadRequest {
		t.Fatalf("the replay left the family usable: %d", res.Code)
	}
}

func (env *oidcTestEnv) userInfo(authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/oidc/userinfo", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return env.do(req)
}

func TestOIDCUserInfoAndRevocation(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	tokens := env.tokens(nil)

	res := env.userInfo("Bearer " + tokens.AccessToken)
	claims := decodeBody[map[string]any](t, res)
	if res.Code != http.StatusOK || claims["sub"] != env.user.Id || claims["email"] != "oidc-user@example.com" ||
		claims["email_verified"] != true || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("userinfo = %d %s", res.Code, res.Body.String())
	}
	post := httptest.NewRequest(http.MethodPost, "/api/oidc/userinfo", nil)
	post.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	if res := env.do(post); res.Code != http.StatusOK {
		t.Fatalf("POST userinfo = %d", res.Code)
	}

	if res := env.userInfo(""); res.Code != http.StatusUnauthorized || res.Header().Get("WWW-Authenticate") != `Bearer realm="oidc"` {
		t.Fatalf("without a token = %d %v", res.Code, res.Header())
	}
	if res := env.userInfo(env.session); res.Code != http.StatusUnauthorized {
		t.Fatalf("a panel session at userinfo = %d", res.Code)
	}

	if res := env.postClientForm("/api/oidc/revoke", url.Values{}, env.secret); res.Code != http.StatusBadRequest {
		t.Fatalf("revoke without a token = %d", res.Code)
	}
	if res := env.postClientForm("/api/oidc/revoke", url.Values{"token": {tokens.AccessToken}}, env.secret); res.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", res.Code, res.Body.String())
	}
	res = env.userInfo("Bearer " + tokens.AccessToken)
	if res.Code != http.StatusUnauthorized || res.Header().Get("WWW-Authenticate") != `Bearer realm="oidc", error="invalid_token"` {
		t.Fatalf("revoked token = %d %v", res.Code, res.Header())
	}
}

func TestOIDCAuthorizeTellsTheConsentPageAboutPromptNone(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	for prompt, want := range map[string]string{"none": "none", "": "", "consent": "", "login": ""} {
		res := env.get("/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"prompt": prompt}).Encode())
		location, err := url.Parse(res.Header().Get("Location"))
		if res.Code != http.StatusSeeOther || err != nil || location.Path != "/oidc/authorize" ||
			location.Query().Get("request") == "" || location.Query().Get("prompt") != want {
			t.Errorf("prompt=%q: %d %q", prompt, res.Code, res.Header().Get("Location"))
		}
	}
}
