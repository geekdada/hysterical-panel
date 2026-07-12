// Package collector polls every enabled Hysteria node's /traffic and /online
// endpoints. It turns cumulative traffic counters into per-user, per-node
// deltas and replaces the latest online-device projection for each node.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/hysteria"
	"hysterical-panel/internal/onlinedevices"
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
	nodes, err := c.app.FindRecordsByFilter("nodes", "deleted_at = '' && enabled = true", "", 0, 0)
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

	var traffic map[string]hysteria.TrafficEntry
	var online map[string]int64
	var trafficErr, onlineErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		traffic, trafficErr = cl.Traffic(cctx)
	}()
	go func() {
		defer wg.Done()
		online, onlineErr = cl.Online(cctx)
	}()
	wg.Wait()

	if trafficErr != nil {
		c.recordNodeError(node, "poll /traffic: "+trafficErr.Error())
		trafficErr = fmt.Errorf("poll /traffic: %w", trafficErr)
	} else if err := c.recordTraffic(node, traffic); err != nil {
		trafficErr = err
	}

	if onlineErr != nil {
		onlineErr = fmt.Errorf("poll /online: %w", onlineErr)
	} else if err := c.recordOnlineSnapshot(node.Id, online, time.Now().UTC()); err != nil {
		onlineErr = fmt.Errorf("record /online snapshot: %w", err)
	}

	return errors.Join(trafficErr, onlineErr)
}

func (c *Collector) recordTraffic(node *core.Record, traffic map[string]hysteria.TrafficEntry) error {
	previousPoll := node.GetDateTime("last_polled_at").Time()

	now := time.Now().UTC()
	bucketHour := now.Truncate(time.Hour)
	bucketDay := now.Truncate(24 * time.Hour)
	var nodeDtx, nodeDrx int64

	resolver, err := authstrings.LoadResolver(c.app)
	if err != nil {
		return err
	}
	type userCounters struct {
		user   *core.Record
		tx, rx int64
	}
	byUser := make(map[string]*userCounters)
	for clientID, entry := range traffic {
		user := resolver.Resolve(clientID)
		if user == nil {
			// Unknown or deleted Node Client ID — skip silently.
			continue
		}
		counters := byUser[user.Id]
		if counters == nil {
			counters = &userCounters{user: user}
			byUser[user.Id] = counters
		}
		if entry.Tx < 0 || entry.Rx < 0 || entry.Tx > math.MaxInt64-counters.tx || entry.Rx > math.MaxInt64-counters.rx {
			return fmt.Errorf("invalid traffic counters for user %s", user.Id)
		}
		counters.tx += entry.Tx
		counters.rx += entry.Rx
	}

	for _, counters := range byUser {
		user := counters.user
		// Always advance the cursor, even for disabled users, so re-enabling
		// resumes from "now" instead of dumping the whole disabled-period
		// counter into a single bucket.
		dtx, drx, err := c.applyDelta(user, node, counters.tx, counters.rx)
		if err != nil {
			log.Printf("[collector] delta user=%s node=%s: %v", user.Id, node.Id, err)
			continue
		}
		// Disabled users keep their counter tracked but stop accruing usage.
		if user.GetString("status") != "active" {
			continue
		}
		nodeDtx += dtx
		nodeDrx += drx
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
	node.Set("current_tx_speed", speedPerSecond(nodeDtx, previousPoll, now))
	node.Set("current_rx_speed", speedPerSecond(nodeDrx, previousPoll, now))
	elapsed := int64(0)
	if !previousPoll.IsZero() {
		elapsed = int64(now.Sub(previousPoll).Seconds())
	}
	return c.app.RunInTransaction(func(txApp core.App) error {
		if elapsed > 0 {
			if err := recordObservation(txApp, node, now, elapsed, nodeDtx, nodeDrx); err != nil {
				return err
			}
		}
		return txApp.Save(node)
	})
}

// recordOnlineSnapshot replaces the latest known positive per-user counts for
// one node. The node-wide total intentionally includes auth strings that don't
// map to a panel user, while user projections only contain known users.
func (c *Collector) recordOnlineSnapshot(nodeID string, online map[string]int64, observedAt time.Time) error {
	var total int64
	for _, count := range online {
		if count < 0 {
			return fmt.Errorf("negative online device count")
		}
		if count > math.MaxInt64-total {
			return fmt.Errorf("online device count overflow")
		}
		total += count
	}

	return c.app.RunInTransaction(func(txApp core.App) error {
		node, err := txApp.FindRecordById("nodes", nodeID)
		if err != nil {
			return err
		}
		// The request may have started before an admin disabled or deleted the
		// node. Never let a late response resurrect a cleared projection.
		if !node.GetBool("enabled") || !node.GetDateTime("deleted_at").IsZero() {
			if err := onlinedevices.DeleteNodeCounts(txApp, nodeID); err != nil {
				return err
			}
			node.Set("online_devices", 0)
			if node.GetDateTime("online_devices_observed_at").IsZero() {
				node.Set("online_devices_observed_at", observedAt.UTC())
			}
			return txApp.Save(node)
		}
		resolver, err := authstrings.LoadResolver(txApp)
		if err != nil {
			return err
		}
		countsByUser := make(map[string]int64)
		for clientID, count := range online {
			user := resolver.Resolve(clientID)
			if user == nil {
				continue
			}
			if count > math.MaxInt64-countsByUser[user.Id] {
				return fmt.Errorf("online device count overflow for user %s", user.Id)
			}
			countsByUser[user.Id] += count
		}

		if err := onlinedevices.DeleteNodeCounts(txApp, nodeID); err != nil {
			return err
		}

		collection, err := txApp.FindCollectionByNameOrId("online_device_counts")
		if err != nil {
			return err
		}
		for userID, count := range countsByUser {
			if count == 0 {
				continue
			}
			record := core.NewRecord(collection)
			record.Set("user", userID)
			record.Set("node", nodeID)
			record.Set("count", count)
			if err := txApp.Save(record); err != nil {
				return err
			}
		}

		node.Set("online_devices", total)
		node.Set("online_devices_observed_at", observedAt.UTC())
		return txApp.Save(node)
	})
}

func recordObservation(app core.App, node *core.Record, observedAt time.Time, elapsed, tx, rx int64) error {
	collection, err := app.FindCollectionByNameOrId("monitor_observations")
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("node", node.Id)
	record.Set("observed_at", observedAt.UTC())
	record.Set("elapsed_seconds", elapsed)
	record.Set("tx_bytes", tx)
	record.Set("rx_bytes", rx)
	return app.Save(record)
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

func speedPerSecond(deltaBytes int64, from, to time.Time) int64 {
	if deltaBytes <= 0 || from.IsZero() {
		return 0
	}
	seconds := to.Sub(from).Seconds()
	if seconds <= 0 {
		return 0
	}
	return int64(float64(deltaBytes) / seconds)
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
	node.Set("current_tx_speed", 0)
	node.Set("current_rx_speed", 0)
	if err := c.app.Save(node); err != nil {
		log.Printf("[collector] record node error: %v", err)
	}
}
