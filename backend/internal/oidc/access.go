package oidc

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// ErrInvalidToken means the access token is unknown, expired, revoked or no
// longer backed by a usable grant.
var ErrInvalidToken = errors.New("invalid access token")

// LookupAccessToken returns the User and scope behind an access token. A
// refresh token is never accepted here.
func LookupAccessToken(app core.App, raw string, now time.Time) (*core.Record, []string, error) {
	if raw == "" {
		return nil, nil, ErrInvalidToken
	}
	rec, err := app.FindFirstRecordByFilter(TokensCollection,
		"token_hash = {:hash} && kind = 'access' && revoked_at = '' && expires_at > {:now}",
		dbx.Params{"hash": token.Sha256Hex(raw), "now": dbTime(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrInvalidToken
	}
	if err != nil {
		return nil, nil, err
	}
	grant, err := app.FindRecordById(GrantsCollection, rec.GetString("grant"))
	if err != nil {
		return nil, nil, err
	}
	client, err := app.FindRecordById(ClientsCollection, grant.GetString("client"))
	if err != nil {
		return nil, nil, err
	}
	user, err := grantUser(app, grant, client)
	if errors.Is(err, errInvalidGrant) {
		return nil, nil, ErrInvalidToken
	}
	if err != nil {
		return nil, nil, err
	}
	return user, strings.Fields(rec.GetString("scope")), nil
}

// RevokeToken implements RFC 7009. Unknown tokens and tokens of other apps
// are ignored, so the answer never confirms that a token exists. Revoking a
// refresh token ends its whole family.
func RevokeToken(app core.App, client *core.Record, raw string, now time.Time) error {
	return app.RunInTransaction(func(tx core.App) error {
		rec, err := tx.FindFirstRecordByFilter(TokensCollection, "token_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(raw)})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		grant, err := tx.FindRecordById(GrantsCollection, rec.GetString("grant"))
		if err != nil {
			return err
		}
		if grant.GetString("client") != client.Id {
			return nil
		}
		if rec.GetString("kind") == tokenKindRefresh {
			return revokeFamily(tx, rec.GetString("family"), now)
		}
		_, err = claimOnce(tx, TokensCollection, rec.Id, "revoked_at", now)
		return err
	})
}
