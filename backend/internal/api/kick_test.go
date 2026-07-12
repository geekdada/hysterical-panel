package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

// TestFanOutKicksConcurrencyCap proves the semaphore holds: with 10 targets
// and a kicker that blocks briefly, the high-water mark of concurrent
// executions must never exceed kickConcurrency (3).
func TestFanOutKicksConcurrencyCap(t *testing.T) {
	const n = 10
	targets := make([]kickTarget, n)
	for i := range targets {
		targets[i] = kickTarget{id: "n", name: "node", url: "http://x", secret: "s"}
	}

	var inFlight, maxInFlight int32
	var mu sync.Mutex

	kick := func(_ context.Context, _ kickTarget) error {
		cur := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if int(cur) > int(maxInFlight) {
			maxInFlight = cur
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}

	results := fanOutKicks(context.Background(), targets, kick)
	if len(results) != n {
		t.Fatalf("len(results) = %d, want %d", len(results), n)
	}
	if int(maxInFlight) > kickConcurrency {
		t.Fatalf("max in-flight = %d, must not exceed %d", maxInFlight, kickConcurrency)
	}
	if int(maxInFlight) < 2 {
		t.Fatalf("max in-flight = %d, fan-out did not run concurrently", maxInFlight)
	}
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("result %d: unexpected error %v", i, r.err)
		}
	}
}

// TestFanOutKicksOneFailureDoesNotAbortOthers injects a failing kicker for a
// single target surrounded by healthy ones and asserts every target still
// produces a result and siblings still execute.
func TestFanOutKicksOneFailureDoesNotAbortOthers(t *testing.T) {
	targets := []kickTarget{
		{id: "a", name: "node-a", url: "http://a", secret: "s"},
		{id: "b", name: "node-b", url: "http://b", secret: "s"},
		{id: "c", name: "node-c", url: "http://c", secret: "s"},
	}
	var ran atomic.Int32
	kick := func(_ context.Context, t kickTarget) error {
		ran.Add(1)
		if t.id == "b" {
			return errors.New("node down")
		}
		return nil
	}

	results := fanOutKicks(context.Background(), targets, kick)
	if got := ran.Load(); got != int32(len(targets)) {
		t.Fatalf("kicker ran %d times, want %d", got, len(targets))
	}
	if len(results) != len(targets) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(targets))
	}
	var failed int
	for _, r := range results {
		if r.err != nil {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("failed results = %d, want exactly 1", failed)
	}
	if results[1].err == nil {
		t.Fatalf("expected target b to fail, got nil error")
	}
}

// TestFanOutKicksTargetDecryptErrorIsCarried verifies that a target carrying a
// pre-resolved error (e.g. failed api_secret decryption) is reported as failed
// without invoking the kicker.
func TestFanOutKicksTargetDecryptErrorIsCarried(t *testing.T) {
	boom := errors.New("decrypt failed")
	targets := []kickTarget{
		{id: "ok", name: "ok-node", url: "http://ok", secret: "s"},
		{id: "bad", name: "bad-node", url: "http://bad", err: boom},
	}
	var ran atomic.Int32
	kick := func(_ context.Context, tgt kickTarget) error {
		ran.Add(1)
		if tgt.id == "bad" {
			t.Fatalf("kicker must not be called for a target with a pre-set error")
		}
		return nil
	}

	results := fanOutKicks(context.Background(), targets, kick)
	if got := ran.Load(); got != 1 {
		t.Fatalf("kicker ran %d times, want 1 (the decrypt-failed target is skipped)", got)
	}
	if results[0].err != nil {
		t.Fatalf("ok target: unexpected error %v", results[0].err)
	}
	if !errors.Is(results[1].err, boom) {
		t.Fatalf("bad target err = %v, want %v", results[1].err, boom)
	}
}

// TestFanOutKicksEmptyReturnsEmpty is the degenerate case: no targets, no calls.
func TestFanOutKicksEmptyReturnsEmpty(t *testing.T) {
	results := fanOutKicks(context.Background(), nil, func(context.Context, kickTarget) error {
		t.Fatalf("kicker must not be called for empty targets")
		return nil
	})
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestKickUserSendsStableUserID(t *testing.T) {
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	received := make(chan []string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
			t.Errorf("decode kick body: %v", err)
		}
		received <- ids
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes: %v", err)
	}
	secret, err := box.Encrypt("node-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "kick target")
	node.Set("api_url", server.URL)
	node.Set("api_secret", secret)
	node.Set("enabled", true)
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}

	(&Handlers{app: app, box: box}).kickUser("stable-user-id")
	select {
	case ids := <-received:
		if len(ids) != 1 || ids[0] != "stable-user-id" {
			t.Fatalf("kick ids = %v, want [stable-user-id]", ids)
		}
	case <-time.After(time.Second):
		t.Fatal("kick request not received")
	}
}
