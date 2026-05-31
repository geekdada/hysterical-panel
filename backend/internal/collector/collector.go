// Package collector polls every enabled Hysteria node's /traffic endpoint and
// turns the cumulative counters into per-user, per-node deltas, persisting them
// into the users running totals and the hourly/daily aggregation tables.
package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/hysteria"
)

// Collector owns the background polling loop.
type Collector struct {
	app core.App
	box *cryptobox.Box

	mu       sync.Mutex
	lastPoll map[string]time.Time // node id -> last poll time
}

// New builds a collector.
func New(app core.App, box *cryptobox.Box) *Collector {
	return &Collector{
		app:      app,
		box:      box,
		lastPoll: map[string]time.Time{},
	}
}

// Start launches the loop. It ticks every 5s and polls each node when its own
// poll_interval has elapsed. Cancel ctx to stop.
func (c *Collector) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		// fire once shortly after boot
		c.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.tick(ctx)
			}
		}
	}()
}

func (c *Collector) tick(ctx context.Context) {
	nodes, err := c.app.FindRecordsByFilter("nodes", "enabled = true", "", 0, 0)
	if err != nil {
		log.Printf("[collector] list nodes: %v", err)
		return
	}
	now := time.Now()
	for _, node := range nodes {
		interval := node.GetInt("poll_interval")
		if interval <= 0 {
			interval = 30
		}
		c.mu.Lock()
		last := c.lastPoll[node.Id]
		due := last.IsZero() || now.Sub(last) >= time.Duration(interval)*time.Second
		c.mu.Unlock()
		if !due {
			continue
		}
		c.mu.Lock()
		c.lastPoll[node.Id] = now
		c.mu.Unlock()

		if err := c.pollNode(ctx, node); err != nil {
			log.Printf("[collector] node %s (%s): %v", node.GetString("name"), node.Id, err)
		}
	}
}

func (c *Collector) pollNode(ctx context.Context, node *core.Record) error {
	secret, err := c.box.Decrypt(node.GetString("api_secret"))
	if err != nil {
		c.recordNodeError(node, "decrypt secret: "+err.Error())
		return err
	}
	cl := hysteria.New(node.GetString("api_url"), secret, 8*time.Second)

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	traffic, err := cl.Traffic(cctx)
	if err != nil {
		c.recordNodeError(node, "poll /traffic: "+err.Error())
		return err
	}

	now := time.Now().UTC()
	bucketHour := now.Truncate(time.Hour)
	bucketDay := now.Truncate(24 * time.Hour)

	for authStr, entry := range traffic {
		user, err := c.app.FindFirstRecordByFilter("users", "auth_string = {:a}", map[string]any{"a": authStr})
		if err != nil || user == nil {
			// orphan auth string (deleted/unknown user) — skip silently
			continue
		}
		dtx, drx, err := c.applyDelta(user, node, entry.Tx, entry.Rx)
		if err != nil {
			log.Printf("[collector] delta user=%s node=%s: %v", authStr, node.Id, err)
			continue
		}
		if dtx == 0 && drx == 0 {
			continue
		}
		if err := c.bumpUserTotals(user, dtx, drx); err != nil {
			log.Printf("[collector] bump user totals: %v", err)
		}
		if err := c.upsertAgg("traffic_hourly", user.Id, node.Id, bucketHour, dtx, drx); err != nil {
			log.Printf("[collector] upsert hourly: %v", err)
		}
		if err := c.upsertAgg("traffic_daily", user.Id, node.Id, bucketDay, dtx, drx); err != nil {
			log.Printf("[collector] upsert daily: %v", err)
		}
	}

	node.Set("last_polled_at", now)
	node.Set("last_error", "")
	if err := c.app.Save(node); err != nil {
		return err
	}
	return nil
}

// applyDelta reads the cursor for (user,node), computes the delta with reset
// handling, and writes the new cursor. Returns the delta to be accumulated.
func (c *Collector) applyDelta(user, node *core.Record, curTx, curRx int64) (int64, int64, error) {
	cursor, err := c.app.FindFirstRecordByFilter(
		"traffic_cursor",
		"user = {:u} && node = {:n}",
		map[string]any{"u": user.Id, "n": node.Id},
	)
	if err != nil || cursor == nil {
		// first observation: treat the whole counter as the delta
		coll, cerr := c.app.FindCollectionByNameOrId("traffic_cursor")
		if cerr != nil {
			return 0, 0, cerr
		}
		cursor = core.NewRecord(coll)
		cursor.Set("user", user.Id)
		cursor.Set("node", node.Id)
		cursor.Set("last_tx", curTx)
		cursor.Set("last_rx", curRx)
		if serr := c.app.Save(cursor); serr != nil {
			return 0, 0, serr
		}
		return curTx, curRx, nil
	}

	lastTx := int64(cursor.GetInt("last_tx"))
	lastRx := int64(cursor.GetInt("last_rx"))

	dtx := delta(curTx, lastTx)
	drx := delta(curRx, lastRx)

	cursor.Set("last_tx", curTx)
	cursor.Set("last_rx", curRx)
	if err := c.app.Save(cursor); err != nil {
		return 0, 0, err
	}
	return dtx, drx, nil
}

// delta handles the counter-reset case: if the current value dropped below the
// last seen value, Hysteria restarted and the counter went back to zero, so the
// current value itself is the increment.
func delta(cur, last int64) int64 {
	if cur >= last {
		return cur - last
	}
	return cur
}

func (c *Collector) bumpUserTotals(user *core.Record, dtx, drx int64) error {
	user.Set("used_tx", int64(user.GetInt("used_tx"))+dtx)
	user.Set("used_rx", int64(user.GetInt("used_rx"))+drx)
	return c.app.Save(user)
}

func (c *Collector) upsertAgg(coll, userID, nodeID string, bucket time.Time, dtx, drx int64) error {
	rec, err := c.app.FindFirstRecordByFilter(
		coll,
		"user = {:u} && node = {:n} && bucket = {:b}",
		map[string]any{"u": userID, "n": nodeID, "b": bucket.Format("2006-01-02 15:04:05.000Z")},
	)
	if err != nil || rec == nil {
		c2, cerr := c.app.FindCollectionByNameOrId(coll)
		if cerr != nil {
			return cerr
		}
		rec = core.NewRecord(c2)
		rec.Set("user", userID)
		rec.Set("node", nodeID)
		rec.Set("bucket", bucket.UTC())
		rec.Set("tx", dtx)
		rec.Set("rx", drx)
		return c.app.Save(rec)
	}
	rec.Set("tx", int64(rec.GetInt("tx"))+dtx)
	rec.Set("rx", int64(rec.GetInt("rx"))+drx)
	return c.app.Save(rec)
}

func (c *Collector) recordNodeError(node *core.Record, msg string) {
	node.Set("last_error", msg)
	if err := c.app.Save(node); err != nil {
		log.Printf("[collector] record node error: %v", err)
	}
}
