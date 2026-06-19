package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		nodes, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		nodes.Fields.Add(&core.DateField{Name: "deleted_at"})
		return app.Save(nodes)
	}, func(app core.App) error {
		nodes, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		nodes.Fields.RemoveByName("deleted_at")
		return app.Save(nodes)
	})
}
