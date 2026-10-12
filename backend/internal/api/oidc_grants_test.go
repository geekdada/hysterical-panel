package api

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestOIDCGrantsListAndRevokeThroughThePanel(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	tokens := env.tokens(nil)
	path := "/api/panel/users/" + env.user.Id + "/oidc-grants"

	res := env.panelGet(path)
	grants := decodeBody[[]OIDCGrant](t, res)
	if res.Code != http.StatusOK || len(grants) != 1 || grants[0].ClientName != "Help center" ||
		!slices.Equal(grants[0].Scopes, []string{"openid", "email"}) || grants[0].Created == "" || grants[0].LastUsedAt == nil {
		t.Fatalf("list = %d %s", res.Code, res.Body.String())
	}

	if res := env.panelRequest(http.MethodDelete, path+"/"+grants[0].ID); res.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", res.Code, res.Body.String())
	}
	if res := env.userInfo("Bearer " + tokens.AccessToken); res.Code != http.StatusUnauthorized {
		t.Fatalf("userinfo after revoke = %d", res.Code)
	}
	if res := env.panelRequest(http.MethodDelete, path+"/"+grants[0].ID); res.Code != http.StatusNotFound {
		t.Fatalf("second revoke = %d %s", res.Code, res.Body.String())
	}
	if res := env.panelGet(path); len(decodeBody[[]OIDCGrant](t, res)) != 0 {
		t.Fatalf("list after revoke = %s", res.Body.String())
	}
}

func TestOIDCGrantsAreOwnerOrAdminOnly(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	env.approveCode(nil)
	other := newUsersTestRecord(t, env.app, "other-oidc@example.com", "OtherOIDCSecret123")
	session, err := issueSessionToken(env.app, other)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	grants := decodeBody[[]OIDCGrant](t, env.panelGet("/api/panel/users/"+env.user.Id+"/oidc-grants"))
	if len(grants) != 1 {
		t.Fatalf("grants = %+v", grants)
	}
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/panel/users/"+env.user.Id+"/oidc-grants", nil),
		httptest.NewRequest(http.MethodDelete, "/api/panel/users/"+env.user.Id+"/oidc-grants/"+grants[0].ID, nil),
		// The other User's own path must not reach a grant that is not theirs.
		httptest.NewRequest(http.MethodDelete, "/api/panel/users/"+other.Id+"/oidc-grants/"+grants[0].ID, nil),
	} {
		req.Header.Set("Authorization", session)
		if res := env.do(req); res.Code != http.StatusForbidden && res.Code != http.StatusNotFound {
			t.Fatalf("%s %s = %d", req.Method, req.URL.Path, res.Code)
		}
	}
	if grants := decodeBody[[]OIDCGrant](t, env.panelGet("/api/panel/users/"+env.user.Id+"/oidc-grants")); len(grants) != 1 {
		t.Fatalf("grant was revoked by another user: %+v", grants)
	}
}
