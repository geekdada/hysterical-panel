package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Admin-curated links shown to every User outside the panel.
func init() {
	m.Register(func(app core.App) error {
		links := core.NewBaseCollection("resource_links")
		links.Fields.Add(&core.TextField{Name: "url", Required: true, Max: 2048})
		links.Fields.Add(&core.TextField{Name: "title", Max: 200})
		links.Fields.Add(&core.TextField{Name: "fetched_title", Max: 200})
		// favicon_data is the base64 of an icon capped at 100 KiB. Hidden keeps
		// it out of every PocketBase API response.
		links.Fields.Add(&core.TextField{Name: "favicon_data", Max: 140000, Hidden: true})
		links.Fields.Add(&core.TextField{Name: "favicon_type", Max: 100})
		links.Fields.Add(&core.TextField{Name: "favicon_hash", Max: 16})
		links.Fields.Add(&core.NumberField{Name: "sort_order", OnlyInt: true})
		links.Fields.Add(&core.NumberField{Name: "visit_count", OnlyInt: true})
		links.Fields.Add(&core.DateField{Name: "last_visited_at"})
		links.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		links.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		links.AddIndex("idx_resource_links_sort_order", false, "sort_order", "")
		return app.Save(links)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("resource_links")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
