package oidc

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/token"
)

// Grant types accepted at the token endpoint.
const (
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
)

const (
	tokenKindAccess  = "access"
	tokenKindRefresh = "refresh"
	tokenTypeBearer  = "Bearer"
)

// codeVerifierPattern is the code_verifier syntax of RFC 7636 4.1.
var codeVerifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// TokenRequest is a token endpoint request from an authenticated app.
type TokenRequest struct {
	Client *core.Record
	Form   url.Values
	Issuer string
	Box    *cryptobox.Box
	Now    time.Time
}

// TokenResponse is the response of RFC 6749 5.1 and OIDC Core 3.1.3.3.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

// issuance is what one token response is issued from. scope covers the
// access token and the ID token. refreshScope covers the refresh token,
// which keeps the granted scope when a refresh narrows it.
type issuance struct {
	grant, user  *core.Record
	family       string
	scope        []string
	refreshScope []string
	authTime     time.Time
	nonce        string
	refreshUntil time.Time
}

// ExchangeCode redeems an authorization code (RFC 6749 4.1.3, RFC 7636 4.6).
func ExchangeCode(app core.App, req TokenRequest) (*TokenResponse, *Error) {
	raw := req.Form.Get("code")
	if raw == "" || req.Form.Get("redirect_uri") == "" || req.Form.Get("code_verifier") == "" {
		return nil, NewError(InvalidRequest, "code, redirect_uri and code_verifier are required")
	}
	return runGrant(app, func(tx core.App) (*TokenResponse, error) {
		code, grant, err := findIssued(tx, CodesCollection, "code_hash = {:hash}", raw, req.Client)
		if err != nil {
			return nil, err
		}
		if !code.GetDateTime("used_at").IsZero() {
			return nil, replayed(tx, code, req.Now)
		}
		if !req.Now.Before(code.GetDateTime("expires_at").Time()) ||
			code.GetString("redirect_uri") != req.Form.Get("redirect_uri") ||
			!verifyPKCE(req.Form.Get("code_verifier"), code.GetString("code_challenge")) {
			return nil, errInvalidGrant
		}
		user, err := grantUser(tx, grant, req.Client)
		if err != nil {
			return nil, err
		}
		if err := claimGrant(tx, CodesCollection, code, "used_at", req.Now); err != nil {
			return nil, err
		}
		scope := strings.Fields(code.GetString("scope"))
		return issueTokens(tx, req, issuance{
			grant:        grant,
			user:         user,
			family:       code.GetString("family"),
			scope:        scope,
			refreshScope: scope,
			authTime:     code.GetDateTime("auth_time").Time(),
			nonce:        code.GetString("nonce"),
		})
	})
}

// runGrant runs a grant in one transaction. An OAuth error commits, so the
// family revocation of a replay sticks although the request fails. Any other
// error rolls back and becomes server_error.
func runGrant(app core.App, grant func(tx core.App) (*TokenResponse, error)) (*TokenResponse, *Error) {
	var resp *TokenResponse
	var oauthErr *Error
	err := app.RunInTransaction(func(tx core.App) error {
		var err error
		resp, err = grant(tx)
		if errors.As(err, &oauthErr) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, WrapServerError(err)
	}
	if oauthErr != nil {
		return nil, oauthErr
	}
	return resp, nil
}

// findIssued loads a code or refresh token and its grant. A value issued to
// another app fails like an unknown one.
func findIssued(tx core.App, collection, filter, raw string, client *core.Record) (rec, grant *core.Record, err error) {
	rec, err = tx.FindFirstRecordByFilter(collection, filter, dbx.Params{"hash": token.Sha256Hex(raw)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errInvalidGrant
	}
	if err != nil {
		return nil, nil, err
	}
	grant, err = tx.FindRecordById(GrantsCollection, rec.GetString("grant"))
	if err != nil {
		return nil, nil, err
	}
	if grant.GetString("client") != client.Id {
		return nil, nil, errInvalidGrant
	}
	return rec, grant, nil
}

// claimGrant marks a code or refresh token used. Losing the claim means
// another request used it first, which is a replay.
func claimGrant(tx core.App, collection string, rec *core.Record, field string, now time.Time) error {
	claimed, err := claimOnce(tx, collection, rec.Id, field, now)
	if err != nil {
		return err
	}
	if !claimed {
		return replayed(tx, rec, now)
	}
	return nil
}

// replayed revokes every token of the family and fails the request
// (RFC 6749 4.1.2, RFC 9700 4.14.2).
func replayed(tx core.App, rec *core.Record, now time.Time) error {
	if err := revokeFamily(tx, rec.GetString("family"), now); err != nil {
		return err
	}
	return errInvalidGrant
}

// verifyPKCE checks an S256 code_verifier against the stored challenge.
func verifyPKCE(verifier, challenge string) bool {
	if !codeVerifierPattern.MatchString(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// issueTokens issues an access token, an ID token and, when the app allows
// refresh and the grant includes offline_access, a refresh token. Every grant
// includes openid, so every response carries an ID token.
func issueTokens(tx core.App, req TokenRequest, in issuance) (*TokenResponse, error) {
	accessToken, err := saveToken(tx, tokenKindAccess, in, in.scope, req.Now.Add(AccessTokenTTL), time.Time{})
	if err != nil {
		return nil, err
	}
	resp := &TokenResponse{
		AccessToken: accessToken,
		TokenType:   tokenTypeBearer,
		ExpiresIn:   int(AccessTokenTTL.Seconds()),
		Scope:       strings.Join(in.scope, " "),
	}
	if req.Client.GetBool("refresh_enabled") && slices.Contains(in.refreshScope, ScopeOfflineAccess) {
		if resp.RefreshToken, err = saveRefreshToken(tx, in, req.Now); err != nil {
			return nil, err
		}
	}
	if resp.IDToken, err = signIDToken(tx, req, in, accessToken); err != nil {
		return nil, err
	}
	return resp, touchLastUsed(tx, req.Now, in.grant, req.Client)
}

// saveRefreshToken slides the idle lifetime but never past the absolute cap
// set when the family started.
func saveRefreshToken(tx core.App, in issuance, now time.Time) (string, error) {
	until := in.refreshUntil
	if until.IsZero() {
		until = now.Add(RefreshTokenMaxTTL)
	}
	expires := now.Add(RefreshTokenIdleTTL)
	if until.Before(expires) {
		expires = until
	}
	return saveToken(tx, tokenKindRefresh, in, in.refreshScope, expires, until)
}

func saveToken(tx core.App, kind string, in issuance, scope []string, expiresAt, absoluteExpiresAt time.Time) (string, error) {
	raw, err := token.New(32)
	if err != nil {
		return "", err
	}
	collection, err := tx.FindCollectionByNameOrId(TokensCollection)
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(collection)
	rec.Set("token_hash", token.Sha256Hex(raw))
	rec.Set("kind", kind)
	rec.Set("grant", in.grant.Id)
	rec.Set("family", in.family)
	rec.Set("scope", strings.Join(scope, " "))
	rec.Set("expires_at", expiresAt.UTC())
	if !in.authTime.IsZero() {
		rec.Set("auth_time", in.authTime.UTC())
	}
	if !absoluteExpiresAt.IsZero() {
		rec.Set("absolute_expires_at", absoluteExpiresAt.UTC())
	}
	return raw, tx.Save(rec)
}

// touchLastUsed records when the grant and the app last received tokens.
// A plain UPDATE leaves the records' updated time alone.
func touchLastUsed(tx core.App, now time.Time, records ...*core.Record) error {
	for _, rec := range records {
		_, err := tx.NonconcurrentDB().Update(rec.Collection().Name,
			dbx.Params{"last_used_at": dbTime(now)}, dbx.HashExp{"id": rec.Id}).Execute()
		if err != nil {
			return err
		}
	}
	return nil
}

// RefreshTokens rotates a refresh token (RFC 6749 6, RFC 9700 4.14.2). The
// presented token stops working, and presenting it again revokes the family.
func RefreshTokens(app core.App, req TokenRequest) (*TokenResponse, *Error) {
	raw := req.Form.Get("refresh_token")
	if raw == "" {
		return nil, NewError(InvalidRequest, "refresh_token is required")
	}
	return runGrant(app, func(tx core.App) (*TokenResponse, error) {
		rec, grant, err := findIssued(tx, TokensCollection, "token_hash = {:hash} && kind = 'refresh'", raw, req.Client)
		if err != nil {
			return nil, err
		}
		if !rec.GetDateTime("rotated_at").IsZero() {
			return nil, replayed(tx, rec, req.Now)
		}
		if !rec.GetDateTime("revoked_at").IsZero() || !req.Now.Before(rec.GetDateTime("expires_at").Time()) ||
			!req.Client.GetBool("refresh_enabled") {
			return nil, errInvalidGrant
		}
		user, err := grantUser(tx, grant, req.Client)
		if err != nil {
			return nil, err
		}
		granted := grantableScopes(req.Client, strings.Fields(rec.GetString("scope")))
		scope, err := narrowScope(granted, req.Form.Get("scope"))
		if err != nil {
			return nil, err
		}
		if err := claimGrant(tx, TokensCollection, rec, "rotated_at", req.Now); err != nil {
			return nil, err
		}
		return issueTokens(tx, req, issuance{
			grant:        grant,
			user:         user,
			family:       rec.GetString("family"),
			scope:        scope,
			refreshScope: granted,
			authTime:     rec.GetDateTime("auth_time").Time(),
			refreshUntil: rec.GetDateTime("absolute_expires_at").Time(),
		})
	})
}

// narrowScope applies the optional scope of a refresh request. It may only
// drop scopes, and openid stays because the response carries an ID token.
func narrowScope(granted []string, requested string) ([]string, error) {
	if requested == "" {
		return granted, nil
	}
	scope := strings.Fields(requested)
	if !slices.Contains(scope, ScopeOpenID) || !isSubset(scope, granted) {
		return nil, NewError(InvalidScope, "a refresh may only narrow the granted scope and must keep openid")
	}
	return orderScopes(scope), nil
}
