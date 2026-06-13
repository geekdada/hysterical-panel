package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// app_settings is a singleton holding runtime-mutable feature flags.
		// No API rules are set, so it is only reachable from server-side Go code
		// (custom /api/panel/settings handlers), never via the auto collection API.
		settings := core.NewBaseCollection("app_settings")
		settings.Fields.Add(&core.BoolField{Name: "invitations_enabled"})
		settings.Fields.Add(&core.BoolField{Name: "open_registration"})
		settings.Fields.Add(&core.BoolField{Name: "require_invite_for_open"})
		settings.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		settings.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		if err := app.Save(settings); err != nil {
			return err
		}

		// Seed the single settings record with safe defaults (everything off),
		// so upgrading deployments keep the current admin-only behaviour until an
		// admin opts in.
		rec := core.NewRecord(settings)
		rec.Set("invitations_enabled", false)
		rec.Set("open_registration", false)
		rec.Set("require_invite_for_open", false)
		return app.Save(rec)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
