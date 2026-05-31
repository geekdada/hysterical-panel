package migrations

import (
	"errors"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add the non-admin account role used for self-service account diagnostics.
func init() {
	m.Register(func(app core.App) error {
		return setUserRoleValues(app, []string{"admin", "user"})
	}, func(app core.App) error {
		return setUserRoleValues(app, []string{"admin"})
	})
}

func setUserRoleValues(app core.App, values []string) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	role, ok := users.Fields.GetByName("role").(*core.SelectField)
	if !ok {
		return errors.New("users.role select field not found")
	}
	role.Values = values
	return app.Save(users)
}
