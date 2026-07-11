package monitoring

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type notificationCopy struct {
	transitions  map[string]string
	kinds        map[string]string
	severities   map[string]string
	alertSuffix  string
	separator    string
	severity     string
	monitor      string
	node         string
	averageRate  string
	threshold    string
	lastPoll     string
	timeSince    string
	window       string
	started      string
	duration     string
	viewNode     string
	durationCopy durationCopy
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
	transitions: map[string]string{"firing": "FIRING", "resolved": "RESOLVED"},
	kinds:       map[string]string{"offline": "Offline", "high_traffic": "High traffic"},
	severities:  map[string]string{"warning": "Warning", "critical": "Critical"},
	alertSuffix: " alert",
	separator:   ": ",
	severity:    "Severity",
	monitor:     "Monitor",
	node:        "Node",
	averageRate: "Average rate",
	threshold:   "Threshold",
	lastPoll:    "Last successful poll",
	timeSince:   "Time since last success",
	window:      "Evaluation window",
	started:     "Started",
	duration:    "Duration",
	viewNode:    "View node",
	durationCopy: durationCopy{
		seconds:    "s",
		minutes:    "m",
		minutePart: "m",
		hours:      "h",
		days:       "d",
	},
}

var chineseNotificationCopy = notificationCopy{
	transitions: map[string]string{"firing": "告警触发", "resolved": "告警恢复"},
	kinds:       map[string]string{"offline": "节点离线", "high_traffic": "高流量"},
	severities:  map[string]string{"warning": "警告", "critical": "严重"},
	separator:   "：",
	severity:    "严重程度",
	monitor:     "监控项",
	node:        "节点",
	averageRate: "平均速率",
	threshold:   "阈值",
	lastPoll:    "最后成功采集",
	timeSince:   "距上次成功",
	window:      "评估窗口",
	started:     "开始时间",
	duration:    "持续时间",
	viewNode:    "查看节点",
	durationCopy: durationCopy{
		unitSeparator: " ",
		seconds:       "秒",
		minutes:       "分钟",
		minutePart:    "分",
		hours:         "小时",
		days:          "天",
	},
}

func (s *Service) message(alert, node *core.Record, transition string, now time.Time) string {
	copy := englishNotificationCopy
	if alert.GetString("notification_language_snapshot") == "zh-cn" {
		copy = chineseNotificationCopy
	}
	kind := alert.GetString("monitor_kind_snapshot")
	severity := alert.GetString("severity_snapshot")
	valueField := "firing_value"
	if transition == "resolved" {
		valueField = "recovery_value"
	}
	value := notificationValue(alert.Get(valueField))
	duration := max(now.Sub(alert.GetDateTime("started_at").Time()), 0).Round(time.Second)
	window := time.Duration(alert.GetInt("evaluation_window_seconds_snapshot")) * time.Second
	link := s.frontendURL + "/nodes/" + node.Id

	lines := []string{
		fmt.Sprintf("[%s] %s%s", mapValue(copy.transitions, transition, "FIRING"), mapValue(copy.kinds, kind, kind), copy.alertSuffix),
		copy.field(copy.severity, mapValue(copy.severities, severity, severity)),
		copy.field(copy.monitor, alert.GetString("monitor_name_snapshot")),
		copy.field(copy.node, node.GetString("name")),
	}
	if kind == "high_traffic" {
		lines = append(lines,
			copy.field(copy.averageRate, formatNotificationRate(notificationInt64(value["average_bytes_per_second"]))),
			copy.field(copy.threshold, formatNotificationRate(notificationInt64(value["threshold_bytes_per_second"]))),
		)
	} else {
		lines = append(lines,
			copy.field(copy.lastPoll, fmt.Sprint(value["last_successful_poll_at"])),
			copy.field(copy.timeSince, formatNotificationDuration(time.Duration(notificationInt64(value["stale_seconds"]))*time.Second, copy.durationCopy)),
		)
	}
	lines = append(lines,
		copy.field(copy.window, formatNotificationDuration(window, copy.durationCopy)),
		copy.field(copy.started, alert.GetDateTime("started_at").Time().UTC().Format(time.RFC3339)),
		copy.field(copy.duration, formatNotificationDuration(duration, copy.durationCopy)),
		copy.field(copy.viewNode, link),
	)
	return strings.Join(lines, "\n")
}

func (copy notificationCopy) field(label, value string) string {
	return label + copy.separator + value
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
