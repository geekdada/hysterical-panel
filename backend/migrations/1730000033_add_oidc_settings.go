package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add the OIDC provider switch to app_settings (ADR-0010). It defaults to off,
// so /api/oidc/* stays absent until an admin enables it.
func init() {
	m.Register(func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(&core.BoolField{Name: "oidc_enabled"})
		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("oidc_enabled")
		return app.Save(settings)
	})
}
