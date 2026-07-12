package migrations

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"hysterical-panel/internal/token"
)

// Move node credentials out of users so the panel can retain every retired
// Auth String for legacy Node Client ID attribution while exposing exactly one
// Current Auth String through the product APIs.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		authStrings := core.NewBaseCollection("user_auth_strings")
		authStrings.Fields.Add(&core.RelationField{
			Name:          "user",
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  users.Id,
		})
		authStrings.Fields.Add(&core.TextField{Name: "auth_string", Required: true, Max: 128, Hidden: true})
		authStrings.Fields.Add(&core.TextField{Name: "auth_string_anytls_hash", Required: true, Max: 64, Hidden: true})
		authStrings.Fields.Add(&core.SelectField{
			Name:      "state",
			Required:  true,
			MaxSelect: 1,
			Values:    []string{"current", "retired"},
		})
		authStrings.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		authStrings.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		authStrings.AddIndex("idx_user_auth_strings_auth", true, "auth_string", "")
		authStrings.AddIndex("idx_user_auth_strings_anytls_hash", true, "auth_string_anytls_hash", "")
		authStrings.AddIndex("idx_user_auth_strings_one_current", true, "user", "state = 'current'")
		authStrings.AddIndex("idx_user_auth_strings_user", false, "user", "")
		if err := app.Save(authStrings); err != nil {
			return err
		}

		records, err := app.FindRecordsByFilter("users", "", "", 0, 0)
		if err != nil {
			return err
		}
		for _, user := range records {
			authString := user.GetString("auth_string")
			if authString == "" {
				return fmt.Errorf("user %s has no auth_string", user.Id)
			}
			collision, lookupErr := app.FindRecordById("users", authString)
			if lookupErr == nil && collision != nil {
				return fmt.Errorf("user %s auth_string conflicts with user id %s", user.Id, authString)
			}
			if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
				return fmt.Errorf("check user %s auth_string against user ids: %w", user.Id, lookupErr)
			}
			hash := token.Sha256Hex(authString)
			record := core.NewRecord(authStrings)
			record.Set("user", user.Id)
			record.Set("auth_string", authString)
			record.Set("auth_string_anytls_hash", hash)
			record.Set("state", "current")
			if err := app.Save(record); err != nil {
				return fmt.Errorf("backfill user %s auth string: %w", user.Id, err)
			}
		}

		users.RemoveIndex("idx_users_auth_string")
		users.RemoveIndex("idx_users_auth_string_anytls_hash")
		users.Fields.RemoveByName("auth_string")
		users.Fields.RemoveByName("auth_string_anytls_hash")
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		authStrings, err := app.FindCollectionByNameOrId("user_auth_strings")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.TextField{Name: "auth_string", Max: 128})
		users.Fields.Add(&core.TextField{Name: "auth_string_anytls_hash", Max: 64})
		if err := app.Save(users); err != nil {
			return err
		}

		currents, err := app.FindRecordsByFilter("user_auth_strings", "state = 'current'", "", 0, 0)
		if err != nil {
			return err
		}
		for _, current := range currents {
			user, err := app.FindRecordById("users", current.GetString("user"))
			if err != nil {
				return err
			}
			user.Set("auth_string", current.GetString("auth_string"))
			user.Set("auth_string_anytls_hash", current.GetString("auth_string_anytls_hash"))
			if err := app.Save(user); err != nil {
				return err
			}
		}

		authField, ok := users.Fields.GetByName("auth_string").(*core.TextField)
		if !ok {
			return errors.New("users.auth_string field not found")
		}
		hashField, ok := users.Fields.GetByName("auth_string_anytls_hash").(*core.TextField)
		if !ok {
			return errors.New("users.auth_string_anytls_hash field not found")
		}
		authField.Required = true
		hashField.Required = true
		users.AddIndex("idx_users_auth_string", true, "auth_string", "")
		users.AddIndex("idx_users_auth_string_anytls_hash", true, "auth_string_anytls_hash", "")
		if err := app.Save(users); err != nil {
			return err
		}
		return app.Delete(authStrings)
	})
}
