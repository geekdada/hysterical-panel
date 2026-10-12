package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// The old 30-day prune deleted daily rows of the cutoff day while keeping that
// day's later hourly rows. Rebuild those daily rows from the surviving hours
// so the day is not lost once hourly pruning removes them too.
func init() {
	m.Register(func(app core.App) error {
		type missingDay struct {
			User string `db:"user"`
			Node string `db:"node"`
			Day  string `db:"day"`
			Tx   int64  `db:"tx"`
			Rx   int64  `db:"rx"`
		}
		var missing []missingDay
		err := app.DB().NewQuery(`
			SELECT h.user AS user, h.node AS node, substr(h.bucket, 1, 10) || ' 00:00:00.000Z' AS day,
				SUM(h.tx) AS tx, SUM(h.rx) AS rx
			FROM traffic_hourly h
			WHERE NOT EXISTS (
				SELECT 1 FROM traffic_daily d
				WHERE d.user = h.user AND d.node = h.node AND d.bucket = substr(h.bucket, 1, 10) || ' 00:00:00.000Z'
			)
			GROUP BY h.user, h.node, day`).All(&missing)
		if err != nil {
			return err
		}
		if len(missing) == 0 {
			return nil
		}

		daily, err := app.FindCollectionByNameOrId("traffic_daily")
		if err != nil {
			return err
		}
		for _, row := range missing {
			rec := core.NewRecord(daily)
			rec.Load(map[string]any{"user": row.User, "node": row.Node, "bucket": row.Day, "tx": row.Tx, "rx": row.Rx})
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		// Backfilled rows match their hourly sums and stay valid after rollback.
		return nil
	})
}
