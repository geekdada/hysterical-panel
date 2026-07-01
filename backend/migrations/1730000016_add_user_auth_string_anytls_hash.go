package migrations

import (
	"errors"

	"hysterical-panel/internal/token"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add users.auth_string_anytls_hash = hex(sha256(auth_string)) so anytls clients
// (which authenticate with hex(sha256(password))) can be matched to users. The
// value is kept in sync on every save by the OnRecordCreate/OnRecordUpdate hooks
// bound in internal/api; those hooks are inactive during migration, so we backfill
// existing rows explicitly here.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// Drop legacy column from a failed partial apply of the old field name.
		if users.Fields.GetByName("auth_string_hash") != nil {
			users.RemoveIndex("idx_users_auth_string_hash")
			users.Fields.RemoveByName("auth_string_hash")
			if err := app.Save(users); err != nil {
				return err
			}
		}

		// Add the column without a unique index first. Creating the index in the
		// same Save as the new field would leave every existing row at the empty
		// default and fail the UNIQUE constraint.
		if users.Fields.GetByName("auth_string_anytls_hash") == nil {
			users.Fields.Add(&core.TextField{
				Name:     "auth_string_anytls_hash",
				Required: false,
				Max:      64,
			})
			if err := app.Save(users); err != nil {
				return err
			}
		}

		records, err := app.FindRecordsByFilter("users", "", "", 0, 0)
		if err != nil {
			return err
		}
		for _, rec := range records {
			if rec.GetString("auth_string_anytls_hash") != "" {
				continue
			}
			rec.Set("auth_string_anytls_hash", token.Sha256Hex(rec.GetString("auth_string")))
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		field, ok := users.Fields.GetByName("auth_string_anytls_hash").(*core.TextField)
		if !ok {
			return errors.New("users.auth_string_anytls_hash field not found")
		}
		field.Required = true
		if users.GetIndex("idx_users_auth_string_anytls_hash") == "" {
			users.AddIndex("idx_users_auth_string_anytls_hash", true, "auth_string_anytls_hash", "")
		}
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.RemoveIndex("idx_users_auth_string_anytls_hash")
		users.Fields.RemoveByName("auth_string_anytls_hash")
		return app.Save(users)
	})
}
