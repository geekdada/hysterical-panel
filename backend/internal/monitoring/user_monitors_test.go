package monitoring

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestLowAllowanceAlertLifecycle(t *testing.T) {
	app := newMonitoringTestApp(t)
	service := New(app, nil, nil, "https://panel.example")
	now := time.Now().UTC().Truncate(time.Millisecond)
	user := newMonitoringUser(t, app, "low@example.com")
	grant := newMonitoringGrant(t, app, user, newMonitoringType(t, app, 100), now.Add(-24*time.Hour))
	monitor := newUserMonitor(t, app, KindLowAllowance, map[string]any{"threshold_percent": 10})

	setWindowUsage(t, app, grant, 91, 0)
	evaluateUserMonitorAt(t, service, monitor, now)
	first := requireFiringUserAlert(t, service, monitor, user)
	if got := first.GetString("user_email_snapshot"); got != "low@example.com" {
		t.Fatalf("email snapshot = %q", got)
	}
	if got := notificationInt64(notificationValue(first.Get("firing_value"))["remaining_bytes"]); got != 9 {
		t.Fatalf("firing remaining_bytes = %d, want 9", got)
	}

	evaluateUserMonitorAt(t, service, monitor, now.Add(time.Minute))
	assertAlertCount(t, app, monitor, 1)

	setWindowUsage(t, app, grant, 91, 100)
	evaluateUserMonitorAt(t, service, monitor, now.Add(2*time.Minute))
	assertAlertStatus(t, app, first.Id, "resolved", "condition_cleared")

	setWindowUsage(t, app, grant, 195, 100)
	evaluateUserMonitorAt(t, service, monitor, now.Add(3*time.Minute))
	second := requireFiringUserAlert(t, service, monitor, user)
	if second.Id == first.Id {
		t.Fatal("dropping below the threshold again must open a new Alert")
	}
	assertAlertCount(t, app, monitor, 2)

	evaluateUserMonitorAt(t, service, monitor, now.Add(30*24*time.Hour))
	assertAlertStatus(t, app, second.Id, "resolved", "condition_cleared")
}

func TestLowAllowanceAlertIsPerUserAndCancelledForDisabledUser(t *testing.T) {
	app := newMonitoringTestApp(t)
	service := New(app, nil, nil, "https://panel.example")
	now := time.Now().UTC().Truncate(time.Millisecond)
	typ := newMonitoringType(t, app, 100)
	monitor := newUserMonitor(t, app, KindLowAllowance, map[string]any{"threshold_percent": 10})
	alice := newMonitoringUser(t, app, "alice@example.com")
	bob := newMonitoringUser(t, app, "bob@example.com")
	setWindowUsage(t, app, newMonitoringGrant(t, app, alice, typ, now.Add(-time.Hour)), 95, 0)
	setWindowUsage(t, app, newMonitoringGrant(t, app, bob, typ, now.Add(-time.Hour)), 99, 0)

	evaluateUserMonitorAt(t, service, monitor, now)
	assertAlertCount(t, app, monitor, 2)

	alert := requireFiringUserAlert(t, service, monitor, alice)
	alice.Set("status", "disabled")
	if err := app.Save(alice); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	evaluateUserMonitorAt(t, service, monitor, now.Add(time.Minute))
	assertAlertStatus(t, app, alert.Id, "cancelled", "user_unavailable")
	requireFiringUserAlert(t, service, monitor, bob)
}

func TestExpiringSubscriptionAlertLifecycle(t *testing.T) {
	app := newMonitoringTestApp(t)
	service := New(app, nil, nil, "https://panel.example")
	now := time.Now().UTC().Truncate(time.Millisecond)
	user := newMonitoringUser(t, app, "expiring@example.com")
	typ := newMonitoringType(t, app, 100)
	current := newMonitoringGrant(t, app, user, typ, now.Add(-357*24*time.Hour))
	end := current.GetDateTime("ends_at").Time()
	monitor := newUserMonitor(t, app, KindExpiringSubscription, map[string]any{"threshold_days": 7})

	evaluateUserMonitorAt(t, service, monitor, now)
	first := requireFiringUserAlert(t, service, monitor, user)

	queued := newMonitoringGrant(t, app, user, typ, end)
	evaluateUserMonitorAt(t, service, monitor, now.Add(time.Minute))
	resolved := assertAlertStatus(t, app, first.Id, "resolved", "condition_cleared")
	if got := notificationValue(resolved.Get("recovery_value"))["next_subscription_starts_at"]; got != dateString(end) {
		t.Fatalf("recovery next_subscription_starts_at = %v, want %s", got, dateString(end))
	}

	queued.Set("terminated_at", now.Add(2*time.Minute))
	if err := app.Save(queued); err != nil {
		t.Fatalf("cancel queued grant: %v", err)
	}
	evaluateUserMonitorAt(t, service, monitor, now.Add(2*time.Minute))
	second := requireFiringUserAlert(t, service, monitor, user)

	evaluateUserMonitorAt(t, service, monitor, end)
	assertAlertStatus(t, app, second.Id, "cancelled", "subscription_ended")
}

func TestDeletingUserCancelsAlertAndKeepsEmailSnapshot(t *testing.T) {
	app := newMonitoringTestApp(t)
	service := New(app, nil, nil, "https://panel.example")
	service.BindHooks()
	now := time.Now().UTC().Truncate(time.Millisecond)
	user := newMonitoringUser(t, app, "gone@example.com")
	setWindowUsage(t, app, newMonitoringGrant(t, app, user, newMonitoringType(t, app, 100), now.Add(-time.Hour)), 100, 0)
	monitor := newUserMonitor(t, app, KindLowAllowance, map[string]any{"threshold_percent": 10})
	evaluateUserMonitorAt(t, service, monitor, now)
	alert := requireFiringUserAlert(t, service, monitor, user)

	if err := app.Delete(user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	stored := assertAlertStatus(t, app, alert.Id, "cancelled", "user_deleted")
	if stored.GetString("user") != "" || stored.GetString("user_email_snapshot") != "gone@example.com" {
		t.Fatalf("deleted user alert user=%q email=%q", stored.GetString("user"), stored.GetString("user_email_snapshot"))
	}
}

func TestUserAlertNotificationMessages(t *testing.T) {
	service := &Service{frontendURL: "https://panel.example"}
	at := time.Date(2026, 7, 12, 10, 2, 3, 0, time.UTC)
	low, _ := notificationMessageRecords("en", KindLowAllowance, "warning")
	low.Set("user", "user-1")
	low.Set("user_email_snapshot", "low@example.com")
	low.Set("firing_value", map[string]any{"used_tx_bytes": 80 << 30, "used_rx_bytes": 12 << 30, "allowance_bytes": 100 << 30, "remaining_bytes": 8 << 30, "threshold_percent": 10, "window_ends_at": "2026-07-30T00:00:00Z"})
	assertMessage(t, service.message(low, nil, "firing", at), []string{
		"[FIRING] Low allowance alert",
		"Severity: Warning",
		"Monitor: 核心流量",
		"User: low@example.com",
		"Used this window: 92.0 GiB",
		"Window allowance: 100.0 GiB",
		"Remaining: 8.0 GiB (8%)",
		"Threshold: 10%",
		"Window ends: 2026-07-30T00:00:00Z",
		"Started: 2026-07-12T10:00:00Z",
		"Duration: 2m 3s",
		"View user: https://panel.example/users/user-1",
	})

	expiring, _ := notificationMessageRecords("zh-cn", KindExpiringSubscription, "critical")
	expiring.Set("user_email_snapshot", "gone@example.com")
	expiring.Set("recovery_value", map[string]any{"subscription_ends_at": "2026-07-15T00:00:00Z", "remaining_seconds": 3 * 86400, "threshold_days": 7, "next_subscription_starts_at": "2026-07-15T00:00:00Z"})
	assertMessage(t, service.message(expiring, nil, "resolved", at), []string{
		"[告警恢复] 订阅即将到期",
		"严重程度：严重",
		"监控项：核心流量",
		"用户：gone@example.com",
		"订阅结束：2026-07-15T00:00:00Z",
		"剩余时间：3 天",
		"阈值：7 天",
		"下一份订阅开始：2026-07-15T00:00:00Z",
		"开始时间：2026-07-12T10:00:00Z",
		"持续时间：2 分 3 秒",
	})
}

func assertMessage(t *testing.T, got string, want []string) {
	t.Helper()
	if expected := strings.Join(want, "\n"); got != expected {
		t.Fatalf("message() =\n%s\n\nwant:\n%s", got, expected)
	}
}

func evaluateUserMonitorAt(t *testing.T, service *Service, monitor *core.Record, at time.Time) {
	t.Helper()
	if err := service.evaluateUserMonitor(monitor, at); err != nil {
		t.Fatalf("evaluateUserMonitor() error = %v", err)
	}
}

func requireFiringUserAlert(t *testing.T, service *Service, monitor, user *core.Record) *core.Record {
	t.Helper()
	alert, _ := service.findFiring(monitor.Id, alertSubject{user: user})
	if alert == nil {
		t.Fatalf("no firing alert for user %s", user.GetString("email"))
	}
	return alert
}

func assertAlertCount(t *testing.T, app core.App, monitor *core.Record, want int) {
	t.Helper()
	alerts, err := app.FindRecordsByFilter("alerts", "monitor = {:m}", "", 0, 0, map[string]any{"m": monitor.Id})
	if err != nil || len(alerts) != want {
		t.Fatalf("alert count = %d (err %v), want %d", len(alerts), err, want)
	}
}

func assertAlertStatus(t *testing.T, app core.App, alertID, status, reason string) *core.Record {
	t.Helper()
	alert, err := app.FindRecordById("alerts", alertID)
	if err != nil {
		t.Fatalf("find alert: %v", err)
	}
	if alert.GetString("status") != status || alert.GetString("resolution_reason") != reason {
		t.Fatalf("alert status=%q reason=%q, want %q %q", alert.GetString("status"), alert.GetString("resolution_reason"), status, reason)
	}
	return alert
}

func newMonitoringUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	user.Set("subscription_required", true)
	user.SetVerified(true)
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

func newMonitoringType(t *testing.T, app core.App, allowance int64) *core.Record {
	t.Helper()
	types, err := app.FindCollectionByNameOrId("subscription_types")
	if err != nil {
		t.Fatalf("find subscription types: %v", err)
	}
	typ := core.NewRecord(types)
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", allowance)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatalf("save subscription type: %v", err)
	}
	return typ
}

func newMonitoringGrant(t *testing.T, app core.App, user, typ *core.Record, start time.Time) *core.Record {
	t.Helper()
	grants, err := app.FindCollectionByNameOrId("user_subscriptions")
	if err != nil {
		t.Fatalf("find user subscriptions: %v", err)
	}
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	return grant
}

// setWindowUsage writes the current window's used tx and top-up. Window 0 is
// enough because every test grant starts less than 30 days before evaluation.
func setWindowUsage(t *testing.T, app core.App, grant *core.Record, usedTx, extra int64) {
	t.Helper()
	grant.Set("window_index", 0)
	grant.Set("used_tx_bytes", usedTx)
	grant.Set("used_rx_bytes", 0)
	grant.Set("extra_bytes", extra)
	if err := app.Save(grant); err != nil {
		t.Fatalf("save window usage: %v", err)
	}
}

func newUserMonitor(t *testing.T, app core.App, kind string, config map[string]any) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("monitors")
	if err != nil {
		t.Fatalf("find monitors: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", kind)
	record.Set("name_key", kind)
	record.Set("kind", kind)
	record.Set("enabled", true)
	record.Set("severity", "warning")
	record.Set("notification_language", "en")
	record.Set("channels", []string{})
	record.Set("config", config)
	if err := app.Save(record); err != nil {
		t.Fatalf("save monitor: %v", err)
	}
	return record
}
