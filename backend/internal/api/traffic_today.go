package api

import (
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// GET /traffic/today
// Admin dashboard total for the current UTC day.
func (h *Handlers) trafficToday(e *core.RequestEvent) error {
	bucket := time.Now().UTC().Truncate(24 * time.Hour)
	bucketString := bucket.Format("2006-01-02 15:04:05.000Z")

	rows, err := h.app.FindRecordsByFilter(
		"traffic_daily", "bucket = {:bucket}", "", 0, 0,
		map[string]any{"bucket": bucketString},
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
		"utc_date": bucket.Format("2006-01-02"),
		"bucket":   bucketString,
		"total":    map[string]any{"tx": totalTx, "rx": totalRx},
	})
}
