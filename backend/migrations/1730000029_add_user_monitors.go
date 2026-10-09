package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

var (
	nodeMonitorKinds       = []string{"offline", "high_traffic"}
	allMonitorKinds        = []string{"offline", "high_traffic", "low_allowance", "expiring_subscription"}
	nodeResolutionReasons  = []string{"condition_cleared", "monitor_disabled", "monitor_deleted", "node_disabled", "node_removed_from_scope", "monitor_reconfigured"}
	userResolutionReasons  = []string{"user_unavailable", "user_deleted", "subscription_ended"}
	nodeOnlyMonitorFields  = []string{"evaluation_window_seconds", "node_scope"}
	nodeOnlyAlertSnapshots = []string{"node", "evaluation_window_seconds_snapshot"}
)

// User Monitors have no Node scope or evaluation window, and their Alerts
// reference a User instead of a Node (ADR-0009).
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		monitors, err := app.FindCollectionByNameOrId("monitors")
		if err != nil {
			return err
		}
		if err := setSelectValues(monitors, "kind", allMonitorKinds); err != nil {
			return err
		}
		if err := setRequired(monitors, nodeOnlyMonitorFields, false); err != nil {
			return err
		}
		if err := app.Save(monitors); err != nil {
			return err
		}

		alerts, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		if err := setSelectValues(alerts, "monitor_kind_snapshot", allMonitorKinds); err != nil {
			return err
		}
		if err := setSelectValues(alerts, "resolution_reason", append(append([]string{}, nodeResolutionReasons...), userResolutionReasons...)); err != nil {
			return err
		}
		if err := setRequired(alerts, nodeOnlyAlertSnapshots, false); err != nil {
			return err
		}
		alerts.Fields.Add(&core.RelationField{Name: "user", MaxSelect: 1, CollectionId: users.Id})
		alerts.Fields.Add(&core.TextField{Name: "user_email_snapshot", Max: 255})
		alerts.RemoveIndex("idx_alerts_one_firing")
		alerts.AddIndex("idx_alerts_one_firing", true, "monitor, node, user", "status = 'firing'")
		alerts.AddIndex("idx_alerts_user_status", false, "user, status", "")
		return app.Save(alerts)
	}, func(app core.App) error {
		if _, err := app.DB().NewQuery("DELETE FROM alerts WHERE node = ''").Execute(); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery("DELETE FROM monitors WHERE kind NOT IN ('offline', 'high_traffic')").Execute(); err != nil {
			return err
		}
		alerts, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		alerts.RemoveIndex("idx_alerts_user_status")
		alerts.RemoveIndex("idx_alerts_one_firing")
		alerts.AddIndex("idx_alerts_one_firing", true, "monitor, node", "status = 'firing'")
		alerts.Fields.RemoveByName("user")
		alerts.Fields.RemoveByName("user_email_snapshot")
		if err := setSelectValues(alerts, "monitor_kind_snapshot", nodeMonitorKinds); err != nil {
			return err
		}
		if err := setSelectValues(alerts, "resolution_reason", nodeResolutionReasons); err != nil {
			return err
		}
		if err := setRequired(alerts, nodeOnlyAlertSnapshots, true); err != nil {
			return err
		}
		if err := app.Save(alerts); err != nil {
			return err
		}

		monitors, err := app.FindCollectionByNameOrId("monitors")
		if err != nil {
			return err
		}
		if err := setSelectValues(monitors, "kind", nodeMonitorKinds); err != nil {
			return err
		}
		if err := setRequired(monitors, nodeOnlyMonitorFields, true); err != nil {
			return err
		}
		return app.Save(monitors)
	})
}

func setSelectValues(collection *core.Collection, name string, values []string) error {
	field, ok := collection.Fields.GetByName(name).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s select field not found", collection.Name, name)
	}
	field.Values = values
	return nil
}

func setRequired(collection *core.Collection, names []string, required bool) error {
	for _, name := range names {
		switch field := collection.Fields.GetByName(name).(type) {
		case *core.NumberField:
			field.Required = required
		case *core.SelectField:
			field.Required = required
		case *core.RelationField:
			field.Required = required
		default:
			return fmt.Errorf("%s.%s cannot change required", collection.Name, name)
		}
	}
	return nil
}
