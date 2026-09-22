package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

var subscriptionUsageFields = []string{"used_tx_bytes", "used_rx_bytes", "grant_tx_bytes", "grant_rx_bytes"}

// Subscriptions had not launched, so the combined window usage is dropped
// rather than assigned to an invented direction.
func init() {
	m.Register(func(app core.App) error {
		grants, err := app.FindCollectionByNameOrId("user_subscriptions")
		if err != nil {
			return err
		}
		grants.Fields.RemoveByName("used_bytes")
		for _, name := range subscriptionUsageFields {
			// Decimal text for the same float64 reason as 1730000025.
			grants.Fields.Add(&core.TextField{Name: name})
		}
		return app.Save(grants)
	}, func(app core.App) error {
		grants, err := app.FindCollectionByNameOrId("user_subscriptions")
		if err != nil {
			return err
		}
		grants.Fields.Add(&core.TextField{Name: "used_bytes"})
		if err := app.Save(grants); err != nil {
			return err
		}
		records, err := app.FindAllRecords("user_subscriptions")
		if err != nil {
			return err
		}
		for _, record := range records {
			record.Set("used_bytes", int64(record.GetInt("used_tx_bytes"))+int64(record.GetInt("used_rx_bytes")))
			if err := app.Save(record); err != nil {
				return err
			}
		}
		for _, name := range subscriptionUsageFields {
			grants.Fields.RemoveByName(name)
		}
		return app.Save(grants)
	})
}
