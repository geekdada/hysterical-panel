package monitoring

import (
	"strings"
	"testing"
	"time"

	_ "hysterical-panel/migrations"

	"github.com/pocketbase/pocketbase/core"
)

func TestNotificationMessageEnglishHighTraffic(t *testing.T) {
	service := &Service{frontendURL: "https://panel.example"}
	alert, node := notificationMessageRecords("en", "high_traffic", "critical")
	alert.Set("firing_value", map[string]any{
		"average_bytes_per_second":   1572864,
		"threshold_bytes_per_second": 1048576,
	})

	got := service.message(alert, node, "firing", time.Date(2026, 7, 12, 10, 2, 3, 0, time.UTC))
	want := strings.Join([]string{
		"[FIRING] High traffic alert",
		"Severity: Critical",
		"Monitor: 核心流量",
		"Node: edge-1",
		"Average rate: 1.5 MB/s",
		"Threshold: 1.0 MB/s",
		"Evaluation window: 5m",
		"Started: 2026-07-12T10:00:00Z",
		"Duration: 2m 3s",
		"View node: https://panel.example/nodes/node-1",
	}, "\n")
	if got != want {
		t.Fatalf("message() =\n%s\n\nwant:\n%s", got, want)
	}
}

func TestNotificationMessageChineseOfflineRecovery(t *testing.T) {
	service := &Service{frontendURL: "https://panel.example"}
	alert, node := notificationMessageRecords("zh-cn", "offline", "warning")
	alert.Set("recovery_value", map[string]any{
		"last_successful_poll_at": "2026-07-12T10:01:30Z",
		"stale_seconds":           30,
	})

	got := service.message(alert, node, "resolved", time.Date(2026, 7, 12, 10, 2, 3, 0, time.UTC))
	want := strings.Join([]string{
		"[告警恢复] 节点离线",
		"严重程度：警告",
		"监控项：核心流量",
		"节点：edge-1",
		"最后成功采集：2026-07-12T10:01:30Z",
		"距上次成功：30 秒",
		"评估窗口：5 分钟",
		"开始时间：2026-07-12T10:00:00Z",
		"持续时间：2 分 3 秒",
		"查看节点：https://panel.example/nodes/node-1",
	}, "\n")
	if got != want {
		t.Fatalf("message() =\n%s\n\nwant:\n%s", got, want)
	}
}

func TestNotificationMessageUnknownLanguageFallsBackToEnglish(t *testing.T) {
	service := &Service{frontendURL: "https://panel.example"}
	alert, node := notificationMessageRecords("unsupported", "offline", "warning")
	alert.Set("firing_value", map[string]any{
		"last_successful_poll_at": "2026-07-12T09:55:00Z",
		"stale_seconds":           300,
	})

	got := service.message(alert, node, "firing", time.Date(2026, 7, 12, 10, 2, 3, 0, time.UTC))
	if !strings.HasPrefix(got, "[FIRING] Offline alert\nSeverity: Warning") {
		t.Fatalf("unknown language did not fall back to English:\n%s", got)
	}
}

func TestNotificationMessageLocalizationMatrix(t *testing.T) {
	cases := []struct {
		name       string
		language   string
		kind       string
		transition string
		want       []string
	}{
		{"English offline firing", "en", "offline", "firing", []string{"[FIRING] Offline alert", "Last successful poll: 2026-07-12T09:55:00Z", "Time since last success: 5m"}},
		{"English offline resolved", "en", "offline", "resolved", []string{"[RESOLVED] Offline alert", "Severity: Critical", "Duration: 2m 3s"}},
		{"English high traffic firing", "en", "high_traffic", "firing", []string{"[FIRING] High traffic alert", "Average rate: 1.5 MB/s", "Threshold: 1.0 MB/s"}},
		{"English high traffic resolved", "en", "high_traffic", "resolved", []string{"[RESOLVED] High traffic alert", "Evaluation window: 5m", "Started: 2026-07-12T10:00:00Z"}},
		{"Chinese offline firing", "zh-cn", "offline", "firing", []string{"[告警触发] 节点离线", "最后成功采集：2026-07-12T09:55:00Z", "距上次成功：5 分钟"}},
		{"Chinese offline resolved", "zh-cn", "offline", "resolved", []string{"[告警恢复] 节点离线", "严重程度：严重", "持续时间：2 分 3 秒"}},
		{"Chinese high traffic firing", "zh-cn", "high_traffic", "firing", []string{"[告警触发] 高流量", "平均速率：1.5 MB/s", "阈值：1.0 MB/s"}},
		{"Chinese high traffic resolved", "zh-cn", "high_traffic", "resolved", []string{"[告警恢复] 高流量", "评估窗口：5 分钟", "开始时间：2026-07-12T10:00:00Z"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{frontendURL: "https://panel.example"}
			alert, node := notificationMessageRecords(tc.language, tc.kind, "critical")
			value := map[string]any{
				"last_successful_poll_at":    "2026-07-12T09:55:00Z",
				"stale_seconds":              300,
				"average_bytes_per_second":   1572864,
				"threshold_bytes_per_second": 1048576,
			}
			alert.Set("firing_value", value)
			alert.Set("recovery_value", value)
			got := service.message(alert, node, tc.transition, time.Date(2026, 7, 12, 10, 2, 3, 0, time.UTC))
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("message missing %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "high_traffic") || strings.Contains(got, `\"average_bytes_per_second\"`) {
				t.Fatalf("message leaked machine-facing values:\n%s", got)
			}
		})
	}
}

func TestAlertSnapshotsNotificationLanguageForItsWholeLifecycle(t *testing.T) {
	app := newMonitoringTestApp(t)
	node := newMonitoringNode(t, app)
	monitor := newMonitoringMonitor(t, app, "zh-cn")
	service := New(app, nil, nil, "https://panel.example")
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)

	if err := service.openAlert(monitor, node, map[string]any{"stale_seconds": 300}, now); err != nil {
		t.Fatalf("openAlert() error = %v", err)
	}
	first, err := service.findFiring(monitor.Id, node.Id)
	if err != nil || first == nil {
		t.Fatalf("find first firing alert: alert=%v err=%v", first, err)
	}
	if got := first.GetString("notification_language_snapshot"); got != "zh-cn" {
		t.Fatalf("first alert language = %q, want zh-cn", got)
	}
	firstMessage := service.message(first, node, "firing", now)
	if !strings.Contains(firstMessage, "开始时间：2026-07-12T10:00:00Z") {
		t.Fatalf("persisted alert start time is not RFC3339 UTC:\n%s", firstMessage)
	}

	monitor.Set("notification_language", "en")
	if err := app.Save(monitor); err != nil {
		t.Fatalf("save monitor language: %v", err)
	}
	if got := first.GetString("notification_language_snapshot"); got != "zh-cn" {
		t.Fatalf("existing alert language changed to %q", got)
	}
	if err := service.resolveAlert(first, node, map[string]any{"stale_seconds": 0}, now.Add(time.Minute)); err != nil {
		t.Fatalf("resolveAlert() error = %v", err)
	}
	if err := service.openAlert(monitor, node, map[string]any{"stale_seconds": 300}, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("open second alert: %v", err)
	}
	second, err := service.findFiring(monitor.Id, node.Id)
	if err != nil || second == nil {
		t.Fatalf("find second firing alert: alert=%v err=%v", second, err)
	}
	if got := second.GetString("notification_language_snapshot"); got != "en" {
		t.Fatalf("second alert language = %q, want en", got)
	}
}

func TestNotificationRateCapsAtGigabytesPerSecond(t *testing.T) {
	if got := formatNotificationRate(5 * 1024 * 1024 * 1024 * 1024); got != "5120.0 GB/s" {
		t.Fatalf("formatNotificationRate() = %q, want 5120.0 GB/s", got)
	}
}

func notificationMessageRecords(language, kind, severity string) (*core.Record, *core.Record) {
	alerts := core.NewBaseCollection("alerts")
	alert := core.NewRecord(alerts)
	alert.Set("notification_language_snapshot", language)
	alert.Set("monitor_kind_snapshot", kind)
	alert.Set("severity_snapshot", severity)
	alert.Set("monitor_name_snapshot", "核心流量")
	alert.Set("evaluation_window_seconds_snapshot", 300)
	alert.Set("started_at", "2026-07-12T10:00:00Z")

	nodes := core.NewBaseCollection("nodes")
	node := core.NewRecord(nodes)
	node.Id = "node-1"
	node.Set("name", "edge-1")
	return alert, node
}

func newMonitoringTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

func newMonitoringNode(t *testing.T, app core.App) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", "edge-1")
	record.Set("api_url", "https://node.example")
	record.Set("api_secret", "encrypted-secret")
	record.Set("poll_interval", 30)
	record.Set("enabled", true)
	if err := app.Save(record); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return record
}

func newMonitoringMonitor(t *testing.T, app core.App, language string) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("monitors")
	if err != nil {
		t.Fatalf("find monitors: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", "Fleet offline")
	record.Set("name_key", "fleet offline")
	record.Set("kind", "offline")
	record.Set("enabled", true)
	record.Set("severity", "warning")
	record.Set("notification_language", language)
	record.Set("evaluation_window_seconds", 300)
	record.Set("node_scope", "all_enabled")
	record.Set("nodes", []string{})
	record.Set("channels", []string{})
	record.Set("config", map[string]any{})
	if err := app.Save(record); err != nil {
		t.Fatalf("save monitor: %v", err)
	}
	return record
}
