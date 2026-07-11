package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		return setObservationByteFieldsRequired(app, false)
	}, func(app core.App) error {
		return setObservationByteFieldsRequired(app, true)
	})
}

func setObservationByteFieldsRequired(app core.App, required bool) error {
	observations, err := app.FindCollectionByNameOrId("monitor_observations")
	if err != nil {
		return err
	}
	for _, name := range []string{"tx_bytes", "rx_bytes"} {
		field, ok := observations.Fields.GetByName(name).(*core.NumberField)
		if !ok {
			return fmt.Errorf("monitor_observations.%s is not a number field", name)
		}
		// Zero is a valid counter delta for idle and one-way traffic intervals.
		// PocketBase treats zero as blank when Required is enabled.
		field.Required = required
	}
	return app.Save(observations)
}
