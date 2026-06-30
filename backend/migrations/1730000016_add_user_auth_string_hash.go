package migrations

import (
	"hysterical-panel/internal/token"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add users.auth_string_hash = hex(sha256(auth_string)) so anytls clients (which
// authenticate with hex(sha256(password))) can be matched to users. The value is
// kept in sync on every save by the OnRecordCreate/OnRecordUpdate hooks bound in
// internal/api; those hooks are inactive during migration, so we backfill
// existing rows explicitly here.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.TextField{
			Name:     "auth_string_hash",
			Required: true,
			Max:      64,
		})
		users.AddIndex("idx_users_auth_string_hash", true, "auth_string_hash", "")
		if err := app.Save(users); err != nil {
			return err
		}

		records, err := app.FindRecordsByFilter("users", "", "", 0, 0)
		if err != nil {
			return err
		}
		for _, rec := range records {
			rec.Set("auth_string_hash", token.Sha256Hex(rec.GetString("auth_string")))
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.RemoveIndex("idx_users_auth_string_hash")
		users.Fields.RemoveByName("auth_string_hash")
		return app.Save(users)
	})
}
