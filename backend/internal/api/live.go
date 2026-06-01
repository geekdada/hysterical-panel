package api

import (
	"context"
	"net"
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
	agg := newLiveAggregator(h.ipLookup)

	for _, r := range results {
		nodeStreams := make([]map[string]any, 0, len(r.streams))
		for _, s := range r.streams {
			nodeStreams = append(nodeStreams, agg.add(s, now))
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

	return ok(e, map[string]any{
		"online_devices": totalOnline,
		"active_streams": totalStreams,
		"by_node":        byNode,
		"top_domains":    agg.topDomains(),
		"by_connection":  agg.byConnection(),
	})
}

// hostOf strips the port from "host:port", leaving the host/domain. It keeps
// IPv6 literals intact whether they are bracketed with a port or bare.
func hostOf(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return trimHostBrackets(host)
	}
	return trimHostBrackets(addr)
}

func trimHostBrackets(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return host
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

// liveAggregator accumulates per-domain and per-connection (device) stats while
// rendering individual streams. It is shared by the user-scoped and node-scoped
// live endpoints so the aggregation logic lives in exactly one place.
type liveAggregator struct {
	domains  map[string]*domainStat
	conns    map[int64]*connStat
	ipLookup ipMetadataLookup
}

type domainStat struct {
	streams int
	tx, rx  int64
}

type connStat struct {
	count   int
	tx, rx  int64
	domains map[string]int64 // domain -> total bytes, for picking the top domain
}

func newLiveAggregator(ipLookup ipMetadataLookup) *liveAggregator {
	return &liveAggregator{
		domains:  map[string]*domainStat{},
		conns:    map[int64]*connStat{},
		ipLookup: ipLookup,
	}
}

// add records one stream into the domain/connection tallies and returns the
// stream rendered as a response map (with computed lifetime/idle).
func (a *liveAggregator) add(s hysteria.Stream, now time.Time) map[string]any {
	lifetime, idle := lifetimeIdle(s.InitialAt, s.LastActiveAt, now)
	rendered := map[string]any{
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
	}

	dom := hostOf(s.HookedReqAddr)
	if dom == "" {
		dom = hostOf(s.ReqAddr)
	}
	ds := a.domains[dom]
	if ds == nil {
		ds = &domainStat{}
		a.domains[dom] = ds
	}
	ds.streams++
	ds.tx += s.Tx
	ds.rx += s.Rx

	cs := a.conns[s.Connection]
	if cs == nil {
		cs = &connStat{domains: map[string]int64{}}
		a.conns[s.Connection] = cs
	}
	cs.count++
	cs.tx += s.Tx
	cs.rx += s.Rx
	cs.domains[dom] += s.Rx + s.Tx

	return rendered
}

// topDomains returns domain aggregates sorted by total bytes descending.
func (a *liveAggregator) topDomains() []map[string]any {
	out := make([]map[string]any, 0, len(a.domains))
	for dom, d := range a.domains {
		entry := map[string]any{
			"domain":  dom,
			"streams": d.streams,
			"tx":      d.tx,
			"rx":      d.rx,
		}
		if a.ipLookup != nil {
			if meta := a.ipLookup.LookupHost(dom); meta != nil {
				entry["ip_meta"] = meta
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["tx"].(int64)+out[i]["rx"].(int64) > out[j]["tx"].(int64)+out[j]["rx"].(int64)
	})
	return out
}

// byConnection returns per-connection (device) aggregates, each tagged with its
// heaviest domain, sorted by total bytes descending.
func (a *liveAggregator) byConnection() []map[string]any {
	out := make([]map[string]any, 0, len(a.conns))
	for conn, c := range a.conns {
		top := ""
		var topBytes int64 = -1
		for d, b := range c.domains {
			if b > topBytes {
				topBytes = b
				top = d
			}
		}
		out = append(out, map[string]any{
			"connection":   conn,
			"stream_count": c.count,
			"tx":           c.tx,
			"rx":           c.rx,
			"top_domain":   top,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["tx"].(int64)+out[i]["rx"].(int64) > out[j]["tx"].(int64)+out[j]["rx"].(int64)
	})
	return out
}
