package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestMonitoringLanguageMigrationSchema(t *testing.T) {
	app := newMigratedTestApp(t)

	monitors, err := app.FindCollectionByNameOrId("monitors")
	if err != nil {
		t.Fatalf("find monitors: %v", err)
	}
	assertLanguageSelectField(t, monitors, "notification_language")

	alerts, err := app.FindCollectionByNameOrId("alerts")
	if err != nil {
		t.Fatalf("find alerts: %v", err)
	}
	assertLanguageSelectField(t, alerts, "notification_language_snapshot")
}

func TestMonitoringLanguageMigrationBackfillsEnglishAndRollsBack(t *testing.T) {
	app := newMigratedTestApp(t)
	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert notification language migration: reverted=%v err=%v", reverted, err)
	}

	monitor := newPreLanguageMonitor(t, app)
	alert := newPreLanguageAlert(t, app, monitor)
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply notification language migration: %v", err)
	}

	monitor, err := app.FindRecordById("monitors", monitor.Id)
	if err != nil {
		t.Fatalf("reload monitor: %v", err)
	}
	alert, err = app.FindRecordById("alerts", alert.Id)
	if err != nil {
		t.Fatalf("reload alert: %v", err)
	}
	if got := monitor.GetString("notification_language"); got != "en" {
		t.Fatalf("backfilled monitor language = %q, want en", got)
	}
	if got := alert.GetString("notification_language_snapshot"); got != "en" {
		t.Fatalf("backfilled alert language = %q, want en", got)
	}

	if _, err := runner.Down(1); err != nil {
		t.Fatalf("rollback notification language migration: %v", err)
	}
	monitors, _ := app.FindCollectionByNameOrId("monitors")
	alerts, _ := app.FindCollectionByNameOrId("alerts")
	if monitors.Fields.GetByName("notification_language") != nil || alerts.Fields.GetByName("notification_language_snapshot") != nil {
		t.Fatal("rollback left notification language fields behind")
	}
}

func TestMonitorAPIStoresValidatesAndReturnsNotificationLanguage(t *testing.T) {
	app := newMigratedTestApp(t)
	lifecycle := &monitorLifecycleSpy{}
	h := &Handlers{app: app, monitoring: lifecycle}

	createBody := map[string]any{
		"name":                      "Fleet offline",
		"kind":                      "offline",
		"enabled":                   true,
		"severity":                  "warning",
		"evaluation_window_seconds": 300,
		"node_scope":                "all_enabled",
		"node_ids":                  []string{},
		"channel_ids":               []string{},
		"notification_language":     "zh-cn",
		"config":                    map[string]any{},
	}
	e, response := notificationChannelEvent(t, app, http.MethodPost, "", createBody)
	if err := h.createMonitor(e); err != nil {
		t.Fatalf("createMonitor() error = %v", err)
	}
	var created Monitor
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode monitor: %v", err)
	}
	if created.NotificationLanguage != "zh-cn" {
		t.Fatalf("notification_language = %q, want zh-cn", created.NotificationLanguage)
	}

	patchBody := map[string]any{"notification_language": "fr"}
	e, _ = notificationChannelEvent(t, app, http.MethodPatch, created.ID, patchBody)
	if err := h.updateMonitor(e); err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid monitor notification language") {
		t.Fatalf("invalid language error = %v", err)
	}

	patchBody = map[string]any{"notification_language": "en"}
	e, response = notificationChannelEvent(t, app, http.MethodPatch, created.ID, patchBody)
	if err := h.updateMonitor(e); err != nil {
		t.Fatalf("updateMonitor() language-only error = %v", err)
	}
	if lifecycle.cancelCalls != 0 {
		t.Fatalf("language-only update cancelled alerts %d times", lifecycle.cancelCalls)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode updated monitor: %v", err)
	}
	if created.NotificationLanguage != "en" {
		t.Fatalf("updated notification_language = %q, want en", created.NotificationLanguage)
	}
}

func assertLanguageSelectField(t *testing.T, collection *core.Collection, name string) {
	t.Helper()
	field, ok := collection.Fields.GetByName(name).(*core.SelectField)
	if !ok {
		t.Fatalf("%s.%s is not a select field", collection.Name, name)
	}
	if !field.Required {
		t.Fatalf("%s.%s must be required", collection.Name, name)
	}
	if got := strings.Join(field.Values, ","); got != "en,zh-cn" {
		t.Fatalf("%s.%s values = %q, want en,zh-cn", collection.Name, name, got)
	}
}

type monitorLifecycleSpy struct {
	cancelCalls int
}

func (s *monitorLifecycleSpy) EvaluateNow(context.Context) {}

func (s *monitorLifecycleSpy) CancelMonitorAlerts(string, string) error {
	s.cancelCalls++
	return nil
}

func (s *monitorLifecycleSpy) CancelNodeAlerts(string, string) error {
	return nil
}

func newPreLanguageMonitor(t *testing.T, app core.App) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("monitors")
	if err != nil {
		t.Fatalf("find monitors: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", "Legacy offline")
	record.Set("name_key", "legacy offline")
	record.Set("kind", "offline")
	record.Set("enabled", true)
	record.Set("severity", "warning")
	record.Set("evaluation_window_seconds", 300)
	record.Set("node_scope", "all_enabled")
	record.Set("nodes", []string{})
	record.Set("channels", []string{})
	record.Set("config", map[string]any{})
	if err := app.Save(record); err != nil {
		t.Fatalf("save legacy monitor: %v", err)
	}
	return record
}

func newPreLanguageAlert(t *testing.T, app core.App, monitor *core.Record) *core.Record {
	t.Helper()
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes: %v", err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "edge-legacy")
	node.Set("api_url", "https://legacy-node.example")
	node.Set("api_secret", "encrypted-secret")
	node.Set("poll_interval", 30)
	node.Set("enabled", true)
	if err := app.Save(node); err != nil {
		t.Fatalf("save legacy node: %v", err)
	}

	alerts, err := app.FindCollectionByNameOrId("alerts")
	if err != nil {
		t.Fatalf("find alerts: %v", err)
	}
	record := core.NewRecord(alerts)
	record.Set("monitor", monitor.Id)
	record.Set("node", node.Id)
	record.Set("status", "firing")
	record.Set("severity_snapshot", "warning")
	record.Set("monitor_name_snapshot", monitor.GetString("name"))
	record.Set("monitor_kind_snapshot", "offline")
	record.Set("monitor_config_snapshot", map[string]any{})
	record.Set("evaluation_window_seconds_snapshot", 300)
	record.Set("channel_ids_snapshot", []string{})
	record.Set("firing_value", map[string]any{"stale_seconds": 300})
	record.Set("started_at", "2026-07-12T10:00:00Z")
	record.Set("last_evaluated_at", "2026-07-12T10:00:00Z")
	if err := app.Save(record); err != nil {
		t.Fatalf("save legacy alert: %v", err)
	}
	return record
}
