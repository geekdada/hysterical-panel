package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		nodes, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("traffic_cursor")
		c.Fields.Add(&core.RelationField{
			Name:          "user",
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  users.Id,
		})
		c.Fields.Add(&core.RelationField{
			Name:          "node",
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  nodes.Id,
		})
		c.Fields.Add(&core.NumberField{Name: "last_tx", OnlyInt: true})
		c.Fields.Add(&core.NumberField{Name: "last_rx", OnlyInt: true})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

		c.AddIndex("idx_traffic_cursor_user_node", true, "user, node", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("traffic_cursor")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
