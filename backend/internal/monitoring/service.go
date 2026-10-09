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
	evaluationInterval     = 5 * time.Second
	userEvaluationInterval = time.Minute
	cleanupInterval        = time.Hour
	observationRetention   = 25 * time.Hour
	alertRetention         = 30 * 24 * time.Hour
	deliveryConcurrency    = 3
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
		userTicker := time.NewTicker(userEvaluationInterval)
		cleanupTicker := time.NewTicker(cleanupInterval)
		defer evalTicker.Stop()
		defer userTicker.Stop()
		defer cleanupTicker.Stop()
		s.EvaluateNow(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-evalTicker.C:
				s.evaluate(ctx, func(kind string) bool { return !IsUserKind(kind) }, false)
			case <-userTicker.C:
				s.evaluate(ctx, IsUserKind, true)
			case <-cleanupTicker.C:
				if err := s.cleanup(time.Now().UTC()); err != nil {
					log.Printf("[monitoring] cleanup: %v", err)
				}
			}
		}
	}()
}

// EvaluateNow evaluates every Monitor after a configuration change. It waits
// for a running evaluation so the change is never skipped.
func (s *Service) EvaluateNow(ctx context.Context) {
	s.evaluate(ctx, func(string) bool { return true }, true)
}

// evaluate runs the Monitors whose kind matches include. Node Monitors run
// often enough to skip an overlapping run; User Monitors run once a minute
// and wait instead. Persisted firing alerts make restarts idempotent: only a
// genuine state transition schedules a notification.
func (s *Service) evaluate(ctx context.Context, include func(kind string) bool, wait bool) {
	if wait {
		s.evalMu.Lock()
	} else if !s.evalMu.TryLock() {
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
		if include(monitor.GetString("kind")) {
			s.evaluateMonitor(monitor, time.Now().UTC())
		}
	}
}

func (s *Service) evaluateMonitor(monitor *core.Record, now time.Time) {
	evaluate := s.evaluateNodeMonitor
	if IsUserKind(monitor.GetString("kind")) {
		evaluate = s.evaluateUserMonitor
	}
	if err := evaluate(monitor, now); err != nil {
		log.Printf("[monitoring] monitor %s: %v", monitor.Id, err)
	}
}

func (s *Service) evaluateNodeMonitor(monitor *core.Record, now time.Time) error {
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
	firing, err := s.app.FindRecordsByFilter("alerts", "monitor = {:m} && node != '' && status = 'firing'", "", 0, 0, map[string]any{"m": monitor.Id})
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
	subject := alertSubject{node: node}
	firing, err := s.findFiring(monitor.Id, subject)
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

	return s.applyDecision(decision, monitor, subject, firing, value, now)
}

func (s *Service) applyDecision(decision Decision, monitor *core.Record, subject alertSubject, firing *core.Record, value map[string]any, now time.Time) error {
	switch decision {
	case DecisionFire:
		return s.openAlert(monitor, subject, value, now)
	case DecisionResolve:
		return s.resolveAlert(firing, subject.node, value, now)
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
	return configInt(monitor, "threshold_bytes_per_second")
}

// configInt reads one positive integer from a Monitor's typed config.
func configInt(monitor *core.Record, key string) (int64, error) {
	var config map[string]json.Number
	raw, err := json.Marshal(monitor.Get("config"))
	if err != nil {
		return 0, err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return 0, fmt.Errorf("invalid %s config: %w", monitor.GetString("kind"), err)
	}
	value, err := config[key].Int64()
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid %s config: %s must be a positive integer", monitor.GetString("kind"), key)
	}
	return value, nil
}

// alertSubject is the Node or the User an Alert is about. Exactly one is set.
type alertSubject struct {
	node *core.Record
	user *core.Record
}

func (s alertSubject) reference() (field, id string) {
	if s.user != nil {
		return "user", s.user.Id
	}
	return "node", s.node.Id
}

func (s *Service) findFiring(monitorID string, subject alertSubject) (*core.Record, error) {
	field, id := subject.reference()
	record, err := s.app.FindFirstRecordByFilter("alerts", "monitor = {:m} && "+field+" = {:id} && status = 'firing'", map[string]any{"m": monitorID, "id": id})
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

func (s *Service) openAlert(monitor *core.Record, subject alertSubject, value map[string]any, now time.Time) error {
	collection, err := s.app.FindCollectionByNameOrId("alerts")
	if err != nil {
		return err
	}
	alert := core.NewRecord(collection)
	alert.Set("monitor", monitor.Id)
	field, id := subject.reference()
	alert.Set(field, id)
	if subject.user != nil {
		alert.Set("user_email_snapshot", subject.user.GetString("email"))
	}
	alert.Set("status", "firing")
	alert.Set("severity_snapshot", monitor.GetString("severity"))
	alert.Set("notification_language_snapshot", monitor.GetString("notification_language"))
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
	s.scheduleDeliveries(alert, subject.node, "firing", now)
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
	return cancelAlertIn(s.app, alert, reason, now)
}

// cancelAlertIn takes the app explicitly so hooks can cancel inside their transaction.
func cancelAlertIn(app core.App, alert *core.Record, reason string, now time.Time) error {
	alert.Set("status", "cancelled")
	alert.Set("ended_at", now)
	alert.Set("last_evaluated_at", now)
	alert.Set("resolution_reason", reason)
	return app.Save(alert)
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
