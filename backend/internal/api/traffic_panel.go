package api

import (
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// GET /traffic?from=&to=
// Admin dashboard global total from traffic_daily (UTC buckets, inclusive).
func (h *Handlers) panelTraffic(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	if from == "" || to == "" {
		return apis.NewBadRequestError("from and to are required", nil)
	}

	rows, err := h.app.FindRecordsByFilter(
		"traffic_daily", "bucket >= {:from} && bucket <= {:to}", "", 0, 0,
		map[string]any{"from": from, "to": to},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read traffic", err)
	}

	var totalTx, totalRx int64
	for _, r := range rows {
		totalTx += int64(r.GetInt("tx"))
		totalRx += int64(r.GetInt("rx"))
	}

	return ok(e, map[string]any{
		"from":  from,
		"to":    to,
		"total": map[string]any{"tx": totalTx, "rx": totalRx},
	})
}

// GET /nodes/traffic/summary?from=&to=
// Admin dashboard per-node total from traffic_hourly over a UTC datetime range.
func (h *Handlers) panelNodeTrafficSummary(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	if from == "" || to == "" {
		return apis.NewBadRequestError("from and to are required", nil)
	}

	rows, err := h.app.FindRecordsByFilter(
		"traffic_hourly", "bucket >= {:from} && bucket <= {:to}", "", 0, 0,
		map[string]any{"from": from, "to": to},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read node traffic", err)
	}

	type nodeAgg struct{ tx, rx int64 }
	perNode := map[string]*nodeAgg{}
	var totalTx, totalRx int64
	for _, r := range rows {
		nodeID := r.GetString("node")
		a := perNode[nodeID]
		if a == nil {
			a = &nodeAgg{}
			perNode[nodeID] = a
		}
		tx := int64(r.GetInt("tx"))
		rx := int64(r.GetInt("rx"))
		a.tx += tx
		a.rx += rx
		totalTx += tx
		totalRx += rx
	}

	byNode := make([]map[string]any, 0, len(perNode))
	for nodeID, a := range perNode {
		name := nodeID
		if n, err := h.app.FindRecordById("nodes", nodeID); err == nil {
			name = n.GetString("name")
		}
		byNode = append(byNode, map[string]any{
			"node": map[string]any{"id": nodeID, "name": name},
			"tx":   a.tx,
			"rx":   a.rx,
		})
	}
	sort.Slice(byNode, func(i, j int) bool {
		return byNode[i]["tx"].(int64)+byNode[i]["rx"].(int64) > byNode[j]["tx"].(int64)+byNode[j]["rx"].(int64)
	})

	return ok(e, map[string]any{
		"from":    from,
		"to":      to,
		"total":   map[string]any{"tx": totalTx, "rx": totalRx},
		"by_node": byNode,
	})
}
