package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// The registration flags now form a strict hierarchy where open_registration is
// the master switch (see internal/api.registrationDecision). Legacy deployments
// could be in the old "invite-only" state (invitations_enabled=true while
// open_registration=false), which previously opened registration behind a
// mandatory code. Preserve that behaviour by promoting such records to the
// equivalent hierarchical state: open registration on, invite code required.
func init() {
	m.Register(func(app core.App) error {
		recs, err := app.FindRecordsByFilter("app_settings", "", "", 0, 0)
		if err != nil {
			return err
		}
		for _, rec := range recs {
			if rec.GetBool("invitations_enabled") && !rec.GetBool("open_registration") {
				rec.Set("open_registration", true)
				rec.Set("require_invite_for_open", true)
				if err := app.Save(rec); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(app core.App) error {
		// No safe inverse: the promotion is lossy (the pre-migration state cannot
		// be distinguished from a deliberately hierarchical one). Leave as-is.
		return nil
	})
}
