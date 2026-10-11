package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Lock the generic users record API to superusers. PocketBase's default rules
// let anyone create a user and let users update their own record, which would
// expose role, status and usage fields. The panel manages users only through
// /api/panel/* and the built-in auth endpoints, which do not depend on these
// rules. A nil create rule also stops OAuth2 logins from creating new users.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.ListRule = nil
		users.ViewRule = nil
		users.CreateRule = nil
		users.UpdateRule = nil
		users.DeleteRule = nil
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		ownerRule := "id = @request.auth.id"
		users.ListRule = types.Pointer(ownerRule)
		users.ViewRule = types.Pointer(ownerRule)
		users.CreateRule = types.Pointer("")
		users.UpdateRule = types.Pointer(ownerRule)
		users.DeleteRule = types.Pointer(ownerRule)
		return app.Save(users)
	})
}
