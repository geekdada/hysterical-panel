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

		ignored := core.NewBaseCollection("ignored_connection_ips")
		ignored.Fields.Add(&core.TextField{Name: "ip", Required: true, Max: 45})
		ignored.Fields.Add(&core.RelationField{
			Name:         "created_by",
			MaxSelect:    1,
			CollectionId: users.Id,
		})
		ignored.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		ignored.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		ignored.AddIndex("idx_ignored_connection_ips_ip", true, "ip", "")
		return app.Save(ignored)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("ignored_connection_ips")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
