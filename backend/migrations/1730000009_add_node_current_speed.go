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
		nodes.Fields.Add(&core.NumberField{Name: "current_tx_speed", OnlyInt: true})
		nodes.Fields.Add(&core.NumberField{Name: "current_rx_speed", OnlyInt: true})
		return app.Save(nodes)
	}, func(app core.App) error {
		nodes, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		nodes.Fields.RemoveByName("current_tx_speed")
		nodes.Fields.RemoveByName("current_rx_speed")
		return app.Save(nodes)
	})
}
