package api

import (
	"context"
	"log"
	"sync"
	"time"

	"hysterical-panel/internal/hysteria"
)

// kickConcurrency caps the number of in-flight Hysteria /kick calls during a
// fan-out. Designed to be modest on the panel process and on the nodes; tuned
// to 3 as the product requirement.
const kickConcurrency = 3

// kickOverallTimeout bounds the detached background goroutine even if many
// nodes are unresponsive. Each per-node call additionally has its own 5s
// client timeout.
const kickOverallTimeout = 30 * time.Second

// kickPerNodeTimeout is the per-node HTTP client timeout used for a single
// /kick call.
const kickPerNodeTimeout = 5 * time.Second

// kickTarget describes one node to kick a user on.
type kickTarget struct {
	id   string
	name string
	url  string
	// secret is the decrypted node API secret; empty when decryption failed,
	// in which case err is set and the kicker is skipped.
	secret string
	err    error
}

// kickResult records the outcome of one node kick.
type kickResult struct {
	nodeID   string
	nodeName string
	err      error
}

// kicker issues a /kick for one user against one node.
type kicker func(ctx context.Context, t kickTarget) error

// fanOutKicks kicks the user on every target concurrently, capping in-flight
// calls at kickConcurrency. One target failing never aborts the others: each
// target produces its own result (including decryption errors carried on the
// target itself). Returns one result per target, in input order.
func fanOutKicks(ctx context.Context, targets []kickTarget, kick kicker) []kickResult {
	results := make([]kickResult, len(targets))
	if len(targets) == 0 {
		return results
	}

	sem := make(chan struct{}, kickConcurrency)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t kickTarget) {
			defer wg.Done()
			res := kickResult{nodeID: t.id, nodeName: t.name}
			if t.err != nil {
				res.err = t.err
				results[i] = res
				return
			}
			// Acquire the semaphore before issuing the call so that at most
			// kickConcurrency nodes are hit at once.
			sem <- struct{}{}
			defer func() { <-sem }()
			res.err = kick(ctx, t)
			results[i] = res
		}(i, t)
	}
	wg.Wait()
	return results
}

// kickUser issues a best-effort /kick for the given user across every node
// visible to them. It is intended to run as a detached goroutine: it uses a
// fresh context (not the request context, which dies when PATCH returns) and
// never surfaces errors to a caller. All failures are logged only.
//
// This complements hysteriaAuth's 403 (which blocks new connections for
// disabled users): /kick only drops currently-established sessions.
func (h *Handlers) kickUser(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), kickOverallTimeout)
	defer cancel()

	nodes, err := h.nodesForUser(userID)
	if err != nil {
		log.Printf("[kick] user %s: list nodes: %v", userID, err)
		return
	}

	targets := make([]kickTarget, len(nodes))
	for i, n := range nodes {
		t := kickTarget{id: n.Id, name: n.GetString("name"), url: n.GetString("api_url")}
		secret, derr := h.box.Decrypt(n.GetString("api_secret"))
		if derr != nil {
			t.err = derr
		}
		t.secret = secret
		targets[i] = t
	}

	kick := func(ctx context.Context, t kickTarget) error {
		if t.secret == "" {
			return nil
		}
		cl := hysteria.New(t.url, t.secret, kickPerNodeTimeout)
		return cl.Kick(ctx, []string{userID})
	}

	for _, r := range fanOutKicks(ctx, targets, kick) {
		if r.err != nil {
			log.Printf("[kick] user %s node %s (%s): %v", userID, r.nodeID, r.nodeName, r.err)
		}
	}
}
