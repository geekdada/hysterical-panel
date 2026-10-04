package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		cursors, err := app.FindCollectionByNameOrId("traffic_cursor")
		if err != nil {
			return err
		}
		cursors.Fields.Add(&core.NumberField{Name: "current_tx_speed", OnlyInt: true})
		cursors.Fields.Add(&core.NumberField{Name: "current_rx_speed", OnlyInt: true})
		return app.Save(cursors)
	}, func(app core.App) error {
		cursors, err := app.FindCollectionByNameOrId("traffic_cursor")
		if err != nil {
			return err
		}
		cursors.Fields.RemoveByName("current_tx_speed")
		cursors.Fields.RemoveByName("current_rx_speed")
		return app.Save(cursors)
	})
}
