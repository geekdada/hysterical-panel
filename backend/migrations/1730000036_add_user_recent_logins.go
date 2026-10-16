package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Record when and where each User last signed in to the panel, plus the
// latest unique sign-in IPs.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.Add(&core.DateField{Name: "last_login_at"})
		users.Fields.Add(&core.TextField{Name: "last_login_ip", Max: 64})
		users.Fields.Add(&core.JSONField{Name: "recent_login_ips", MaxSize: 1024})
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.RemoveByName("last_login_at")
		users.Fields.RemoveByName("last_login_ip")
		users.Fields.RemoveByName("recent_login_ips")
		return app.Save(users)
	})
}
