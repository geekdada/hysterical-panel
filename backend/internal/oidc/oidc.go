// Package oidc holds the storage, signing keys and session bookkeeping of the
// panel's OpenID Connect provider (ADR-0010). HTTP handlers live in
// internal/api.
package oidc

import "time"

const (
	ClientsCollection          = "oidc_clients"
	AuthRequestsCollection     = "oidc_auth_requests"
	CodesCollection            = "oidc_codes"
	GrantsCollection           = "oidc_grants"
	TokensCollection           = "oidc_tokens"
	SigningKeysCollection      = "oidc_signing_keys"
	SessionAuthTimesCollection = "oidc_session_auth_times"
)

// Lifetimes are fixed by ADR-0010.
const (
	AuthRequestTTL      = 10 * time.Minute
	CodeTTL             = 60 * time.Second
	AccessTokenTTL      = 10 * time.Minute
	IDTokenTTL          = 10 * time.Minute
	RefreshTokenIdleTTL = 30 * 24 * time.Hour
	RefreshTokenMaxTTL  = 90 * 24 * time.Hour

	// RetiredKeyRetention keeps a rotated-out key in JWKS long enough for
	// relying parties to verify ID tokens it signed before the rotation.
	RetiredKeyRetention = 24 * time.Hour

	// expiredTokenRetention keeps expired codes and tokens a while longer so
	// a late replay is still recognised and revokes its family.
	expiredTokenRetention = 7 * 24 * time.Hour
)

// Prompt values (OIDC Core 3.1.2.1) in the order they are stored.
const (
	PromptNone          = "none"
	PromptLogin         = "login"
	PromptConsent       = "consent"
	PromptSelectAccount = "select_account"
)

// PromptValues lists every supported prompt value in canonical order.
var PromptValues = []string{PromptNone, PromptLogin, PromptConsent, PromptSelectAccount}
