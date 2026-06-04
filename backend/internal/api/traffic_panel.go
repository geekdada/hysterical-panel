package api

import (
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
