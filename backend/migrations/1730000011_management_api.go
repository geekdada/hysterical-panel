package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Extend the app_settings singleton with the Management API feature flag and
// the SHA-256 hash of its bearer token. The token hash is empty by default;
// admins set it via PATCH /api/panel/settings before enabling the surface.
func init() {
	m.Register(func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(&core.BoolField{Name: "management_api_enabled"})
		settings.Fields.Add(&core.TextField{Name: "management_api_token_hash", Max: 64})
		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("management_api_enabled")
		settings.Fields.RemoveByName("management_api_token_hash")
		return app.Save(settings)
	})
}
