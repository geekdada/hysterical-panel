package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type monitorLifecycle interface {
	EvaluateNow(ctx context.Context)
	CancelMonitorAlerts(monitorID, reason string) error
	CancelNodeAlerts(nodeID, reason string) error
}

type monitorInput struct {
	Name                    *string         `json:"name"`
	Kind                    *string         `json:"kind"`
	Enabled                 *bool           `json:"enabled"`
	Severity                *string         `json:"severity"`
	EvaluationWindowSeconds *int            `json:"evaluation_window_seconds"`
	NodeScope               *string         `json:"node_scope"`
	NodeIDs                 *[]string       `json:"node_ids"`
	ChannelIDs              *[]string       `json:"channel_ids"`
	Config                  *map[string]any `json:"config"`
}

func (h *Handlers) listMonitors(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("monitors", "deleted_at = ''", "-created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list monitors", err)
	}
	result := make([]Monitor, 0, len(records))
	for _, record := range records {
		result = append(result, publicMonitor(record))
	}
	return ok(e, result)
}

func (h *Handlers) getMonitor(e *core.RequestEvent) error {
	record, err := h.findMonitor(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(e, publicMonitor(record))
}

func (h *Handlers) createMonitor(e *core.RequestEvent) error {
	var input monitorInput
	if err := e.BindBody(&input); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if input.Name == nil || input.Kind == nil || input.Severity == nil || input.EvaluationWindowSeconds == nil || input.NodeScope == nil || input.Config == nil {
		if input.Name == nil || input.Kind == nil || input.EvaluationWindowSeconds == nil || input.NodeScope == nil || input.Config == nil {
			return apis.NewBadRequestError("name, kind, evaluation_window_seconds, node_scope and config are required", nil)
		}
	}
	if input.Severity == nil {
		severity := "warning"
		input.Severity = &severity
	}
	if input.NodeIDs == nil {
		empty := []string{}
		input.NodeIDs = &empty
	}
	if input.ChannelIDs == nil {
		empty := []string{}
		input.ChannelIDs = &empty
	}
	if input.Enabled == nil {
		enabled := true
		input.Enabled = &enabled
	}
	if err := h.validateMonitorInput(input, ""); err != nil {
		return err
	}
	collection, err := h.app.FindCollectionByNameOrId("monitors")
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	applyMonitorInput(record, input)
	if err := h.app.Save(record); err != nil {
		return apis.NewBadRequestError("failed to save monitor", err)
	}
	h.monitoring.EvaluateNow(e.Request.Context())
	return ok(e, publicMonitor(record))
}

func (h *Handlers) updateMonitor(e *core.RequestEvent) error {
	record, err := h.findMonitor(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var input monitorInput
	if err := e.BindBody(&input); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	merged := monitorInputFromRecord(record)
	mergeMonitorInput(&merged, input)
	if err := h.validateMonitorInput(merged, record.Id); err != nil {
		return err
	}
	reconfigured := monitorEvaluationChanged(record, merged)
	disabled := record.GetBool("enabled") && merged.Enabled != nil && !*merged.Enabled
	applyMonitorInput(record, merged)
	if err := h.app.Save(record); err != nil {
		return apis.NewBadRequestError("failed to save monitor", err)
	}
	if disabled || reconfigured {
		reason := "monitor_reconfigured"
		if disabled {
			reason = "monitor_disabled"
		}
		if err := h.monitoring.CancelMonitorAlerts(record.Id, reason); err != nil {
			return apis.NewBadRequestError("failed to cancel monitor alerts", err)
		}
	}
	h.monitoring.EvaluateNow(e.Request.Context())
	return ok(e, publicMonitor(record))
}

func (h *Handlers) deleteMonitor(e *core.RequestEvent) error {
	record, err := h.findMonitor(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	record.Set("enabled", false)
	record.Set("deleted_at", time.Now().UTC())
	if err := h.app.Save(record); err != nil {
		return apis.NewBadRequestError("failed to delete monitor", err)
	}
	if err := h.monitoring.CancelMonitorAlerts(record.Id, "monitor_deleted"); err != nil {
		return apis.NewBadRequestError("failed to cancel monitor alerts", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func (h *Handlers) findMonitor(id string) (*core.Record, error) {
	record, err := h.app.FindRecordById("monitors", id)
	if err != nil || !record.GetDateTime("deleted_at").IsZero() {
		return nil, apis.NewNotFoundError("monitor not found", err)
	}
	return record, nil
}

func (h *Handlers) validateMonitorInput(input monitorInput, exceptID string) error {
	name := strings.TrimSpace(*input.Name)
	if name == "" || len(name) > 128 {
		return apis.NewBadRequestError("monitor name must be between 1 and 128 characters", nil)
	}
	filter := "name_key = {:key} && deleted_at = ''"
	params := map[string]any{"key": strings.ToLower(name)}
	if exceptID != "" {
		filter += " && id != {:id}"
		params["id"] = exceptID
	}
	if existing, _ := h.app.FindFirstRecordByFilter("monitors", filter, params); existing != nil {
		return apis.NewBadRequestError("monitor name is already in use", nil)
	}
	if *input.Kind != "offline" && *input.Kind != "high_traffic" {
		return apis.NewBadRequestError("invalid monitor kind", nil)
	}
	if *input.Severity != "warning" && *input.Severity != "critical" {
		return apis.NewBadRequestError("invalid monitor severity", nil)
	}
	if *input.EvaluationWindowSeconds < 60 || *input.EvaluationWindowSeconds > 86400 {
		return apis.NewBadRequestError("evaluation_window_seconds must be between 60 and 86400", nil)
	}
	if *input.NodeScope != "all_enabled" && *input.NodeScope != "selected" {
		return apis.NewBadRequestError("invalid node_scope", nil)
	}
	if *input.NodeScope == "all_enabled" && len(*input.NodeIDs) != 0 {
		return apis.NewBadRequestError("all_enabled scope cannot include node_ids", nil)
	}
	if *input.NodeScope == "selected" && len(*input.NodeIDs) == 0 {
		return apis.NewBadRequestError("selected scope requires node_ids", nil)
	}
	for _, id := range *input.NodeIDs {
		node, err := h.app.FindRecordById("nodes", id)
		if err != nil || !node.GetDateTime("deleted_at").IsZero() {
			return apis.NewBadRequestError("invalid node_id", nil)
		}
	}
	for _, id := range *input.ChannelIDs {
		if _, err := h.app.FindRecordById("notification_channels", id); err != nil {
			return apis.NewBadRequestError("invalid channel_id", nil)
		}
	}
	if *input.Kind == "offline" {
		if len(*input.Config) != 0 {
			return apis.NewBadRequestError("offline config must be empty", nil)
		}
	} else {
		if len(*input.Config) != 1 {
			return apis.NewBadRequestError("high_traffic config requires threshold_bytes_per_second", nil)
		}
		threshold, ok := (*input.Config)["threshold_bytes_per_second"].(float64)
		if !ok || threshold <= 0 || threshold != float64(int64(threshold)) {
			return apis.NewBadRequestError("threshold_bytes_per_second must be a positive integer", nil)
		}
	}
	return nil
}

func applyMonitorInput(record *core.Record, input monitorInput) {
	record.Set("name", strings.TrimSpace(*input.Name))
	record.Set("name_key", strings.ToLower(strings.TrimSpace(*input.Name)))
	record.Set("kind", *input.Kind)
	record.Set("enabled", *input.Enabled)
	record.Set("severity", *input.Severity)
	record.Set("evaluation_window_seconds", *input.EvaluationWindowSeconds)
	record.Set("node_scope", *input.NodeScope)
	record.Set("nodes", *input.NodeIDs)
	record.Set("channels", *input.ChannelIDs)
	record.Set("config", *input.Config)
}

func monitorInputFromRecord(record *core.Record) monitorInput {
	name, kind, enabled := record.GetString("name"), record.GetString("kind"), record.GetBool("enabled")
	severity, window, scope := record.GetString("severity"), record.GetInt("evaluation_window_seconds"), record.GetString("node_scope")
	nodes, channels, config := record.GetStringSlice("nodes"), record.GetStringSlice("channels"), jsonMap(record.Get("config"))
	return monitorInput{Name: &name, Kind: &kind, Enabled: &enabled, Severity: &severity, EvaluationWindowSeconds: &window, NodeScope: &scope, NodeIDs: &nodes, ChannelIDs: &channels, Config: &config}
}

func mergeMonitorInput(target *monitorInput, patch monitorInput) {
	if patch.Name != nil {
		target.Name = patch.Name
	}
	if patch.Kind != nil {
		target.Kind = patch.Kind
	}
	if patch.Enabled != nil {
		target.Enabled = patch.Enabled
	}
	if patch.Severity != nil {
		target.Severity = patch.Severity
	}
	if patch.EvaluationWindowSeconds != nil {
		target.EvaluationWindowSeconds = patch.EvaluationWindowSeconds
	}
	if patch.NodeScope != nil {
		target.NodeScope = patch.NodeScope
	}
	if patch.NodeIDs != nil {
		target.NodeIDs = patch.NodeIDs
	}
	if patch.ChannelIDs != nil {
		target.ChannelIDs = patch.ChannelIDs
	}
	if patch.Config != nil {
		target.Config = patch.Config
	}
}

func monitorEvaluationChanged(record *core.Record, input monitorInput) bool {
	return record.GetString("kind") != *input.Kind || record.GetBool("enabled") != *input.Enabled || record.GetString("severity") != *input.Severity || record.GetInt("evaluation_window_seconds") != *input.EvaluationWindowSeconds || record.GetString("node_scope") != *input.NodeScope || fmt.Sprint(record.GetStringSlice("nodes")) != fmt.Sprint(*input.NodeIDs) || fmt.Sprint(jsonMap(record.Get("config"))) != fmt.Sprint(*input.Config)
}

func publicMonitor(record *core.Record) Monitor {
	return Monitor{ID: record.Id, Name: record.GetString("name"), Kind: record.GetString("kind"), Enabled: record.GetBool("enabled"), Severity: record.GetString("severity"), EvaluationWindowSeconds: record.GetInt("evaluation_window_seconds"), NodeScope: record.GetString("node_scope"), NodeIDs: record.GetStringSlice("nodes"), ChannelIDs: record.GetStringSlice("channels"), Config: jsonMap(record.Get("config")), Created: record.GetString("created"), Updated: record.GetString("updated")}
}

func (h *Handlers) listAlerts(e *core.RequestEvent) error {
	return h.alertsResponse(e, "")
}

func (h *Handlers) nodeAlerts(e *core.RequestEvent) error {
	if _, err := h.findActiveNode(e.Request.PathValue("id")); err != nil {
		return err
	}
	return h.alertsResponse(e, e.Request.PathValue("id"))
}

func (h *Handlers) alertsResponse(e *core.RequestEvent, forcedNodeID string) error {
	page, _ := strconv.Atoi(e.Request.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(e.Request.URL.Query().Get("per_page"))
	if perPage != 25 && perPage != 50 && perPage != 100 {
		perPage = 25
	}
	filters := []string{"1=1"}
	params := map[string]any{}
	for key, field := range map[string]string{"monitor_id": "monitor", "severity": "severity_snapshot"} {
		if value := e.Request.URL.Query().Get(key); value != "" {
			filters = append(filters, field+" = {:"+key+"}")
			params[key] = value
		}
	}
	if status := e.Request.URL.Query().Get("status"); status != "" {
		if status == "history" {
			filters = append(filters, "status != 'firing'")
		} else {
			filters = append(filters, "status = {:status}")
			params["status"] = status
		}
	}
	nodeID := forcedNodeID
	if nodeID == "" {
		nodeID = e.Request.URL.Query().Get("node_id")
	}
	if nodeID != "" {
		filters = append(filters, "node = {:node_id}")
		params["node_id"] = nodeID
	}
	all, err := h.app.FindRecordsByFilter("alerts", strings.Join(filters, " && "), "", 0, 0, params)
	if err != nil {
		return apis.NewBadRequestError("failed to list alerts", err)
	}
	sortAlerts(all)
	total := len(all)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	items := make([]Alert, 0, end-start)
	for _, record := range all[start:end] {
		items = append(items, h.publicAlert(record))
	}
	return ok(e, AlertListResponse{Items: items, Total: int64(total), Page: page, PerPage: perPage})
}

func (h *Handlers) alertSummary(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("alerts", "status = 'firing'", "", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to summarize alerts", err)
	}
	sortAlerts(records)
	result := AlertSummaryResponse{Total: int64(len(records))}
	limit := len(records)
	if limit > 5 {
		limit = 5
	}
	result.Items = make([]Alert, 0, limit)
	for i, record := range records {
		if record.GetString("severity_snapshot") == "critical" {
			result.Critical++
		} else {
			result.Warning++
		}
		if i < limit {
			result.Items = append(result.Items, h.publicAlert(record))
		}
	}
	return ok(e, result)
}

func sortAlerts(records []*core.Record) {
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if (a.GetString("status") == "firing") != (b.GetString("status") == "firing") {
			return a.GetString("status") == "firing"
		}
		if (a.GetString("severity_snapshot") == "critical") != (b.GetString("severity_snapshot") == "critical") {
			return a.GetString("severity_snapshot") == "critical"
		}
		return a.GetDateTime("started_at").Time().After(b.GetDateTime("started_at").Time())
	})
}

func (h *Handlers) publicAlert(record *core.Record) Alert {
	start := record.GetDateTime("started_at").Time()
	end := record.GetDateTime("ended_at").Time()
	if end.IsZero() {
		end = time.Now().UTC()
	}
	result := Alert{ID: record.Id, MonitorID: record.GetString("monitor"), Status: record.GetString("status"), Severity: record.GetString("severity_snapshot"), MonitorName: record.GetString("monitor_name_snapshot"), MonitorKind: record.GetString("monitor_kind_snapshot"), MonitorConfig: jsonMap(record.Get("monitor_config_snapshot")), EvaluationWindowSeconds: record.GetInt("evaluation_window_seconds_snapshot"), FiringValue: jsonMap(record.Get("firing_value")), RecoveryValue: jsonMap(record.Get("recovery_value")), StartedAt: record.GetString("started_at"), EndedAt: record.GetString("ended_at"), LastEvaluatedAt: record.GetString("last_evaluated_at"), ResolutionReason: record.GetString("resolution_reason"), DurationSeconds: int64(end.Sub(start).Seconds())}
	node := h.nodeRefByID(record.GetString("node"))
	result.Node = NodeRef{ID: fmt.Sprint(node["id"]), Name: fmt.Sprint(node["name"]), Deleted: node["deleted"] == true}
	deliveries, _ := h.app.FindRecordsByFilter("alert_deliveries", "alert = {:a}", "", 0, 0, map[string]any{"a": record.Id})
	for _, delivery := range deliveries {
		switch delivery.GetString("status") {
		case "succeeded":
			result.Deliveries.Succeeded++
		case "failed":
			result.Deliveries.Failed++
		case "skipped":
			result.Deliveries.Skipped++
		}
	}
	return result
}

func jsonMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
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
