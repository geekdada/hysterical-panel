// Package authstrings owns node credentials and the mapping from Node Client
// IDs back to panel Users.
package authstrings

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

const (
	Collection = "user_auth_strings"
	Current    = "current"
	Retired    = "retired"
)

// CreateCurrent adds a never-before-used Current Auth String for userID.
// Callers that also mutate a User should pass a transaction-scoped app.
func CreateCurrent(app core.App, userID, authString string) (*core.Record, error) {
	user, err := app.FindRecordById("users", authString)
	if err == nil && user != nil {
		return nil, fmt.Errorf("auth string conflicts with user id %s", authString)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check auth string against user ids: %w", err)
	}
	credential, err := app.FindFirstRecordByFilter(
		Collection,
		"auth_string = {:userID}",
		map[string]any{"userID": userID},
	)
	if err == nil && credential != nil {
		return nil, fmt.Errorf("user id %s conflicts with a historical auth string", userID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check user id against auth string history: %w", err)
	}
	collection, err := app.FindCollectionByNameOrId(Collection)
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("user", userID)
	record.Set("auth_string", authString)
	record.Set("auth_string_anytls_hash", token.Sha256Hex(authString))
	record.Set("state", Current)
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return record, nil
}

// Rotate retires the current credential and creates a new Current Auth String.
// Uniqueness across the whole history prevents retired credentials from being
// restored or transferred to another User.
func Rotate(app core.App, userID, authString string) (*core.Record, error) {
	current, err := CurrentForUser(app, userID)
	if err != nil {
		return nil, err
	}
	current.Set("state", Retired)
	if err := app.Save(current); err != nil {
		return nil, err
	}
	return CreateCurrent(app, userID, authString)
}

func CurrentForUser(app core.App, userID string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(
		Collection,
		"user = {:user} && state = 'current'",
		map[string]any{"user": userID},
	)
}

func CurrentValue(app core.App, userID string) (string, error) {
	record, err := CurrentForUser(app, userID)
	if err != nil {
		return "", err
	}
	return record.GetString("auth_string"), nil
}

func FindCurrentUserByAuthString(app core.App, authString string) (*core.Record, error) {
	credential, err := app.FindFirstRecordByFilter(
		Collection,
		"auth_string = {:auth} && state = 'current'",
		map[string]any{"auth": authString},
	)
	if err != nil {
		return nil, err
	}
	return app.FindRecordById("users", credential.GetString("user"))
}

func FindCurrentUserByAnytlsHash(app core.App, hash string) (*core.Record, error) {
	credential, err := app.FindFirstRecordByFilter(
		Collection,
		"auth_string_anytls_hash = {:hash} && state = 'current'",
		map[string]any{"hash": hash},
	)
	if err != nil {
		return nil, err
	}
	return app.FindRecordById("users", credential.GetString("user"))
}

// Exists reports whether an Auth String has ever been used, including Retired
// records that must never become valid credentials again.
func Exists(app core.App, authString string) bool {
	record, _ := app.FindFirstRecordByFilter(
		Collection,
		"auth_string = {:auth}",
		map[string]any{"auth": authString},
	)
	return record != nil
}

// Resolver is a request/poll snapshot that canonicalizes both the stable User
// ID and every historical Auth String to the same User.
type Resolver struct {
	byClientID  map[string]*core.Record
	currentAuth map[string]string
}

func LoadResolver(app core.App) (*Resolver, error) {
	users, err := app.FindRecordsByFilter("users", "", "", 0, 0)
	if err != nil {
		return nil, err
	}
	credentials, err := app.FindRecordsByFilter(Collection, "", "", 0, 0)
	if err != nil {
		return nil, err
	}

	r := &Resolver{
		byClientID:  make(map[string]*core.Record, len(users)+len(credentials)),
		currentAuth: make(map[string]string, len(users)),
	}
	usersByID := make(map[string]*core.Record, len(users))
	for _, user := range users {
		usersByID[user.Id] = user
	}
	for _, credential := range credentials {
		user := usersByID[credential.GetString("user")]
		if user == nil {
			continue
		}
		authString := credential.GetString("auth_string")
		if usersByID[authString] != nil {
			return nil, fmt.Errorf("auth string %s conflicts with a user id", authString)
		}
		r.byClientID[authString] = user
		if credential.GetString("state") == Current {
			r.currentAuth[user.Id] = credential.GetString("auth_string")
		}
	}
	for id, user := range usersByID {
		r.byClientID[id] = user
	}
	return r, nil
}

func (r *Resolver) Resolve(clientID string) *core.Record {
	if r == nil {
		return nil
	}
	return r.byClientID[clientID]
}

func (r *Resolver) CurrentForUser(userID string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("auth string resolver is nil")
	}
	authString := r.currentAuth[userID]
	if authString == "" {
		return "", fmt.Errorf("user %s has no current auth string", userID)
	}
	return authString, nil
}
