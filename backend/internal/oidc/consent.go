package oidc

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

var (
	ErrRequestNotFound          = errors.New("authorization request not found or expired")
	ErrReauthenticationRequired = errors.New("reauthentication required")
	ErrUserNotAllowed           = errors.New("account cannot sign in to connected apps")
)

// Session is what the panel knows about the session deciding a request.
type Session struct {
	AuthTime time.Time
	Known    bool
}

// PendingRequest is an authorization request that can still be decided.
type PendingRequest struct {
	record *core.Record
	Client *core.Record
}

// FindPendingRequest loads an unconsumed, unexpired request whose app is
// still enabled and still lists its redirect_uri.
func FindPendingRequest(app core.App, handle string, now time.Time) (*PendingRequest, error) {
	if handle == "" {
		return nil, ErrRequestNotFound
	}
	rec, err := app.FindFirstRecordByFilter(AuthRequestsCollection,
		"request_hash = {:hash} && consumed_at = '' && expires_at > {:now}",
		dbx.Params{"hash": token.Sha256Hex(handle), "now": dbTime(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	client, err := app.FindRecordById(ClientsCollection, rec.GetString("client"))
	if err != nil || !client.GetBool("enabled") || !slices.Contains(ClientRedirectURIs(client), rec.GetString("redirect_uri")) {
		return nil, ErrRequestNotFound
	}
	return &PendingRequest{record: rec, Client: client}, nil
}

func (p *PendingRequest) Scopes() []string     { return strings.Fields(p.record.GetString("scope")) }
func (p *PendingRequest) RedirectURI() string  { return p.record.GetString("redirect_uri") }
func (p *PendingRequest) ExpiresAt() time.Time { return p.record.GetDateTime("expires_at").Time() }
func (p *PendingRequest) PromptNone() bool     { return p.hasPrompt(PromptNone) }

func (p *PendingRequest) hasPrompt(value string) bool {
	return slices.Contains(strings.Fields(p.record.GetString("prompt")), value)
}

// NeedsReauthentication reports whether the session is too old for the
// request. prompt=login needs a sign-in after the request was made.
func (p *PendingRequest) NeedsReauthentication(s Session, now time.Time) bool {
	requestedAt := p.ExpiresAt().Add(-AuthRequestTTL)
	if p.hasPrompt(PromptLogin) && (!s.Known || s.AuthTime.Before(requestedAt)) {
		return true
	}
	maxAge := p.record.GetInt("max_age")
	return maxAge > 0 && (!s.Known || now.Sub(s.AuthTime) > time.Duration(maxAge)*time.Second)
}

// NeedsConsent reports whether the User must see the consent page. Consent
// is remembered per User, app and scope set (ADR-0010).
func (p *PendingRequest) NeedsConsent(app core.App, userID string) (bool, error) {
	if p.hasPrompt(PromptConsent) || p.hasPrompt(PromptSelectAccount) {
		return true, nil
	}
	if p.Client.GetBool("skip_consent") {
		return false, nil
	}
	grant, err := findActiveGrant(app, userID, p.Client.Id)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !isSubset(p.Scopes(), grantScopes(grant)), nil
}

// Approve turns a pending request into an authorization code and returns the
// redirect back to the app. Approving is the consent, so callers show the
// consent page first unless NeedsConsent says it is not needed.
func Approve(app core.App, handle string, user *core.Record, s Session, issuer string, now time.Time) (string, error) {
	var redirect string
	err := app.RunInTransaction(func(tx core.App) error {
		p, err := FindPendingRequest(tx, handle, now)
		if err != nil {
			return err
		}
		// The caller's record was loaded before the transaction. A disable
		// committed since then has already revoked the grants, and approving
		// would bring the remembered consent back.
		current, err := tx.FindRecordById("users", user.Id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotAllowed
		}
		if err != nil {
			return err
		}
		if !UserMayUse(p.Client, current) {
			return ErrUserNotAllowed
		}
		silent, err := p.silentFailure(tx, user.Id, s, now)
		if err != nil {
			return err
		}
		if silent != nil {
			redirect, err = p.fail(tx, silent, issuer, now)
		} else {
			redirect, err = p.approve(tx, user.Id, s, issuer, now)
		}
		return err
	})
	return redirect, err
}

// silentFailure returns the error a prompt=none request must end with. A
// request that may show pages fails with ErrReauthenticationRequired instead
// and stays pending, so the User can sign in again and come back.
func (p *PendingRequest) silentFailure(tx core.App, userID string, s Session, now time.Time) (*Error, error) {
	if p.NeedsReauthentication(s, now) {
		if p.PromptNone() {
			return NewError(LoginRequired, "the user must sign in again"), nil
		}
		return nil, ErrReauthenticationRequired
	}
	if !p.PromptNone() {
		return nil, nil
	}
	needsConsent, err := p.NeedsConsent(tx, userID)
	if err != nil || !needsConsent {
		return nil, err
	}
	return NewError(ConsentRequired, "the user has not approved this app"), nil
}

// Deny ends a request with access_denied.
func Deny(app core.App, handle, issuer string, now time.Time) (string, error) {
	return finishWithError(app, handle, func(*PendingRequest) *Error {
		return NewError(AccessDenied, "the user denied the request")
	}, issuer, now)
}

// Abort ends a request for a User who is not signed in to the panel.
// prompt=none gets login_required (OIDC Core 3.1.2.6). A User leaving the
// sign-in page gets access_denied.
func Abort(app core.App, handle, issuer string, now time.Time) (string, error) {
	return finishWithError(app, handle, func(p *PendingRequest) *Error {
		if p.PromptNone() {
			return NewError(LoginRequired, "the user is not signed in")
		}
		return NewError(AccessDenied, "the user cancelled the sign-in")
	}, issuer, now)
}

func finishWithError(app core.App, handle string, errorFor func(*PendingRequest) *Error, issuer string, now time.Time) (string, error) {
	var redirect string
	err := app.RunInTransaction(func(tx core.App) error {
		p, err := FindPendingRequest(tx, handle, now)
		if err != nil {
			return err
		}
		redirect, err = p.fail(tx, errorFor(p), issuer, now)
		return err
	})
	return redirect, err
}

func (p *PendingRequest) consume(tx core.App, now time.Time) error {
	claimed, err := claimOnce(tx, AuthRequestsCollection, p.record.Id, "consumed_at", now)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrRequestNotFound
	}
	return nil
}

func (p *PendingRequest) fail(tx core.App, oauthErr *Error, issuer string, now time.Time) (string, error) {
	if err := p.consume(tx, now); err != nil {
		return "", err
	}
	return ErrorRedirect(p.RedirectURI(), oauthErr, p.record.GetString("state"), issuer), nil
}

func (p *PendingRequest) approve(tx core.App, userID string, s Session, issuer string, now time.Time) (string, error) {
	if err := p.consume(tx, now); err != nil {
		return "", err
	}
	grant, err := upsertGrant(tx, userID, p.Client.Id, p.Scopes())
	if err != nil {
		return "", err
	}
	code, err := p.saveCode(tx, grant, s, now)
	if err != nil {
		return "", err
	}
	return codeRedirect(p.RedirectURI(), code, p.record.GetString("state"), issuer), nil
}

// saveCode issues a single-use code that starts a new token family.
func (p *PendingRequest) saveCode(tx core.App, grant *core.Record, s Session, now time.Time) (string, error) {
	code, err := token.New(32)
	if err != nil {
		return "", err
	}
	family, err := token.New(16)
	if err != nil {
		return "", err
	}
	collection, err := tx.FindCollectionByNameOrId(CodesCollection)
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(collection)
	rec.Set("code_hash", token.Sha256Hex(code))
	rec.Set("grant", grant.Id)
	rec.Set("family", family)
	rec.Set("redirect_uri", p.RedirectURI())
	rec.Set("scope", p.record.GetString("scope"))
	rec.Set("nonce", p.record.GetString("nonce"))
	rec.Set("code_challenge", p.record.GetString("code_challenge"))
	rec.Set("expires_at", now.UTC().Add(CodeTTL))
	if s.Known {
		rec.Set("auth_time", s.AuthTime.UTC())
	}
	return code, tx.Save(rec)
}
