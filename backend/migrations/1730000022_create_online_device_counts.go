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

		nodes.Fields.Add(&core.NumberField{Name: "online_devices", OnlyInt: true})
		nodes.Fields.Add(&core.DateField{Name: "online_devices_observed_at"})
		if err := app.Save(nodes); err != nil {
			return err
		}

		counts := core.NewBaseCollection("online_device_counts")
		counts.Fields.Add(&core.RelationField{
			Name:          "user",
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  users.Id,
		})
		counts.Fields.Add(&core.RelationField{
			Name:          "node",
			Required:      true,
			MaxSelect:     1,
			CascadeDelete: true,
			CollectionId:  nodes.Id,
		})
		counts.Fields.Add(&core.NumberField{Name: "count", Required: true, OnlyInt: true, Min: float64Ptr(1)})
		counts.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		counts.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		counts.AddIndex("idx_online_device_counts_user_node", true, "user, node", "")
		counts.AddIndex("idx_online_device_counts_node", false, "node", "")
		return app.Save(counts)
	}, func(app core.App) error {
		counts, err := app.FindCollectionByNameOrId("online_device_counts")
		if err != nil {
			return err
		}
		if err := app.Delete(counts); err != nil {
			return err
		}

		nodes, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		nodes.Fields.RemoveByName("online_devices")
		nodes.Fields.RemoveByName("online_devices_observed_at")
		return app.Save(nodes)
	})
}

func float64Ptr(value float64) *float64 {
	return &value
}
