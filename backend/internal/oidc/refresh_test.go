package oidc

import (
	"net/url"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

// exchangedAt is when exchangedTokens issues tokens.
var exchangedAt = testNow.Add(time.Second)

var offlineScope = map[string]string{"scope": "openid email offline_access"}

func refreshRequest(client *core.Record, box *cryptobox.Box, raw string, now time.Time, overrides map[string]string) TokenRequest {
	form := url.Values{"grant_type": {GrantTypeRefreshToken}, "refresh_token": {raw}}
	applyOverrides(form, overrides)
	return TokenRequest{Client: client, Form: form, Issuer: testIssuer, Box: box, Now: now}
}

// refreshFixture is a refresh-enabled app and a User holding tokens with
// offline_access, issued at exchangedAt.
type refreshFixture struct {
	app    core.App
	box    *cryptobox.Box
	client *core.Record
	user   *core.Record
	tokens *TokenResponse
}

func newRefreshFixture(t *testing.T) refreshFixture {
	t.Helper()
	f := refreshFixture{app: newTestApp(t), box: newTestBox(t, testMasterKey)}
	f.client, _ = newTestClient(t, f.app, map[string]any{"refresh_enabled": true})
	f.user = newTestUser(t, f.app, "refresh@example.com")
	f.tokens = exchangedTokens(t, f.app, f.box, f.client, f.user, offlineScope)
	return f
}

func (f refreshFixture) refresh(raw string, now time.Time, overrides map[string]string) (*TokenResponse, *Error) {
	return RefreshTokens(f.app, refreshRequest(f.client, f.box, raw, now, overrides))
}

func TestRefreshRotatesTheRefreshToken(t *testing.T) {
	f := newRefreshFixture(t)
	now := exchangedAt.Add(time.Hour)
	next, oauthErr := f.refresh(f.tokens.RefreshToken, now, nil)
	if oauthErr != nil {
		t.Fatalf("refresh: %v", oauthErr)
	}
	if next.RefreshToken == "" || next.RefreshToken == f.tokens.RefreshToken || next.AccessToken == f.tokens.AccessToken ||
		next.Scope != "openid email offline_access" {
		t.Fatalf("response = %+v", next)
	}
	old := tokenRecord(t, f.app, f.tokens.RefreshToken)
	if old.GetDateTime("rotated_at").IsZero() || tokenRecord(t, f.app, next.RefreshToken).GetString("family") != old.GetString("family") {
		t.Fatalf("old refresh token = %v", old.PublicExport())
	}
	claims := parseIDToken(t, f.app, f.box, f.client, next.IDToken, now)
	if claims["auth_time"] != float64(testNow.Add(-time.Minute).Unix()) || claims["iat"] != float64(now.Unix()) {
		t.Fatalf("claims = %v", claims)
	}
	if _, hasNonce := claims["nonce"]; hasNonce {
		t.Fatal("a refreshed ID token carries the original nonce")
	}
}

func TestRefreshReplayRevokesTheWholeFamily(t *testing.T) {
	f := newRefreshFixture(t)
	other := exchangedTokens(t, f.app, f.box, f.client, f.user, offlineScope)
	now := exchangedAt.Add(time.Hour)
	next, oauthErr := f.refresh(f.tokens.RefreshToken, now, nil)
	if oauthErr != nil {
		t.Fatalf("refresh: %v", oauthErr)
	}

	if _, oauthErr := f.refresh(f.tokens.RefreshToken, now, nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("replay: %v", oauthErr)
	}
	for name, raw := range map[string]string{
		"first access token":  f.tokens.AccessToken,
		"second access token": next.AccessToken,
		"rotated-in refresh":  next.RefreshToken,
	} {
		if !tokenRevoked(t, f.app, raw) {
			t.Errorf("%s still works after the replay", name)
		}
	}
	if _, oauthErr := f.refresh(next.RefreshToken, now, nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("refresh after the replay: %v", oauthErr)
	}
	if tokenRevoked(t, f.app, other.AccessToken) || tokenRevoked(t, f.app, other.RefreshToken) {
		t.Fatal("the replay revoked another family")
	}
}

func TestRefreshTokensExpire(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		f := newRefreshFixture(t)
		if _, oauthErr := f.refresh(f.tokens.RefreshToken, exchangedAt.Add(RefreshTokenIdleTTL), nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
			t.Fatalf("idle refresh token: %v", oauthErr)
		}
	})
	t.Run("absolute", func(t *testing.T) {
		f := newRefreshFixture(t)
		raw := f.tokens.RefreshToken
		step := RefreshTokenIdleTTL - 24*time.Hour
		for now := exchangedAt.Add(step); now.Before(exchangedAt.Add(RefreshTokenMaxTTL)); now = now.Add(step) {
			next, oauthErr := f.refresh(raw, now, nil)
			if oauthErr != nil {
				t.Fatalf("refresh after %v: %v", now.Sub(exchangedAt), oauthErr)
			}
			raw = next.RefreshToken
		}
		if expires := tokenRecord(t, f.app, raw).GetDateTime("expires_at").Time(); !expires.Equal(exchangedAt.Add(RefreshTokenMaxTTL)) {
			t.Fatalf("last refresh token expires at %v, want the absolute cap", expires)
		}
		if _, oauthErr := f.refresh(raw, exchangedAt.Add(RefreshTokenMaxTTL), nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
			t.Fatalf("refresh past the cap: %v", oauthErr)
		}
	})
}

func TestRefreshScopeMayOnlyNarrow(t *testing.T) {
	f := newRefreshFixture(t)
	now := exchangedAt.Add(time.Hour)
	for _, scope := range []string{"openid email profile", "email offline_access"} {
		if _, oauthErr := f.refresh(f.tokens.RefreshToken, now, map[string]string{"scope": scope}); oauthErr == nil || oauthErr.Code != InvalidScope {
			t.Errorf("scope %q: %v, want invalid_scope", scope, oauthErr)
		}
	}

	narrowed, oauthErr := f.refresh(f.tokens.RefreshToken, now, map[string]string{"scope": "openid"})
	if oauthErr != nil || narrowed.Scope != "openid" || narrowed.RefreshToken == "" {
		t.Fatalf("narrowed = %+v, %v", narrowed, oauthErr)
	}
	if claims := parseIDToken(t, f.app, f.box, f.client, narrowed.IDToken, now); claims["email"] != nil {
		t.Fatalf("narrowed ID token = %v", claims)
	}
	full, oauthErr := f.refresh(narrowed.RefreshToken, now, nil)
	if oauthErr != nil || full.Scope != "openid email offline_access" {
		t.Fatalf("the refresh token lost the granted scope: %+v, %v", full, oauthErr)
	}
}

func TestRefreshRejectsAccessTokens(t *testing.T) {
	f := newRefreshFixture(t)
	if _, oauthErr := f.refresh(f.tokens.AccessToken, exchangedAt, nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("access token as refresh token: %v", oauthErr)
	}
	if tokenRevoked(t, f.app, f.tokens.AccessToken) || tokenRevoked(t, f.app, f.tokens.RefreshToken) {
		t.Fatal("presenting an access token revoked the family")
	}
}

func TestRefreshStopsWhenTheAppOrUserCannotContinue(t *testing.T) {
	cases := []struct {
		name   string
		record func(refreshFixture) *core.Record
		field  string
		value  any
	}{
		{"refresh turned off", func(f refreshFixture) *core.Record { return f.client }, "refresh_enabled", false},
		{"app disabled", func(f refreshFixture) *core.Record { return f.client }, "enabled", false},
		{"user disabled", func(f refreshFixture) *core.Record { return f.user }, "status", "disabled"},
		{"user unverified", func(f refreshFixture) *core.Record { return f.user }, "verified", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			rec := c.record(f)
			rec.Set(c.field, c.value)
			if err := f.app.Save(rec); err != nil {
				t.Fatalf("save: %v", err)
			}
			if _, oauthErr := f.refresh(f.tokens.RefreshToken, exchangedAt.Add(time.Hour), nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
				t.Fatalf("refresh: %v, want invalid_grant", oauthErr)
			}
		})
	}
}
