package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// window_allowance_bytes fixes an Allowance Window's base allowance so a later
// Type edit applies only to windows that have not started (ADR-0012). Existing
// grants need no backfill: an unfixed window follows the Type, which is what it
// shows today, and the next Type edit fixes it before changing the Type.
func init() {
	m.Register(func(app core.App) error {
		grants, err := app.FindCollectionByNameOrId("user_subscriptions")
		if err != nil {
			return err
		}
		// Decimal text for the same float64 reason as 1730000025.
		grants.Fields.Add(&core.TextField{Name: "window_allowance_bytes"})
		return app.Save(grants)
	}, func(app core.App) error {
		grants, err := app.FindCollectionByNameOrId("user_subscriptions")
		if err != nil {
			return err
		}
		grants.Fields.RemoveByName("window_allowance_bytes")
		return app.Save(grants)
	})
}
