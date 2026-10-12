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
