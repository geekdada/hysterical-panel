package api

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"hysterical-panel/internal/oidc"
)

func TestDisablingUserRevokesGrantsOnAnySave(t *testing.T) {
	cases := []struct {
		field  string
		value  any
		reason string
	}{
		{"status", "disabled", oidc.RevokeReasonUserDisabled},
		{"verified", false, oidc.RevokeReasonUserUnverified},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			env := newOIDCTestEnv(t, map[string]any{"refresh_enabled": true})
			tokens := env.tokens(map[string]string{"scope": "openid email offline_access"})
			activeGrants := func() int {
				t.Helper()
				grants, err := env.app.FindRecordsByFilter(oidc.GrantsCollection, "revoked_at = ''", "", 0, 0)
				if err != nil {
					t.Fatalf("grants: %v", err)
				}
				return len(grants)
			}

			user, err := env.app.FindRecordById("users", env.user.Id)
			if err != nil {
				t.Fatalf("load user: %v", err)
			}
			user.Set("last_connected_at", time.Now().UTC())
			if err := env.app.Save(user); err != nil || activeGrants() != 1 {
				t.Fatalf("an unrelated save revoked the grant (err %v)", err)
			}

			user.Set(c.field, c.value)
			if err := env.app.Save(user); err != nil {
				t.Fatalf("save: %v", err)
			}
			grants, err := env.app.FindAllRecords(oidc.GrantsCollection)
			if err != nil || len(grants) != 1 || grants[0].GetString("revoke_reason") != c.reason {
				t.Fatalf("grants after the save = %d, %v", len(grants), err)
			}

			user.Set("status", "active")
			user.Set("verified", true)
			if err := env.app.Save(user); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if res := env.userInfo("Bearer " + tokens.AccessToken); res.Code != http.StatusUnauthorized {
				t.Fatalf("userinfo after re-enabling = %d", res.Code)
			}
			res := env.postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens.RefreshToken}})
			if res.Code != http.StatusBadRequest {
				t.Fatalf("refresh after re-enabling = %d", res.Code)
			}
		})
	}
}
