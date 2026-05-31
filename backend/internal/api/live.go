package api

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/hysteria"
)

// GET /users/:id/live
// Real-time diagnostics: pulls /dump/streams and /online from every visible
// node concurrently, filters by the user's auth_string, and aggregates. Never
// cached, never persisted.
func (h *Handlers) userLive(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	authStr := u.GetString("auth_string")

	nodes, err := h.nodesForUser(u.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to list nodes", err)
	}

	type nodeResult struct {
		nodeID, nodeName string
		online           int
		streams          []hysteria.Stream
		errMsg           string
	}

	results := make([]nodeResult, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func(i int, n *core.Record) {
			defer wg.Done()
			res := nodeResult{nodeID: n.Id, nodeName: n.GetString("name")}
			secret, derr := h.box.Decrypt(n.GetString("api_secret"))
			if derr != nil {
				res.errMsg = "decrypt secret failed"
				results[i] = res
				return
			}
			cl := hysteria.New(n.GetString("api_url"), secret, 5*time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			streams, serr := cl.DumpStreams(ctx)
			if serr != nil {
				res.errMsg = serr.Error()
				results[i] = res
				return
			}
			for _, s := range streams {
				if s.Auth == authStr {
					res.streams = append(res.streams, s)
				}
			}
			if online, oerr := cl.Online(ctx); oerr == nil {
				res.online = online[authStr]
			}
			results[i] = res
		}(i, n)
	}
	wg.Wait()

	now := time.Now().UTC()
	var totalOnline, totalStreams int
	byNode := make([]map[string]any, 0, len(results))

	domainAgg := map[string]*struct {
		streams int
		tx, rx  int64
	}{}
	connAgg := map[int64]*struct {
		count   int
		tx, rx  int64
		domains map[string]int64 // domain -> rx for picking top
	}{}

	for _, r := range results {
		nodeStreams := make([]map[string]any, 0, len(r.streams))
		for _, s := range r.streams {
			lifetime, idle := lifetimeIdle(s.InitialAt, s.LastActiveAt, now)
			nodeStreams = append(nodeStreams, map[string]any{
				"connection":      s.Connection,
				"stream":          s.Stream,
				"state":           s.State,
				"req_addr":        s.ReqAddr,
				"hooked_req_addr": s.HookedReqAddr,
				"tx":              s.Tx,
				"rx":              s.Rx,
				"initial_at":      s.InitialAt,
				"last_active_at":  s.LastActiveAt,
				"lifetime_sec":    lifetime,
				"idle_sec":        idle,
			})

			// domain aggregation
			dom := hostOf(s.HookedReqAddr)
			if dom == "" {
				dom = hostOf(s.ReqAddr)
			}
			da := domainAgg[dom]
			if da == nil {
				da = &struct {
					streams int
					tx, rx  int64
				}{}
				domainAgg[dom] = da
			}
			da.streams++
			da.tx += s.Tx
			da.rx += s.Rx

			// connection (device) aggregation
			ca := connAgg[s.Connection]
			if ca == nil {
				ca = &struct {
					count   int
					tx, rx  int64
					domains map[string]int64
				}{domains: map[string]int64{}}
				connAgg[s.Connection] = ca
			}
			ca.count++
			ca.tx += s.Tx
			ca.rx += s.Rx
			ca.domains[dom] += s.Rx + s.Tx
		}

		totalOnline += r.online
		totalStreams += len(r.streams)

		nodeEntry := map[string]any{
			"node":           map[string]any{"id": r.nodeID, "name": r.nodeName},
			"online_devices": r.online,
			"streams":        nodeStreams,
		}
		if r.errMsg != "" {
			nodeEntry["error"] = r.errMsg
		}
		byNode = append(byNode, nodeEntry)
	}

	// top domains sorted by total bytes desc
	topDomains := make([]map[string]any, 0, len(domainAgg))
	for dom, a := range domainAgg {
		topDomains = append(topDomains, map[string]any{
			"domain":  dom,
			"streams": a.streams,
			"tx":      a.tx,
			"rx":      a.rx,
		})
	}
	sort.Slice(topDomains, func(i, j int) bool {
		bi := topDomains[i]["tx"].(int64) + topDomains[i]["rx"].(int64)
		bj := topDomains[j]["tx"].(int64) + topDomains[j]["rx"].(int64)
		return bi > bj
	})

	// per-connection (device) breakdown
	byConn := make([]map[string]any, 0, len(connAgg))
	for conn, a := range connAgg {
		top := ""
		var topBytes int64 = -1
		for d, b := range a.domains {
			if b > topBytes {
				topBytes = b
				top = d
			}
		}
		byConn = append(byConn, map[string]any{
			"connection":   conn,
			"stream_count": a.count,
			"tx":           a.tx,
			"rx":           a.rx,
			"top_domain":   top,
		})
	}
	sort.Slice(byConn, func(i, j int) bool {
		bi := byConn[i]["tx"].(int64) + byConn[i]["rx"].(int64)
		bj := byConn[j]["tx"].(int64) + byConn[j]["rx"].(int64)
		return bi > bj
	})

	return ok(e, map[string]any{
		"online_devices": totalOnline,
		"active_streams": totalStreams,
		"by_node":        byNode,
		"top_domains":    topDomains,
		"by_connection":  byConn,
	})
}

// hostOf strips the port from "host:port", leaving the host/domain.
func hostOf(addr string) string {
	if addr == "" {
		return ""
	}
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

// lifetimeIdle computes total lifetime and idle seconds from RFC3339 timestamps.
func lifetimeIdle(initialAt, lastActiveAt string, now time.Time) (int64, int64) {
	var lifetime, idle int64 = -1, -1
	if t, err := time.Parse(time.RFC3339Nano, initialAt); err == nil {
		lifetime = int64(now.Sub(t).Seconds())
	}
	if t, err := time.Parse(time.RFC3339Nano, lastActiveAt); err == nil {
		idle = int64(now.Sub(t).Seconds())
	}
	return lifetime, idle
}
