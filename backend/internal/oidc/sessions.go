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
	// Non-refreshable tokens come back unchanged from a refresh.
	if sessionToken == previousToken {
		return nil
	}
	authTime, err := candidateAuthTime(app, authMethod, previousToken, now)
	if err != nil {
		return err
	}
	row, err := findSessionRow(app, sessionToken)
	if err != nil {
		return err
	}
	if row == nil {
		insertErr := insertSessionRow(app, user, sessionToken, authTime, now)
		if insertErr == nil {
			return nil
		}
		// A concurrent request may have recorded the same token first.
		if row, err = findSessionRow(app, sessionToken); err != nil || row == nil {
			return insertErr
		}
	}
	return mergeSessionAuthTime(app, row, authTime)
}

// SessionAuthTime returns the recorded sign-in time of an unexpired panel
// session token. ok is false for tokens issued before recording started and
// for tokens shared with a session of unknown age.
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
	authTime := rec.GetDateTime("auth_time").Time()
	return authTime, !authTime.IsZero(), nil
}

// candidateAuthTime returns zero when a refresh continues a session of unknown age.
func candidateAuthTime(app core.App, authMethod, previousToken string, now time.Time) (time.Time, error) {
	if authMethod != "" {
		return now.UTC(), nil
	}
	known, ok, err := SessionAuthTime(app, previousToken, now)
	if err != nil || !ok {
		return time.Time{}, err
	}
	return known, nil
}

func findSessionRow(app core.App, sessionToken string) (*core.Record, error) {
	row, err := app.FindFirstRecordByFilter(SessionAuthTimesCollection, "token_hash = {:hash}",
		dbx.Params{"hash": token.Sha256Hex(sessionToken)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// A zero authTime records the session as unknown, which forces re-authentication.
func insertSessionRow(app core.App, user *core.Record, sessionToken string, authTime, now time.Time) error {
	col, err := app.FindCollectionByNameOrId(SessionAuthTimesCollection)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("token_hash", token.Sha256Hex(sessionToken))
	rec.Set("user", user.Id)
	if !authTime.IsZero() {
		rec.Set("auth_time", authTime)
	}
	rec.Set("expires_at", now.UTC().Add(user.Collection().AuthToken.DurationTime()))
	return app.Save(rec)
}

// Tokens minted for one user within the same second are byte-identical, so
// one row can stand for several sessions. Keeping the oldest auth time, or
// none once any of them is unknown, stops a stale session from borrowing a
// fresh sign-in.
func mergeSessionAuthTime(app core.App, row *core.Record, authTime time.Time) error {
	current := row.GetDateTime("auth_time").Time()
	if current.IsZero() || (!authTime.IsZero() && !authTime.Before(current)) {
		return nil
	}
	if authTime.IsZero() {
		row.Set("auth_time", "")
	} else {
		row.Set("auth_time", authTime)
	}
	return app.Save(row)
}
