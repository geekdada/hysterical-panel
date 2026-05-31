package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Extend the default "users" auth collection with the fields the panel needs.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.SelectField{
			Name:      "role",
			Required:  true,
			MaxSelect: 1,
			Values:    []string{"admin"},
		})

		users.Fields.Add(&core.TextField{
			Name:     "auth_string",
			Required: true,
			Max:      128,
		})

		users.Fields.Add(&core.NumberField{
			Name:    "quota_bytes",
			OnlyInt: true,
		})

		users.Fields.Add(&core.NumberField{
			Name:    "used_tx",
			OnlyInt: true,
		})

		users.Fields.Add(&core.NumberField{
			Name:    "used_rx",
			OnlyInt: true,
		})

		users.Fields.Add(&core.SelectField{
			Name:      "status",
			Required:  true,
			MaxSelect: 1,
			Values:    []string{"active", "disabled"},
		})

		users.Fields.Add(&core.BoolField{
			Name: "enabled",
		})

		users.AddIndex("idx_users_auth_string", true, "auth_string", "")

		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		for _, name := range []string{"role", "auth_string", "quota_bytes", "used_tx", "used_rx", "status", "enabled"} {
			users.Fields.RemoveByName(name)
		}
		users.RemoveIndex("idx_users_auth_string")
		return app.Save(users)
	})
}
