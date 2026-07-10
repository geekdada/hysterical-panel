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
		nodes.Fields.Add(&core.DateField{Name: "enabled_at"})
		if err := app.Save(nodes); err != nil {
			return err
		}

		channels, err := app.FindCollectionByNameOrId("notification_channels")
		if err != nil {
			return err
		}

		observations := core.NewBaseCollection("monitor_observations")
		observations.Fields.Add(&core.RelationField{Name: "node", Required: true, MaxSelect: 1, CollectionId: nodes.Id, CascadeDelete: true})
		observations.Fields.Add(&core.DateField{Name: "observed_at", Required: true})
		observations.Fields.Add(&core.NumberField{Name: "elapsed_seconds", Required: true, OnlyInt: true})
		observations.Fields.Add(&core.NumberField{Name: "tx_bytes", Required: true, OnlyInt: true})
		observations.Fields.Add(&core.NumberField{Name: "rx_bytes", Required: true, OnlyInt: true})
		observations.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		observations.AddIndex("idx_monitor_observations_node_time", false, "node, observed_at", "")
		observations.AddIndex("idx_monitor_observations_time", false, "observed_at", "")
		if err := app.Save(observations); err != nil {
			return err
		}

		monitors := core.NewBaseCollection("monitors")
		monitors.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 128})
		monitors.Fields.Add(&core.TextField{Name: "name_key", Required: true, Max: 128, Hidden: true})
		monitors.Fields.Add(&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"offline", "high_traffic"}})
		monitors.Fields.Add(&core.BoolField{Name: "enabled"})
		monitors.Fields.Add(&core.SelectField{Name: "severity", Required: true, MaxSelect: 1, Values: []string{"warning", "critical"}})
		monitors.Fields.Add(&core.NumberField{Name: "evaluation_window_seconds", Required: true, OnlyInt: true})
		monitors.Fields.Add(&core.SelectField{Name: "node_scope", Required: true, MaxSelect: 1, Values: []string{"all_enabled", "selected"}})
		monitors.Fields.Add(&core.RelationField{Name: "nodes", CollectionId: nodes.Id, MaxSelect: 999})
		monitors.Fields.Add(&core.RelationField{Name: "channels", CollectionId: channels.Id, MaxSelect: 999})
		monitors.Fields.Add(&core.JSONField{Name: "config", Hidden: true})
		monitors.Fields.Add(&core.DateField{Name: "deleted_at"})
		monitors.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		monitors.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		monitors.AddIndex("idx_monitors_name_key", true, "name_key", "")
		monitors.AddIndex("idx_monitors_active", false, "deleted_at, enabled", "")
		if err := app.Save(monitors); err != nil {
			return err
		}

		alerts := core.NewBaseCollection("alerts")
		alerts.Fields.Add(&core.RelationField{Name: "monitor", Required: true, MaxSelect: 1, CollectionId: monitors.Id})
		alerts.Fields.Add(&core.RelationField{Name: "node", Required: true, MaxSelect: 1, CollectionId: nodes.Id})
		alerts.Fields.Add(&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"firing", "resolved", "cancelled"}})
		alerts.Fields.Add(&core.SelectField{Name: "severity_snapshot", Required: true, MaxSelect: 1, Values: []string{"warning", "critical"}})
		alerts.Fields.Add(&core.TextField{Name: "monitor_name_snapshot", Required: true, Max: 128})
		alerts.Fields.Add(&core.SelectField{Name: "monitor_kind_snapshot", Required: true, MaxSelect: 1, Values: []string{"offline", "high_traffic"}})
		alerts.Fields.Add(&core.JSONField{Name: "monitor_config_snapshot"})
		alerts.Fields.Add(&core.NumberField{Name: "evaluation_window_seconds_snapshot", Required: true, OnlyInt: true})
		alerts.Fields.Add(&core.JSONField{Name: "channel_ids_snapshot"})
		alerts.Fields.Add(&core.JSONField{Name: "firing_value"})
		alerts.Fields.Add(&core.JSONField{Name: "recovery_value"})
		alerts.Fields.Add(&core.DateField{Name: "started_at", Required: true})
		alerts.Fields.Add(&core.DateField{Name: "ended_at"})
		alerts.Fields.Add(&core.DateField{Name: "last_evaluated_at", Required: true})
		alerts.Fields.Add(&core.SelectField{Name: "resolution_reason", MaxSelect: 1, Values: []string{"condition_cleared", "monitor_disabled", "monitor_deleted", "node_disabled", "node_removed_from_scope", "monitor_reconfigured"}})
		alerts.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		alerts.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		alerts.AddIndex("idx_alerts_one_firing", true, "monitor, node", "status = 'firing'")
		alerts.AddIndex("idx_alerts_monitor_node_status", false, "monitor, node, status", "")
		alerts.AddIndex("idx_alerts_status_started", false, "status, started_at", "")
		if err := app.Save(alerts); err != nil {
			return err
		}

		deliveries := core.NewBaseCollection("alert_deliveries")
		deliveries.Fields.Add(&core.RelationField{Name: "alert", Required: true, MaxSelect: 1, CollectionId: alerts.Id, CascadeDelete: true})
		deliveries.Fields.Add(&core.RelationField{Name: "channel", MaxSelect: 1, CollectionId: channels.Id})
		deliveries.Fields.Add(&core.SelectField{Name: "transition", Required: true, MaxSelect: 1, Values: []string{"firing", "resolved"}})
		deliveries.Fields.Add(&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"succeeded", "failed", "skipped"}})
		deliveries.Fields.Add(&core.SelectField{Name: "safe_error", MaxSelect: 1, Values: []string{"timed_out", "delivery_failed", "channel_disabled", "channel_deleted"}})
		deliveries.Fields.Add(&core.DateField{Name: "attempted_at", Required: true})
		deliveries.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		deliveries.AddIndex("idx_alert_deliveries_alert", false, "alert, transition", "")
		return app.Save(deliveries)
	}, func(app core.App) error {
		for _, name := range []string{"alert_deliveries", "alerts", "monitors", "monitor_observations"} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		nodes, err := app.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		nodes.Fields.RemoveByName("enabled_at")
		return app.Save(nodes)
	})
}
