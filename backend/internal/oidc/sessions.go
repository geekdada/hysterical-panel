package oidc

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/token"
)

// RecordSessionAuth remembers when the panel session behind sessionToken came
// from a real sign-in. PocketBase reports a token refresh with an empty
// authMethod, so a refresh copies the auth time of previousToken instead.
func RecordSessionAuth(app core.App, user *core.Record, sessionToken, authMethod, previousToken string, now time.Time) error {
	// Identical claims within one second produce an identical JWT, and
	// non-refreshable tokens come back unchanged from a refresh.
	if sessionToken == previousToken || sessionRecorded(app, sessionToken) {
		return nil
	}
	authTime := now.UTC()
	if authMethod == "" {
		known, ok, err := SessionAuthTime(app, previousToken, now)
		if err != nil || !ok {
			return err
		}
		authTime = known
	}
	col, err := app.FindCollectionByNameOrId(SessionAuthTimesCollection)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("token_hash", token.Sha256Hex(sessionToken))
	rec.Set("user", user.Id)
	rec.Set("auth_time", authTime)
	rec.Set("expires_at", now.UTC().Add(user.Collection().AuthToken.DurationTime()))
	return app.Save(rec)
}

// SessionAuthTime returns the recorded sign-in time of an unexpired panel
// session token. ok is false for tokens issued before recording started.
func SessionAuthTime(app core.App, sessionToken string, now time.Time) (time.Time, bool, error) {
	if sessionToken == "" {
		return time.Time{}, false, nil
	}
	rec, err := app.FindFirstRecordByFilter(SessionAuthTimesCollection,
		"token_hash = {:hash} && expires_at > {:now}",
		dbx.Params{"hash": token.Sha256Hex(sessionToken), "now": now.UTC().Format(types.DefaultDateLayout)})
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return rec.GetDateTime("auth_time").Time(), true, nil
}

func sessionRecorded(app core.App, sessionToken string) bool {
	_, err := app.FindFirstRecordByFilter(SessionAuthTimesCollection, "token_hash = {:hash}",
		dbx.Params{"hash": token.Sha256Hex(sessionToken)})
	return err == nil
}
