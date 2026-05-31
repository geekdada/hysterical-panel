package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("nodes")

		c.Fields.Add(&core.TextField{
			Name:     "name",
			Required: true,
			Max:      128,
		})
		c.Fields.Add(&core.URLField{
			Name:     "api_url",
			Required: true,
		})
		c.Fields.Add(&core.TextField{
			Name:     "api_secret",
			Required: true,
			Hidden:   true,
			Max:      4096,
		})
		c.Fields.Add(&core.NumberField{
			Name:    "poll_interval",
			OnlyInt: true,
		})
		c.Fields.Add(&core.BoolField{
			Name: "enabled",
		})
		c.Fields.Add(&core.DateField{
			Name: "last_polled_at",
		})
		c.Fields.Add(&core.TextField{
			Name: "last_error",
			Max:  2000,
		})
		c.Fields.Add(&core.AutodateField{
			Name:     "created",
			OnCreate: true,
		})
		c.Fields.Add(&core.AutodateField{
			Name:     "updated",
			OnCreate: true,
			OnUpdate: true,
		})

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
