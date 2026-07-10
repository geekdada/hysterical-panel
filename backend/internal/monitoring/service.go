package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/notifications"
)

const (
	evaluationInterval   = 5 * time.Second
	cleanupInterval      = time.Hour
	observationRetention = 25 * time.Hour
	alertRetention       = 30 * 24 * time.Hour
	deliveryConcurrency  = 3
)

type Delivery interface {
	Send(rawURL, message string) notifications.Result
}

type Service struct {
	app         core.App
	box         *cryptobox.Box
	delivery    Delivery
	frontendURL string
	evalMu      sync.Mutex
	deliverySem chan struct{}
}

func New(app core.App, box *cryptobox.Box, delivery Delivery, frontendURL string) *Service {
	return &Service{app: app, box: box, delivery: delivery, frontendURL: strings.TrimRight(frontendURL, "/"), deliverySem: make(chan struct{}, deliveryConcurrency)}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		evalTicker := time.NewTicker(evaluationInterval)
		cleanupTicker := time.NewTicker(cleanupInterval)
		defer evalTicker.Stop()
		defer cleanupTicker.Stop()
		s.EvaluateNow(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-evalTicker.C:
				s.EvaluateNow(ctx)
			case <-cleanupTicker.C:
				if err := s.cleanup(time.Now().UTC()); err != nil {
					log.Printf("[monitoring] cleanup: %v", err)
				}
			}
		}
	}()
}

// EvaluateNow skips overlapping runs. Persisted firing alerts make restarts
// idempotent: only a genuine state transition schedules a notification.
func (s *Service) EvaluateNow(ctx context.Context) {
	if !s.evalMu.TryLock() {
		return
	}
	defer s.evalMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	monitors, err := s.app.FindRecordsByFilter("monitors", "deleted_at = '' && enabled = true", "created", 0, 0)
	if err != nil {
		log.Printf("[monitoring] list monitors: %v", err)
		return
	}
	for _, monitor := range monitors {
		if err := s.evaluateMonitor(monitor, time.Now().UTC()); err != nil {
			log.Printf("[monitoring] monitor %s: %v", monitor.Id, err)
		}
	}
}

func (s *Service) evaluateMonitor(monitor *core.Record, now time.Time) error {
	nodes, err := s.applicableNodes(monitor)
	if err != nil {
		return err
	}
	applicable := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		applicable[node.Id] = struct{}{}
		if err := s.evaluateNode(monitor, node, now); err != nil {
			log.Printf("[monitoring] monitor=%s node=%s: %v", monitor.Id, node.Id, err)
		}
	}
	firing, err := s.app.FindRecordsByFilter("alerts", "monitor = {:m} && status = 'firing'", "", 0, 0, map[string]any{"m": monitor.Id})
	if err != nil {
		return err
	}
	for _, alert := range firing {
		if _, ok := applicable[alert.GetString("node")]; !ok {
			if err := s.cancelAlert(alert, "node_removed_from_scope", now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) applicableNodes(monitor *core.Record) ([]*core.Record, error) {
	if monitor.GetString("node_scope") == "all_enabled" {
		return s.app.FindRecordsByFilter("nodes", "deleted_at = '' && enabled = true", "name", 0, 0)
	}
	ids := monitor.GetStringSlice("nodes")
	result := make([]*core.Record, 0, len(ids))
	for _, id := range ids {
		node, err := s.app.FindRecordById("nodes", id)
		if err == nil && node.GetBool("enabled") && node.GetDateTime("deleted_at").IsZero() {
			result = append(result, node)
		}
	}
	return result, nil
}

func (s *Service) evaluateNode(monitor, node *core.Record, now time.Time) error {
	firing, err := s.findFiring(monitor.Id, node.Id)
	if err != nil {
		return err
	}
	window := time.Duration(monitor.GetInt("evaluation_window_seconds")) * time.Second
	cutoff := now.Add(-window)
	decision := DecisionKeep
	value := map[string]any{}

	switch monitor.GetString("kind") {
	case "offline":
		baseline := node.GetDateTime("last_polled_at").Time()
		if baseline.IsZero() {
			baseline = node.GetDateTime("enabled_at").Time()
		}
		if baseline.IsZero() {
			baseline = node.GetDateTime("created").Time()
		}
		age := now.Sub(baseline)
		value = map[string]any{"last_successful_poll_at": dateString(baseline), "stale_seconds": int64(age.Seconds())}
		if baseline.IsZero() || age >= window {
			if firing == nil {
				decision = DecisionFire
			}
		} else if firing != nil {
			decision = DecisionResolve
		}
	case "high_traffic":
		points, err := s.observations(node.Id, cutoff)
		if err != nil {
			return err
		}
		average, known := WeightedAverage(points)
		if !known {
			return s.touchAlert(firing, now)
		}
		threshold, err := monitorThreshold(monitor)
		if err != nil {
			return err
		}
		value = map[string]any{"average_bytes_per_second": average, "threshold_bytes_per_second": threshold}
		decision = DecideHighTraffic(firing != nil, average, threshold)
	default:
		return fmt.Errorf("unsupported monitor kind %q", monitor.GetString("kind"))
	}

	switch decision {
	case DecisionFire:
		return s.openAlert(monitor, node, value, now)
	case DecisionResolve:
		return s.resolveAlert(firing, node, value, now)
	default:
		return s.touchAlert(firing, now)
	}
}

func (s *Service) observations(nodeID string, cutoff time.Time) ([]Observation, error) {
	records, err := s.app.FindRecordsByFilter("monitor_observations", "node = {:n} && observed_at >= {:c}", "observed_at", 0, 0, map[string]any{"n": nodeID, "c": cutoff})
	if err != nil {
		return nil, err
	}
	points := make([]Observation, 0, len(records))
	for _, record := range records {
		points = append(points, Observation{TxBytes: int64(record.GetInt("tx_bytes")), RxBytes: int64(record.GetInt("rx_bytes")), ElapsedSeconds: int64(record.GetInt("elapsed_seconds"))})
	}
	return points, nil
}

func monitorThreshold(monitor *core.Record) (int64, error) {
	var config struct {
		Threshold int64 `json:"threshold_bytes_per_second"`
	}
	raw, err := json.Marshal(monitor.Get("config"))
	if err != nil {
		return 0, err
	}
	if err := json.Unmarshal(raw, &config); err != nil || config.Threshold <= 0 {
		return 0, fmt.Errorf("invalid high traffic config")
	}
	return config.Threshold, nil
}

func (s *Service) findFiring(monitorID, nodeID string) (*core.Record, error) {
	record, err := s.app.FindFirstRecordByFilter("alerts", "monitor = {:m} && node = {:n} && status = 'firing'", map[string]any{"m": monitorID, "n": nodeID})
	if err != nil {
		return nil, nil
	}
	return record, nil
}

func (s *Service) enabledChannelIDs(monitor *core.Record) []string {
	ids := monitor.GetStringSlice("channels")
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		channel, err := s.app.FindRecordById("notification_channels", id)
		if err == nil && channel.GetBool("enabled") {
			result = append(result, id)
		}
	}
	return result
}

func (s *Service) openAlert(monitor, node *core.Record, value map[string]any, now time.Time) error {
	collection, err := s.app.FindCollectionByNameOrId("alerts")
	if err != nil {
		return err
	}
	alert := core.NewRecord(collection)
	alert.Set("monitor", monitor.Id)
	alert.Set("node", node.Id)
	alert.Set("status", "firing")
	alert.Set("severity_snapshot", monitor.GetString("severity"))
	alert.Set("monitor_name_snapshot", monitor.GetString("name"))
	alert.Set("monitor_kind_snapshot", monitor.GetString("kind"))
	alert.Set("monitor_config_snapshot", monitor.Get("config"))
	alert.Set("evaluation_window_seconds_snapshot", monitor.GetInt("evaluation_window_seconds"))
	alert.Set("channel_ids_snapshot", s.enabledChannelIDs(monitor))
	alert.Set("firing_value", value)
	alert.Set("started_at", now)
	alert.Set("last_evaluated_at", now)
	if err := s.app.Save(alert); err != nil {
		return err
	}
	s.scheduleDeliveries(alert, node, "firing", now)
	return nil
}

func (s *Service) resolveAlert(alert, node *core.Record, value map[string]any, now time.Time) error {
	if alert == nil {
		return nil
	}
	alert.Set("status", "resolved")
	alert.Set("recovery_value", value)
	alert.Set("ended_at", now)
	alert.Set("last_evaluated_at", now)
	alert.Set("resolution_reason", "condition_cleared")
	if err := s.app.Save(alert); err != nil {
		return err
	}
	s.scheduleDeliveries(alert, node, "resolved", now)
	return nil
}

func (s *Service) touchAlert(alert *core.Record, now time.Time) error {
	if alert == nil {
		return nil
	}
	alert.Set("last_evaluated_at", now)
	return s.app.Save(alert)
}

func (s *Service) cancelAlert(alert *core.Record, reason string, now time.Time) error {
	alert.Set("status", "cancelled")
	alert.Set("ended_at", now)
	alert.Set("last_evaluated_at", now)
	alert.Set("resolution_reason", reason)
	return s.app.Save(alert)
}

func (s *Service) CancelMonitorAlerts(monitorID, reason string) error {
	alerts, err := s.app.FindRecordsByFilter("alerts", "monitor = {:m} && status = 'firing'", "", 0, 0, map[string]any{"m": monitorID})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, alert := range alerts {
		if err := s.cancelAlert(alert, reason, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CancelNodeAlerts(nodeID, reason string) error {
	alerts, err := s.app.FindRecordsByFilter("alerts", "node = {:n} && status = 'firing'", "", 0, 0, map[string]any{"n": nodeID})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, alert := range alerts {
		if err := s.cancelAlert(alert, reason, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) scheduleDeliveries(alert, node *core.Record, transition string, now time.Time) {
	for _, channelID := range alert.GetStringSlice("channel_ids_snapshot") {
		channelID := channelID
		go func() {
			s.deliverySem <- struct{}{}
			defer func() { <-s.deliverySem }()
			s.deliver(alert, node, channelID, transition, now)
		}()
	}
}

func (s *Service) deliver(alert, node *core.Record, channelID, transition string, now time.Time) {
	status, safeError := "skipped", ""
	channel, err := s.app.FindRecordById("notification_channels", channelID)
	if err != nil {
		safeError = "channel_deleted"
	} else if !channel.GetBool("enabled") {
		safeError = "channel_disabled"
	} else {
		rawURL, decryptErr := s.box.Decrypt(channel.GetString("url_encrypted"))
		if decryptErr != nil {
			status, safeError = "failed", "delivery_failed"
		} else {
			result := s.delivery.Send(rawURL, s.message(alert, node, transition, now))
			if result.Succeeded {
				status = "succeeded"
			} else {
				status, safeError = "failed", string(result.ErrorCode)
			}
		}
	}
	collection, err := s.app.FindCollectionByNameOrId("alert_deliveries")
	if err != nil {
		log.Printf("[monitoring] delivery record: %v", err)
		return
	}
	record := core.NewRecord(collection)
	record.Set("alert", alert.Id)
	if channel != nil {
		record.Set("channel", channel.Id)
	}
	record.Set("transition", transition)
	record.Set("status", status)
	record.Set("safe_error", safeError)
	record.Set("attempted_at", time.Now().UTC())
	if err := s.app.Save(record); err != nil {
		log.Printf("[monitoring] save delivery: %v", err)
	}
}

func (s *Service) message(alert, node *core.Record, transition string, now time.Time) string {
	state := strings.ToUpper(transition)
	link := s.frontendURL + "/nodes/" + node.Id
	valueField := "firing_value"
	if transition == "resolved" {
		valueField = "recovery_value"
	}
	value, _ := json.Marshal(alert.Get(valueField))
	return fmt.Sprintf("[%s] %s alert\nSeverity: %s\nMonitor: %s\nNode: %s\nKind: %s\nValue: %s\nWindow: %ds\nStarted: %s\nDuration: %s\n%s", state, alert.GetString("monitor_kind_snapshot"), alert.GetString("severity_snapshot"), alert.GetString("monitor_name_snapshot"), node.GetString("name"), alert.GetString("monitor_kind_snapshot"), value, alert.GetInt("evaluation_window_seconds_snapshot"), alert.GetString("started_at"), now.Sub(alert.GetDateTime("started_at").Time()).Round(time.Second), link)
}

func (s *Service) cleanup(now time.Time) error {
	observationCutoff := now.Add(-observationRetention).Format("2006-01-02 15:04:05.000Z")
	alertCutoff := now.Add(-alertRetention).Format("2006-01-02 15:04:05.000Z")
	_, err := s.app.DB().NewQuery("DELETE FROM monitor_observations WHERE observed_at < {:cutoff}").Bind(map[string]any{"cutoff": observationCutoff}).Execute()
	if err != nil {
		return err
	}
	_, err = s.app.DB().NewQuery("DELETE FROM alert_deliveries WHERE alert IN (SELECT id FROM alerts WHERE status != 'firing' AND ended_at < {:cutoff})").Bind(map[string]any{"cutoff": alertCutoff}).Execute()
	if err != nil {
		return err
	}
	_, err = s.app.DB().NewQuery("DELETE FROM alerts WHERE status != 'firing' AND ended_at < {:cutoff}").Bind(map[string]any{"cutoff": alertCutoff}).Execute()
	return err
}

func dateString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
