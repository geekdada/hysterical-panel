package api

import (
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func requiredTrafficRange(e *core.RequestEvent) (string, string, error) {
	q := e.Request.URL.Query()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	if from == "" || to == "" {
		return "", "", apis.NewBadRequestError("from and to are required", nil)
	}
	return from, to, nil
}

// GET /traffic?from=&to=
// Admin dashboard global total from traffic_hourly over a UTC datetime range.
func (h *Handlers) panelTraffic(e *core.RequestEvent) error {
	from, to, rangeErr := requiredTrafficRange(e)
	if rangeErr != nil {
		return rangeErr
	}

	rows, err := h.app.FindRecordsByFilter(
		"traffic_hourly", "bucket >= {:from} && bucket <= {:to}", "", 0, 0,
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

// GET /traffic/series?granularity=hourly|daily&from=&to=
// Admin dashboard global time series. Reads hourly rows inside the requested
// UTC range and collapses them across every node and user. Daily granularity
// groups the matching hourly rows into UTC day buckets, so totals stay aligned
// with range summaries that use the same hourly table.
func (h *Handlers) panelTrafficSeries(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	gran := strings.TrimSpace(q.Get("granularity"))
	if gran != "daily" {
		gran = "hourly"
	}

	from, to, rangeErr := requiredTrafficRange(e)
	if rangeErr != nil {
		return rangeErr
	}

	rows, err := h.app.FindRecordsByFilter(
		"traffic_hourly", "bucket >= {:from} && bucket <= {:to}", "bucket", 0, 0,
		map[string]any{"from": from, "to": to},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read series", err)
	}

	type pt struct{ tx, rx int64 }
	buckets := map[string]*pt{}
	order := []string{}
	for _, r := range rows {
		b := r.GetString("bucket")
		if gran == "daily" {
			b = utcDailyBucketString(b)
		}
		p := buckets[b]
		if p == nil {
			p = &pt{}
			buckets[b] = p
			order = append(order, b)
		}
		p.tx += int64(r.GetInt("tx"))
		p.rx += int64(r.GetInt("rx"))
	}

	points := make([]map[string]any, 0, len(order))
	for _, b := range order {
		points = append(points, map[string]any{
			"bucket": b,
			"tx":     buckets[b].tx,
			"rx":     buckets[b].rx,
		})
	}

	return ok(e, map[string]any{"granularity": gran, "points": points})
}

func utcDailyBucketString(bucket string) string {
	t, err := time.Parse("2006-01-02 15:04:05.000Z", bucket)
	if err != nil {
		return bucket
	}
	return utcDailyBucket(t)
}

// GET /nodes/traffic/summary?from=&to=
// Admin dashboard per-node total from traffic_hourly over a UTC datetime range.
func (h *Handlers) panelNodeTrafficSummary(e *core.RequestEvent) error {
	from, to, rangeErr := requiredTrafficRange(e)
	if rangeErr != nil {
		return rangeErr
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
