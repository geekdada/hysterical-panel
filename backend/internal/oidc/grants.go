package oidc

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// UserMayUse is the one place that decides who may sign in to a Connected
// App (ADR-0010). Every active, verified User may use every enabled app.
// Per-app restrictions belong here.
func UserMayUse(client, user *core.Record) bool {
	return client != nil && user != nil && client.GetBool("enabled") &&
		user.GetString("status") == "active" && user.GetBool("verified")
}

func findActiveGrant(app core.App, userID, clientID string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(GrantsCollection,
		"user = {:user} && client = {:client} && revoked_at = ''",
		dbx.Params{"user": userID, "client": clientID})
}

// upsertGrant records consent. A User has at most one active grant per app,
// and approving more scopes widens it.
func upsertGrant(tx core.App, userID, clientID string, scopes []string) (*core.Record, error) {
	grant, err := findActiveGrant(tx, userID, clientID)
	if errors.Is(err, sql.ErrNoRows) {
		collection, findErr := tx.FindCollectionByNameOrId(GrantsCollection)
		if findErr != nil {
			return nil, findErr
		}
		grant = core.NewRecord(collection)
		grant.Set("user", userID)
		grant.Set("client", clientID)
	} else if err != nil {
		return nil, err
	}
	grant.Set("scopes", unionScopes(grantScopes(grant), scopes))
	return grant, tx.Save(grant)
}

func grantScopes(grant *core.Record) []string {
	return stringList(grant, "scopes")
}

func dbTime(t time.Time) string {
	return t.UTC().Format(types.DefaultDateLayout)
}

// claimOnce stamps an empty date field and reports whether this call did it.
// The conditional UPDATE keeps requests, codes and refresh tokens single-use
// even when two requests race.
func claimOnce(app core.App, collection, id, field string, now time.Time) (bool, error) {
	result, err := app.NonconcurrentDB().Update(collection,
		dbx.Params{field: dbTime(now)},
		dbx.NewExp("[[id]] = {:id} AND [["+field+"]] = ''", dbx.Params{"id": id}),
	).Execute()
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// errInvalidGrant answers every unusable code or refresh token alike, so a
// client cannot tell an unknown value from an expired or foreign one.
var errInvalidGrant = NewError(InvalidGrant, "the code or refresh token is invalid or expired")

// grantUser returns the User behind an unrevoked grant who may still use the
// app, or errInvalidGrant.
func grantUser(app core.App, grant, client *core.Record) (*core.Record, error) {
	if !grant.GetDateTime("revoked_at").IsZero() {
		return nil, errInvalidGrant
	}
	user, err := app.FindRecordById("users", grant.GetString("user"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	if !UserMayUse(client, user) {
		return nil, errInvalidGrant
	}
	return user, nil
}

// revokeFamily revokes every token issued from one code.
func revokeFamily(app core.App, family string, now time.Time) error {
	_, err := app.NonconcurrentDB().Update(TokensCollection,
		dbx.Params{"revoked_at": dbTime(now)},
		dbx.NewExp("[[family]] = {:family} AND [[revoked_at]] = ''", dbx.Params{"family": family}),
	).Execute()
	return err
}

// Reasons stored in oidc_grants.revoke_reason.
const (
	RevokeReasonUserDisabled   = "user_disabled"
	RevokeReasonUserUnverified = "user_unverified"
	RevokeReasonUserRevoked    = "user_revoked"
)

// RevokeUserGrants ends every remembered consent of a User. Their tokens
// stop working because every lookup checks the grant.
func RevokeUserGrants(app core.App, userID, reason string, now time.Time) error {
	_, err := app.NonconcurrentDB().Update(GrantsCollection,
		dbx.Params{"revoked_at": dbTime(now), "revoke_reason": reason},
		dbx.NewExp("[[user]] = {:user} AND [[revoked_at]] = ''", dbx.Params{"user": userID}),
	).Execute()
	return err
}

// ErrGrantNotFound reports a grant that does not exist, is already revoked
// or belongs to another User.
var ErrGrantNotFound = errors.New("connected app grant not found")

// ActiveGrant is a remembered consent as the panel lists it. LastUsedAt is
// zero until the app first receives tokens.
type ActiveGrant struct {
	ID         string
	ClientName string
	Scopes     []string
	Created    time.Time
	LastUsedAt time.Time
}

// ListActiveGrants returns a User's unrevoked grants, newest first.
func ListActiveGrants(app core.App, userID string) ([]ActiveGrant, error) {
	grants, err := app.FindRecordsByFilter(GrantsCollection, "user = {:user} && revoked_at = ''", "-created", 0, 0,
		dbx.Params{"user": userID})
	if err != nil {
		return nil, err
	}
	out := make([]ActiveGrant, 0, len(grants))
	for _, grant := range grants {
		client, err := app.FindRecordById(ClientsCollection, grant.GetString("client"))
		if err != nil {
			return nil, err
		}
		out = append(out, ActiveGrant{
			ID:         grant.Id,
			ClientName: client.GetString("name"),
			Scopes:     grantScopes(grant),
			Created:    grant.GetDateTime("created").Time(),
			LastUsedAt: grant.GetDateTime("last_used_at").Time(),
		})
	}
	return out, nil
}

// RevokeGrant ends one of a User's grants. Every token issued under it stops
// working at once, because each token lookup checks the grant.
func RevokeGrant(app core.App, userID, grantID string, now time.Time) error {
	result, err := app.NonconcurrentDB().Update(GrantsCollection,
		dbx.Params{"revoked_at": dbTime(now), "revoke_reason": RevokeReasonUserRevoked},
		dbx.NewExp("[[id]] = {:id} AND [[user]] = {:user} AND [[revoked_at]] = ''",
			dbx.Params{"id": grantID, "user": userID}),
	).Execute()
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrGrantNotFound
	}
	return nil
}
