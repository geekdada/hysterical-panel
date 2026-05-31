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

		c := core.NewBaseCollection("traffic_hourly")
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
		c.Fields.Add(&core.DateField{Name: "bucket", Required: true})
		c.Fields.Add(&core.NumberField{Name: "tx", OnlyInt: true})
		c.Fields.Add(&core.NumberField{Name: "rx", OnlyInt: true})
		c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

		c.AddIndex("idx_traffic_hourly_user_node_bucket", true, "user, node, bucket", "")
		c.AddIndex("idx_traffic_hourly_bucket", false, "bucket", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("traffic_hourly")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
