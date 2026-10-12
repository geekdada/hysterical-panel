package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Pin the issuer the OIDC provider published when it was turned on, so a
// later PANEL_BACKEND_URL_BASE change is detected instead of silently
// breaking every Connected App (ADR-0010).
func init() {
	m.Register(func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(&core.TextField{Name: "oidc_issuer", Max: 2048})
		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("oidc_issuer")
		return app.Save(settings)
	})
}
