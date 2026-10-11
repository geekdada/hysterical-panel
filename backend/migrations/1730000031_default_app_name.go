package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	pocketBaseDefaultAppName = "Acme"
	panelDefaultAppName      = "Hysterical Panel"
)

// The panel shows PocketBase's Application Name as its brand. Replace the
// stock "Acme" placeholder so fresh installs keep the product name. A name an
// admin already chose is left alone.
func init() {
	m.Register(func(app core.App) error {
		return renameApp(app, pocketBaseDefaultAppName, panelDefaultAppName)
	}, func(app core.App) error {
		return renameApp(app, panelDefaultAppName, pocketBaseDefaultAppName)
	})
}

func renameApp(app core.App, from, to string) error {
	settings := app.Settings()
	if settings.Meta.AppName != from {
		return nil
	}
	settings.Meta.AppName = to
	return app.Save(settings)
}
