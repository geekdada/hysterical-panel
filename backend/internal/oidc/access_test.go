package oidc

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestLookupAccessTokenReturnsTheUserAndScope(t *testing.T) {
	f := newRefreshFixture(t)
	user, scope, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt)
	if err != nil || user.Id != f.user.Id || !slices.Equal(scope, []string{"openid", "email", "offline_access"}) {
		t.Fatalf("lookup = %v, %v, %v", user, scope, err)
	}
	claims := UserClaims(user, scope)
	if claims["sub"] != f.user.Id || claims["email"] != "refresh@example.com" || claims["email_verified"] != true {
		t.Fatalf("claims = %v", claims)
	}
	if claims := UserClaims(user, []string{"openid"}); len(claims) != 1 {
		t.Fatalf("claims without the email scope = %v", claims)
	}
}

func TestLookupAccessTokenRejectsRefreshTokens(t *testing.T) {
	f := newRefreshFixture(t)
	if _, _, err := LookupAccessToken(f.app, f.tokens.RefreshToken, exchangedAt); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh token at userinfo: %v", err)
	}
}

func TestLookupAccessTokenRejectsUnusableTokens(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		f := newRefreshFixture(t)
		if _, _, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt.Add(AccessTokenTTL)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expired: %v", err)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		f := newRefreshFixture(t)
		if _, _, err := LookupAccessToken(f.app, "unknown", exchangedAt); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("unknown: %v", err)
		}
	})
	t.Run("revoked", func(t *testing.T) {
		f := newRefreshFixture(t)
		if err := RevokeToken(f.app, f.client, f.tokens.AccessToken, exchangedAt); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		if _, _, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("revoked: %v", err)
		}
	})
	for name, change := range map[string]func(refreshFixture){
		"user disabled": func(f refreshFixture) { f.user.Set("status", "disabled") },
		"app disabled":  func(f refreshFixture) { f.client.Set("enabled", false) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newRefreshFixture(t)
			change(f)
			if err := f.app.Save(f.user); err != nil {
				t.Fatalf("save user: %v", err)
			}
			if err := f.app.Save(f.client); err != nil {
				t.Fatalf("save client: %v", err)
			}
			if _, _, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("%s: %v", name, err)
			}
		})
	}
}

func TestRevokeToken(t *testing.T) {
	f := newRefreshFixture(t)
	other := exchangedTokens(t, f.app, f.box, f.client, f.user, offlineScope)
	stranger, _ := newTestClient(t, f.app, nil)

	for name, raw := range map[string]string{"unknown token": "unknown", "another app's token": f.tokens.AccessToken} {
		if err := RevokeToken(f.app, stranger, raw, exchangedAt); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if tokenRevoked(t, f.app, f.tokens.AccessToken) {
		t.Fatal("another app revoked the token")
	}

	if err := RevokeToken(f.app, f.client, other.AccessToken, exchangedAt); err != nil {
		t.Fatalf("revoke access token: %v", err)
	}
	if !tokenRevoked(t, f.app, other.AccessToken) || tokenRevoked(t, f.app, other.RefreshToken) {
		t.Fatal("revoking an access token must leave its refresh token")
	}

	if err := RevokeToken(f.app, f.client, f.tokens.RefreshToken, exchangedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke refresh token: %v", err)
	}
	if !tokenRevoked(t, f.app, f.tokens.RefreshToken) || !tokenRevoked(t, f.app, f.tokens.AccessToken) {
		t.Fatal("revoking a refresh token must end its family")
	}
}
