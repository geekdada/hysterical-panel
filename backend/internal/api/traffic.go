package api

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// GET /users/:id/traffic/summary
func (h *Handlers) trafficSummary(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}

	rows, err := h.app.FindRecordsByFilter(
		"traffic_daily", "user = {:u}", "", 0, 0,
		map[string]any{"u": u.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read traffic", err)
	}

	type nodeAgg struct {
		tx, rx int64
		name   string
	}
	perNode := map[string]*nodeAgg{}
	var totalTx, totalRx int64
	for _, r := range rows {
		nodeID := r.GetString("node")
		a := perNode[nodeID]
		if a == nil {
			a = &nodeAgg{}
			perNode[nodeID] = a
		}
		a.tx += int64(r.GetInt("tx"))
		a.rx += int64(r.GetInt("rx"))
		totalTx += int64(r.GetInt("tx"))
		totalRx += int64(r.GetInt("rx"))
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

	return ok(e, map[string]any{
		"total":   map[string]any{"tx": totalTx, "rx": totalRx},
		"by_node": byNode,
	})
}

// GET /users/:id/traffic/series?granularity=hourly|daily&from=&to=&node=
// from/to are UTC datetimes (same format as bucket in responses, e.g. PocketBase date strings with Z).
func (h *Handlers) trafficSeries(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	q := e.Request.URL.Query()
	gran := q.Get("granularity")
	coll := "traffic_hourly"
	if gran == "daily" {
		coll = "traffic_daily"
	} else {
		gran = "hourly"
	}

	filter := "user = {:u}"
	params := map[string]any{"u": u.Id}
	if from := q.Get("from"); from != "" {
		filter += " && bucket >= {:from}"
		params["from"] = from
	}
	if to := q.Get("to"); to != "" {
		filter += " && bucket <= {:to}"
		params["to"] = to
	}
	if node := q.Get("node"); node != "" {
		filter += " && node = {:node}"
		params["node"] = node
	}

	rows, err := h.app.FindRecordsByFilter(coll, filter, "bucket", 0, 0, params)
	if err != nil {
		return apis.NewBadRequestError("failed to read series", err)
	}

	// collapse across nodes into one point per bucket
	type pt struct{ tx, rx int64 }
	buckets := map[string]*pt{}
	order := []string{}
	for _, r := range rows {
		b := r.GetString("bucket")
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
