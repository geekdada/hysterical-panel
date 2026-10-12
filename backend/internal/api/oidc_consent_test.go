package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func (env *oidcTestEnv) panelRequest(method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", env.session)
	return env.do(req)
}

func (env *oidcTestEnv) panelGet(path string) *httptest.ResponseRecorder {
	return env.panelRequest(http.MethodGet, path)
}

func (env *oidcTestEnv) panelPost(path string) *httptest.ResponseRecorder {
	return env.panelRequest(http.MethodPost, path)
}

// approveCode runs /authorize and approves it as the env's User, returning
// the code sent back to the app.
func (env *oidcTestEnv) approveCode(overrides map[string]string) string {
	env.t.Helper()
	handle := env.startAuthorization(overrides)
	res := env.panelPost("/api/panel/oidc/requests/" + handle + "/approve")
	decision := decodeBody[OIDCAuthorizationDecision](env.t, res)
	location, err := url.Parse(decision.RedirectTo)
	if res.Code != http.StatusOK || err != nil || location.Query().Get("code") == "" {
		env.t.Fatalf("approve = %d %s", res.Code, res.Body.String())
	}
	return location.Query().Get("code")
}

func TestOIDCConsentShowsTheRequestAndApprovesIt(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	res := env.panelGet("/api/panel/oidc/requests/" + handle)
	view := decodeBody[OIDCAuthorizationRequest](t, res)
	if res.Code != http.StatusOK || view.ClientName != "Help center" || view.RedirectOrigin != "https://help.example" ||
		!slices.Equal(view.Scopes, []string{"openid", "email"}) || !view.ConsentRequired || view.ReauthenticationRequired || view.PromptNone {
		t.Fatalf("view = %d %+v", res.Code, view)
	}

	res = env.panelPost("/api/panel/oidc/requests/" + handle + "/approve")
	decision := decodeBody[OIDCAuthorizationDecision](t, res)
	location, err := url.Parse(decision.RedirectTo)
	if res.Code != http.StatusOK || err != nil || !strings.HasPrefix(decision.RedirectTo, helpCenterCallback+"?") {
		t.Fatalf("approve = %d %s", res.Code, res.Body.String())
	}
	if q := location.Query(); q.Get("code") == "" || q.Get("state") != "rp-state" || q.Get("iss") != oidcTestIssuer {
		t.Fatalf("redirect query = %v", q)
	}
	if res := env.panelPost("/api/panel/oidc/requests/" + handle + "/approve"); res.Code != http.StatusNotFound {
		t.Fatalf("second approve = %d", res.Code)
	}

	next := env.startAuthorization(nil)
	if view := decodeBody[OIDCAuthorizationRequest](t, env.panelGet("/api/panel/oidc/requests/"+next)); view.ConsentRequired {
		t.Fatal("remembered consent was asked again")
	}
}

func TestOIDCConsentNeedsAUsablePanelSession(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	if res := env.get("/api/panel/oidc/requests/" + handle); res.Code != http.StatusUnauthorized {
		t.Fatalf("without a session = %d", res.Code)
	}
	env.user.Set("verified", false)
	if err := env.app.Save(env.user); err != nil {
		t.Fatalf("unverify: %v", err)
	}
	if res := env.panelGet("/api/panel/oidc/requests/" + handle); res.Code != http.StatusForbidden {
		t.Fatalf("unverified user = %d", res.Code)
	}
}

func TestOIDCConsentDenyAndAbort(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	decision := decodeBody[OIDCAuthorizationDecision](t, env.panelPost("/api/panel/oidc/requests/"+handle+"/deny"))
	if location, err := url.Parse(decision.RedirectTo); err != nil || location.Query().Get("error") != "access_denied" {
		t.Fatalf("deny = %+v", decision)
	}

	silent := env.startAuthorization(map[string]string{"prompt": "none"})
	res := env.do(httptest.NewRequest(http.MethodPost, "/api/panel/oidc/requests/"+silent+"/abort", nil))
	decision = decodeBody[OIDCAuthorizationDecision](t, res)
	if location, err := url.Parse(decision.RedirectTo); res.Code != http.StatusOK || err != nil || location.Query().Get("error") != "login_required" {
		t.Fatalf("abort = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCConsentReturns404WhileTheProviderIsOff(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	env.setEnabled(false)
	if res := env.panelGet("/api/panel/oidc/requests/" + handle); res.Code != http.StatusNotFound {
		t.Fatalf("view while off = %d", res.Code)
	}
	if res := env.panelPost("/api/panel/oidc/requests/" + handle + "/approve"); res.Code != http.StatusNotFound {
		t.Fatalf("approve while off = %d", res.Code)
	}
}
