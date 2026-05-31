package api

import (
	"sort"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// GET /nodes/:id/traffic/summary
// Node-wide traffic: aggregates the node's traffic across every user, with a
// per-user breakdown sorted by total bytes (top talkers first).
func (h *Handlers) nodeTrafficSummary(e *core.RequestEvent) error {
	n, err := h.app.FindRecordById("nodes", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("node not found", err)
	}

	rows, err := h.app.FindRecordsByFilter(
		"traffic_daily", "node = {:n}", "", 0, 0,
		map[string]any{"n": n.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read traffic", err)
	}

	type userAgg struct{ tx, rx int64 }
	perUser := map[string]*userAgg{}
	var totalTx, totalRx int64
	for _, r := range rows {
		userID := r.GetString("user")
		a := perUser[userID]
		if a == nil {
			a = &userAgg{}
			perUser[userID] = a
		}
		a.tx += int64(r.GetInt("tx"))
		a.rx += int64(r.GetInt("rx"))
		totalTx += int64(r.GetInt("tx"))
		totalRx += int64(r.GetInt("rx"))
	}

	byUser := make([]map[string]any, 0, len(perUser))
	for userID, a := range perUser {
		email := userID
		if u, err := h.app.FindRecordById("users", userID); err == nil {
			email = u.GetString("email")
		}
		byUser = append(byUser, map[string]any{
			"user": map[string]any{"id": userID, "email": email},
			"tx":   a.tx,
			"rx":   a.rx,
		})
	}
	sort.Slice(byUser, func(i, j int) bool {
		return byUser[i]["tx"].(int64)+byUser[i]["rx"].(int64) > byUser[j]["tx"].(int64)+byUser[j]["rx"].(int64)
	})

	return ok(e, map[string]any{
		"total":   map[string]any{"tx": totalTx, "rx": totalRx},
		"by_user": byUser,
	})
}

// GET /nodes/:id/traffic/series?granularity=hourly|daily&from=&to=
// from/to are UTC datetimes (same format as bucket in responses). Collapses the
// node's rows across all users into one point per bucket.
func (h *Handlers) nodeTrafficSeries(e *core.RequestEvent) error {
	n, err := h.app.FindRecordById("nodes", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("node not found", err)
	}
	q := e.Request.URL.Query()
	gran := q.Get("granularity")
	coll := "traffic_hourly"
	if gran == "daily" {
		coll = "traffic_daily"
	} else {
		gran = "hourly"
	}

	filter := "node = {:n}"
	params := map[string]any{"n": n.Id}
	if from := q.Get("from"); from != "" {
		filter += " && bucket >= {:from}"
		params["from"] = from
	}
	if to := q.Get("to"); to != "" {
		filter += " && bucket <= {:to}"
		params["to"] = to
	}

	rows, err := h.app.FindRecordsByFilter(coll, filter, "bucket", 0, 0, params)
	if err != nil {
		return apis.NewBadRequestError("failed to read series", err)
	}

	// collapse across users into one point per bucket
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
