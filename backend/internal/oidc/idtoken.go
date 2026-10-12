package oidc

import (
	"crypto/sha256"
	"encoding/base64"
	"slices"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/pocketbase/core"
)

// UserClaims are the claims about a User that scope releases. The ID token
// and userinfo share them, and they follow what help-center-next reads.
func UserClaims(user *core.Record, scope []string) map[string]any {
	claims := map[string]any{"sub": user.Id}
	if slices.Contains(scope, ScopeEmail) {
		claims["email"] = user.Email()
		claims["email_verified"] = user.Verified()
	}
	return claims
}

// signIDToken signs the ID token of a token response (OIDC Core 3.1.3.6). A
// refresh passes the original auth_time and no nonce (OIDC Core 12.2).
func signIDToken(tx core.App, req TokenRequest, in issuance, accessToken string) (string, error) {
	key, err := ActiveSigningKey(tx, req.Box, req.Now)
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims(UserClaims(in.user, in.scope))
	claims["iss"] = req.Issuer
	claims["aud"] = req.Client.GetString("client_id")
	claims["iat"] = req.Now.Unix()
	claims["exp"] = req.Now.Add(IDTokenTTL).Unix()
	claims["at_hash"] = atHash(accessToken)
	if !in.authTime.IsZero() {
		claims["auth_time"] = in.authTime.Unix()
	}
	if in.nonce != "" {
		claims["nonce"] = in.nonce
	}
	idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	idToken.Header["kid"] = key.ID
	return idToken.SignedString(key.Private)
}

// atHash is the left half of the access token's SHA-256 (OIDC Core 3.1.3.6).
func atHash(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return base64.RawURLEncoding.EncodeToString(sum[:len(sum)/2])
}
