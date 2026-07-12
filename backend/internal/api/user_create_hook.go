package api

import (
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
)

// bindUserCreateAuthString provisions node credentials for users created
// through PocketBase's generic record API (including the admin dashboard).
// Custom panel registration paths provision credentials explicitly because
// they don't trigger record request hooks.
func (h *Handlers) bindUserCreateAuthString() {
	h.app.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		provision := func(app core.App) error {
			originalApp := e.App
			e.App = app
			defer func() { e.App = originalApp }()

			if err := e.Next(); err != nil {
				return err
			}
			authString, err := generateUniqueAuthString(app)
			if err != nil {
				return err
			}
			_, err = authstrings.CreateCurrent(app, e.Record.Id, authString)
			return err
		}

		if e.App.IsTransactional() {
			return provision(e.App)
		}
		return e.App.RunInTransaction(provision)
	})
}
