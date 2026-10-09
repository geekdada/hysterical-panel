package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUserMonitorAPIOmitsNodeScope(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app, monitoring: &monitorLifecycleSpy{}}

	e, response := notificationChannelEvent(t, app, http.MethodPost, "", map[string]any{
		"name":                  "Low allowance",
		"kind":                  "low_allowance",
		"notification_language": "en",
		"channel_ids":           []string{},
		"config":                map[string]any{"threshold_percent": 10},
	})
	if err := h.createMonitor(e); err != nil {
		t.Fatalf("createMonitor() error = %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode monitor: %v", err)
	}
	for _, field := range []string{"evaluation_window_seconds", "node_scope", "node_ids"} {
		if value, present := created[field]; !present || value != nil {
			t.Fatalf("%s = %v (present %v), want null", field, value, present)
		}
	}
	id, _ := created["id"].(string)

	e, _ = notificationChannelEvent(t, app, http.MethodPatch, id, map[string]any{"kind": "expiring_subscription", "config": map[string]any{"threshold_days": 7}})
	if err := h.updateMonitor(e); err != nil {
		t.Fatalf("switch to expiring_subscription: %v", err)
	}
	e, _ = notificationChannelEvent(t, app, http.MethodPatch, id, map[string]any{"kind": "offline", "config": map[string]any{}})
	if err := h.updateMonitor(e); err == nil || !strings.Contains(strings.ToLower(err.Error()), "evaluation_window_seconds") {
		t.Fatalf("switch to node kind without a window error = %v", err)
	}
}

func TestUserMonitorAPIRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"node scope", map[string]any{"node_scope": "all_enabled", "config": map[string]any{"threshold_percent": 10}}, "do not take"},
		{"evaluation window", map[string]any{"evaluation_window_seconds": 300, "config": map[string]any{"threshold_percent": 10}}, "do not take"},
		{"percent too high", map[string]any{"config": map[string]any{"threshold_percent": 100}}, "between 1 and 99"},
		{"fractional percent", map[string]any{"config": map[string]any{"threshold_percent": 9.5}}, "between 1 and 99"},
		{"wrong config key", map[string]any{"config": map[string]any{"threshold_days": 7}}, "threshold_percent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newMigratedTestApp(t)
			h := &Handlers{app: app, monitoring: &monitorLifecycleSpy{}}
			body := map[string]any{"name": "Low", "kind": "low_allowance", "notification_language": "en", "channel_ids": []string{}}
			for key, value := range tc.body {
				body[key] = value
			}
			e, _ := notificationChannelEvent(t, app, http.MethodPost, "", body)
			if err := h.createMonitor(e); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("createMonitor() error = %v, want %q", err, tc.want)
			}
		})
	}
}
