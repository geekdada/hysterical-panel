package monitoring

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type notificationCopy struct {
	transitions      map[string]string
	kinds            map[string]string
	severities       map[string]string
	alertSuffix      string
	separator        string
	severity         string
	monitor          string
	node             string
	user             string
	averageRate      string
	threshold        string
	lastPoll         string
	timeSince        string
	used             string
	allowance        string
	remaining        string
	windowEnds       string
	subscriptionEnds string
	timeLeft         string
	nextSubscription string
	window           string
	started          string
	duration         string
	viewNode         string
	viewUser         string
	durationCopy     durationCopy
}

type durationCopy struct {
	unitSeparator string
	seconds       string
	minutes       string
	minutePart    string
	hours         string
	days          string
}

var englishNotificationCopy = notificationCopy{
	transitions:      map[string]string{"firing": "FIRING", "resolved": "RESOLVED"},
	kinds:            map[string]string{"offline": "Offline", "high_traffic": "High traffic", KindLowAllowance: "Low allowance", KindExpiringSubscription: "Subscription expiring"},
	severities:       map[string]string{"warning": "Warning", "critical": "Critical"},
	alertSuffix:      " alert",
	separator:        ": ",
	severity:         "Severity",
	monitor:          "Monitor",
	node:             "Node",
	user:             "User",
	averageRate:      "Average rate",
	threshold:        "Threshold",
	lastPoll:         "Last successful poll",
	timeSince:        "Time since last success",
	used:             "Used this window",
	allowance:        "Window allowance",
	remaining:        "Remaining",
	windowEnds:       "Window ends",
	subscriptionEnds: "Subscription ends",
	timeLeft:         "Time left",
	nextSubscription: "Next subscription starts",
	window:           "Evaluation window",
	started:          "Started",
	duration:         "Duration",
	viewNode:         "View node",
	viewUser:         "View user",
	durationCopy: durationCopy{
		seconds:    "s",
		minutes:    "m",
		minutePart: "m",
		hours:      "h",
		days:       "d",
	},
}

var chineseNotificationCopy = notificationCopy{
	transitions:      map[string]string{"firing": "告警触发", "resolved": "告警恢复"},
	kinds:            map[string]string{"offline": "节点离线", "high_traffic": "高流量", KindLowAllowance: "流量不足", KindExpiringSubscription: "订阅即将到期"},
	severities:       map[string]string{"warning": "警告", "critical": "严重"},
	separator:        "：",
	severity:         "严重程度",
	monitor:          "监控项",
	node:             "节点",
	user:             "用户",
	averageRate:      "平均速率",
	threshold:        "阈值",
	lastPoll:         "最后成功采集",
	timeSince:        "距上次成功",
	used:             "本周期已用",
	allowance:        "本周期额度",
	remaining:        "剩余",
	windowEnds:       "本周期结束",
	subscriptionEnds: "订阅结束",
	timeLeft:         "剩余时间",
	nextSubscription: "下一份订阅开始",
	window:           "评估窗口",
	started:          "开始时间",
	duration:         "持续时间",
	viewNode:         "查看节点",
	viewUser:         "查看用户",
	durationCopy: durationCopy{
		unitSeparator: " ",
		seconds:       "秒",
		minutes:       "分钟",
		minutePart:    "分",
		hours:         "小时",
		days:          "天",
	},
}

// notificationContent is one Notification before it is rendered for a
// destination. Channels get plain text; a later User email can reuse the
// same title and fields in its own template.
type notificationContent struct {
	title  string
	fields []notificationField
}

type notificationField struct {
	label string
	value string
}

func (c *notificationContent) add(label, value string) {
	c.fields = append(c.fields, notificationField{label: label, value: value})
}

func (c notificationContent) text(separator string) string {
	lines := []string{c.title}
	for _, field := range c.fields {
		lines = append(lines, field.label+separator+field.value)
	}
	return strings.Join(lines, "\n")
}

// message renders the Channel text. node is nil for User Alerts.
func (s *Service) message(alert, node *core.Record, transition string, now time.Time) string {
	copy := englishNotificationCopy
	if alert.GetString("notification_language_snapshot") == "zh-cn" {
		copy = chineseNotificationCopy
	}
	return s.content(alert, node, transition, now, copy).text(copy.separator)
}

func (s *Service) content(alert, node *core.Record, transition string, now time.Time, copy notificationCopy) notificationContent {
	kind := alert.GetString("monitor_kind_snapshot")
	severity := alert.GetString("severity_snapshot")
	valueField := "firing_value"
	if transition == "resolved" {
		valueField = "recovery_value"
	}
	value := notificationValue(alert.Get(valueField))
	content := notificationContent{title: fmt.Sprintf("[%s] %s%s", mapValue(copy.transitions, transition, "FIRING"), mapValue(copy.kinds, kind, kind), copy.alertSuffix)}
	content.add(copy.severity, mapValue(copy.severities, severity, severity))
	content.add(copy.monitor, alert.GetString("monitor_name_snapshot"))
	if IsUserKind(kind) {
		content.add(copy.user, alert.GetString("user_email_snapshot"))
	} else {
		content.add(copy.node, node.GetString("name"))
	}
	addKindFields(&content, kind, value, copy)
	if !IsUserKind(kind) {
		window := time.Duration(alert.GetInt("evaluation_window_seconds_snapshot")) * time.Second
		content.add(copy.window, formatNotificationDuration(window, copy.durationCopy))
	}
	duration := max(now.Sub(alert.GetDateTime("started_at").Time()), 0).Round(time.Second)
	content.add(copy.started, alert.GetDateTime("started_at").Time().UTC().Format(time.RFC3339))
	content.add(copy.duration, formatNotificationDuration(duration, copy.durationCopy))
	s.addLink(&content, alert, node, kind, copy)
	return content
}

func (s *Service) addLink(content *notificationContent, alert, node *core.Record, kind string, copy notificationCopy) {
	if !IsUserKind(kind) {
		content.add(copy.viewNode, s.frontendURL+"/nodes/"+node.Id)
		return
	}
	// A deleted User has no page to link to.
	if userID := alert.GetString("user"); userID != "" {
		content.add(copy.viewUser, s.frontendURL+"/users/"+userID)
	}
}

func addKindFields(content *notificationContent, kind string, value map[string]any, copy notificationCopy) {
	switch kind {
	case "high_traffic":
		content.add(copy.averageRate, formatNotificationRate(notificationInt64(value["average_bytes_per_second"])))
		content.add(copy.threshold, formatNotificationRate(notificationInt64(value["threshold_bytes_per_second"])))
	case KindLowAllowance:
		addLowAllowanceFields(content, value, copy)
	case KindExpiringSubscription:
		addExpiringFields(content, value, copy)
	default:
		content.add(copy.lastPoll, fmt.Sprint(value["last_successful_poll_at"]))
		content.add(copy.timeSince, formatNotificationDuration(time.Duration(notificationInt64(value["stale_seconds"]))*time.Second, copy.durationCopy))
	}
}

func addLowAllowanceFields(content *notificationContent, value map[string]any, copy notificationCopy) {
	allowance := notificationInt64(value["allowance_bytes"])
	used := notificationInt64(value["used_tx_bytes"]) + notificationInt64(value["used_rx_bytes"])
	// An over-allowance window shows zero remaining, as on the User page.
	remaining := max(notificationInt64(value["remaining_bytes"]), 0)
	content.add(copy.used, formatNotificationBytes(used))
	content.add(copy.allowance, formatNotificationBytes(allowance))
	content.add(copy.remaining, fmt.Sprintf("%s (%s)", formatNotificationBytes(remaining), formatNotificationPercent(remaining, allowance)))
	content.add(copy.threshold, fmt.Sprintf("%d%%", notificationInt64(value["threshold_percent"])))
	content.add(copy.windowEnds, fmt.Sprint(value["window_ends_at"]))
}

func addExpiringFields(content *notificationContent, value map[string]any, copy notificationCopy) {
	left := time.Duration(max(notificationInt64(value["remaining_seconds"]), 0)) * time.Second
	threshold := time.Duration(notificationInt64(value["threshold_days"])) * 24 * time.Hour
	content.add(copy.subscriptionEnds, fmt.Sprint(value["subscription_ends_at"]))
	content.add(copy.timeLeft, formatNotificationDuration(left, copy.durationCopy))
	content.add(copy.threshold, formatNotificationDuration(threshold, copy.durationCopy))
	if next, ok := value["next_subscription_starts_at"].(string); ok && next != "" {
		content.add(copy.nextSubscription, next)
	}
}

func mapValue(values map[string]string, key, fallback string) string {
	if value := values[key]; value != "" {
		return value
	}
	return fallback
}

func notificationValue(value any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	result := map[string]any{}
	if json.Unmarshal(raw, &result) != nil {
		return map[string]any{}
	}
	return result
}

func notificationInt64(value any) int64 {
	switch value := value.(type) {
	case float64:
		return int64(value)
	case float32:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	case json.Number:
		result, _ := value.Int64()
		return result
	default:
		return 0
	}
}

func formatNotificationRate(bytesPerSecond int64) string {
	bytesPerSecond = max(bytesPerSecond, 0)
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	value := float64(bytesPerSecond)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytesPerSecond, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// formatNotificationBytes uses binary units like the panel's subscription pages.
func formatNotificationBytes(bytes int64) string {
	bytes = max(bytes, 0)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// formatNotificationPercent rounds down so a value just below the threshold
// never displays as the threshold itself.
func formatNotificationPercent(part, whole int64) string {
	if whole <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", int64(float64(part)/float64(whole)*100))
}

func formatNotificationDuration(value time.Duration, copy durationCopy) string {
	seconds := max(int64(value/time.Second), 0)
	if seconds < 60 {
		return copy.durationPart(seconds, copy.seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		if seconds%60 == 0 {
			return copy.durationPart(minutes, copy.minutes)
		}
		return joinDurationParts(copy.durationPart(minutes, copy.minutePart), seconds%60, copy.seconds, copy)
	}
	hours := minutes / 60
	if hours < 24 {
		return joinDurationParts(copy.durationPart(hours, copy.hours), minutes%60, copy.minutePart, copy)
	}
	days := hours / 24
	return joinDurationParts(copy.durationPart(days, copy.days), hours%24, copy.hours, copy)
}

func joinDurationParts(first string, remainder int64, unit string, copy durationCopy) string {
	if remainder == 0 {
		return first
	}
	return first + " " + copy.durationPart(remainder, unit)
}

func (copy durationCopy) durationPart(value int64, unit string) string {
	return fmt.Sprintf("%d%s%s", value, copy.unitSeparator, unit)
}
