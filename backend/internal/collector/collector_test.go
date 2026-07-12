package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/hysteria"
	_ "hysterical-panel/migrations"
)

func newMigratedCollectorTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

func TestRecordObservationAcceptsZeroByteCounters(t *testing.T) {
	app := newMigratedCollectorTestApp(t)
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes collection: %v", err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "test")
	node.Set("api_url", "http://127.0.0.1:9999")
	node.Set("api_secret", "secret")
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}

	tests := []struct {
		name string
		tx   int64
		rx   int64
	}{
		{name: "idle interval", tx: 0, rx: 0},
		{name: "receive only", tx: 0, rx: 1024},
		{name: "transmit only", tx: 2048, rx: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := recordObservation(app, node, time.Now().UTC(), 30, tt.tx, tt.rx)
			if err != nil {
				t.Fatalf("recordObservation(tx=%d, rx=%d): %v", tt.tx, tt.rx, err)
			}
		})
	}
}

func TestSpeedPerSecond(t *testing.T) {
	from := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	to := from.Add(30 * time.Second)

	if got := speedPerSecond(3000, from, to); got != 100 {
		t.Fatalf("speedPerSecond() = %d, want 100", got)
	}
}

func TestSpeedPerSecondWithoutPreviousPoll(t *testing.T) {
	if got := speedPerSecond(3000, time.Time{}, time.Now().UTC()); got != 0 {
		t.Fatalf("speedPerSecond() = %d, want 0", got)
	}
}

func TestRecordTrafficAggregatesLegacyAndStableNodeClientIDs(t *testing.T) {
	app := newMigratedCollectorTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	user := createCollectorTestUser(t, app, "mixed@example.com", "LegacySecret", "active")
	node := createCollectorTestNode(t, app, box, "http://127.0.0.1:9999")
	c := New(app, box)

	if err := c.recordTraffic(node, map[string]hysteria.TrafficEntry{
		"LegacySecret": {Tx: 100, Rx: 200},
		user.Id:        {Tx: 50, Rx: 75},
	}); err != nil {
		t.Fatalf("first recordTraffic: %v", err)
	}
	assertCollectorUserTotals(t, app, user.Id, 150, 275)

	node, err = app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if err := c.recordTraffic(node, map[string]hysteria.TrafficEntry{
		"LegacySecret": {Tx: 120, Rx: 230},
		user.Id:        {Tx: 70, Rx: 90},
	}); err != nil {
		t.Fatalf("second recordTraffic: %v", err)
	}
	assertCollectorUserTotals(t, app, user.Id, 190, 320)

	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "NextSecret")
		return err
	}); err != nil {
		t.Fatalf("rotate auth string: %v", err)
	}
	node, err = app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node after rotation: %v", err)
	}
	if err := c.recordTraffic(node, map[string]hysteria.TrafficEntry{
		"LegacySecret": {Tx: 130, Rx: 240},
		user.Id:        {Tx: 80, Rx: 100},
	}); err != nil {
		t.Fatalf("recordTraffic after rotation: %v", err)
	}
	assertCollectorUserTotals(t, app, user.Id, 210, 340)
}

func assertCollectorUserTotals(t *testing.T, app core.App, userID string, wantTx, wantRx int64) {
	t.Helper()
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got := int64(user.GetInt("used_tx")); got != wantTx {
		t.Fatalf("used_tx = %d, want %d", got, wantTx)
	}
	if got := int64(user.GetInt("used_rx")); got != wantRx {
		t.Fatalf("used_rx = %d, want %d", got, wantRx)
	}
}

func TestPollNodePersistsOnlineSnapshotWhenTrafficFails(t *testing.T) {
	app := newMigratedCollectorTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	user := createCollectorTestUser(t, app, "wang@example.com", "wang", "active")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/traffic":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/online":
			_, _ = fmt.Fprintf(w, `{"wang":2,%q:1,"orphan":3}`, user.Id)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	node := createCollectorTestNode(t, app, box, srv.URL)

	c := New(app, box)
	if err := c.pollNode(context.Background(), node); err == nil {
		t.Fatal("pollNode returned nil error for failed traffic request")
	}

	storedNode, err := app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if got := int64(storedNode.GetInt("online_devices")); got != 6 {
		t.Fatalf("node online_devices = %d, want 6", got)
	}
	if storedNode.GetDateTime("online_devices_observed_at").IsZero() {
		t.Fatal("online_devices_observed_at is zero")
	}
	count, err := app.FindFirstRecordByFilter(
		"online_device_counts",
		"user = {:user} && node = {:node}",
		map[string]any{"user": user.Id, "node": node.Id},
	)
	if err != nil {
		t.Fatalf("find online count: %v", err)
	}
	if got := int64(count.GetInt("count")); got != 3 {
		t.Fatalf("user online count = %d, want 3", got)
	}
}

func TestPollNodeKeepsOnlineSnapshotWhenOnlineFails(t *testing.T) {
	app := newMigratedCollectorTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	user := createCollectorTestUser(t, app, "wang@example.com", "wang", "active")
	onlineRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/traffic":
			_, _ = fmt.Fprint(w, `{}`)
		case "/online":
			onlineRequests++
			if onlineRequests == 1 {
				_, _ = fmt.Fprint(w, `{"wang":2}`)
				return
			}
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	node := createCollectorTestNode(t, app, box, srv.URL)
	c := New(app, box)

	if err := c.pollNode(context.Background(), node); err != nil {
		t.Fatalf("first pollNode: %v", err)
	}
	node, err = app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if err := c.pollNode(context.Background(), node); err == nil {
		t.Fatal("second pollNode returned nil error for failed online request")
	}

	storedNode, err := app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node after failure: %v", err)
	}
	if got := int64(storedNode.GetInt("online_devices")); got != 2 {
		t.Fatalf("node online_devices = %d, want retained value 2", got)
	}
	if got := storedNode.GetString("last_error"); got != "" {
		t.Fatalf("last_error = %q, want traffic health to remain clear", got)
	}
	counts := onlineCountsForNode(t, app, node.Id)
	if got := counts[user.Id]; got != 2 {
		t.Fatalf("user online count = %d, want retained value 2", got)
	}
}

func TestPollNodeReplacesOnlineSnapshotIncludingDisabledUsers(t *testing.T) {
	app := newMigratedCollectorTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	active := createCollectorTestUser(t, app, "wang@example.com", "wang", "active")
	disabled := createCollectorTestUser(t, app, "joe@example.com", "joe", "disabled")
	onlineResponses := []string{
		`{"wang":2,"joe":1,"orphan":3}`,
		`{"joe":4}`,
		`{}`,
	}
	onlineRequest := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/traffic":
			_, _ = fmt.Fprint(w, `{}`)
		case "/online":
			_, _ = fmt.Fprint(w, onlineResponses[onlineRequest])
			onlineRequest++
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	node := createCollectorTestNode(t, app, box, srv.URL)
	c := New(app, box)

	for i, wantTotal := range []int64{6, 4, 0} {
		if err := c.pollNode(context.Background(), node); err != nil {
			t.Fatalf("poll %d: %v", i+1, err)
		}
		node, err = app.FindRecordById("nodes", node.Id)
		if err != nil {
			t.Fatalf("reload node after poll %d: %v", i+1, err)
		}
		if got := int64(node.GetInt("online_devices")); got != wantTotal {
			t.Fatalf("poll %d node online_devices = %d, want %d", i+1, got, wantTotal)
		}

		counts := onlineCountsForNode(t, app, node.Id)
		switch i {
		case 0:
			if counts[active.Id] != 2 || counts[disabled.Id] != 1 || len(counts) != 2 {
				t.Fatalf("first snapshot = %v, want active=2 disabled=1", counts)
			}
		case 1:
			if counts[active.Id] != 0 || counts[disabled.Id] != 4 || len(counts) != 1 {
				t.Fatalf("second snapshot = %v, want only disabled=4", counts)
			}
		case 2:
			if len(counts) != 0 {
				t.Fatalf("empty snapshot left counts: %v", counts)
			}
		}
	}
}

func TestLateOnlineSnapshotDoesNotResurrectDisabledNodeCounts(t *testing.T) {
	app := newMigratedCollectorTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	user := createCollectorTestUser(t, app, "wang@example.com", "wang", "active")
	node := createCollectorTestNode(t, app, box, "http://127.0.0.1:9999")
	c := New(app, box)
	if err := c.recordOnlineSnapshot(node.Id, map[string]int64{"wang": 2}, time.Now().UTC()); err != nil {
		t.Fatalf("record initial snapshot: %v", err)
	}

	node, err = app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	node.Set("enabled", false)
	node.Set("online_devices", 0)
	node.Set("online_devices_observed_at", time.Now().UTC())
	if err := app.Save(node); err != nil {
		t.Fatalf("disable node: %v", err)
	}
	for _, count := range onlineCountRecordsForNode(t, app, node.Id) {
		if err := app.Delete(count); err != nil {
			t.Fatalf("clear count: %v", err)
		}
	}

	// This represents an HTTP result that was fetched before the disable but
	// reached persistence after the lifecycle change committed.
	if err := c.recordOnlineSnapshot(node.Id, map[string]int64{"wang": 7}, time.Now().UTC()); err != nil {
		t.Fatalf("record late snapshot: %v", err)
	}
	storedNode, err := app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload disabled node: %v", err)
	}
	if got := int64(storedNode.GetInt("online_devices")); got != 0 {
		t.Fatalf("disabled node online_devices = %d, want 0", got)
	}
	if counts := onlineCountsForNode(t, app, node.Id); len(counts) != 0 {
		t.Fatalf("late snapshot recreated counts for user %s: %v", user.Id, counts)
	}
}

func createCollectorTestUser(t *testing.T, app core.App, email, auth, status string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("correct horse battery staple")
	user.SetVerified(true)
	user.Set("role", "user")
	user.Set("status", status)
	if err := app.RunInTransaction(func(txApp core.App) error {
		if err := txApp.Save(user); err != nil {
			return err
		}
		_, err := authstrings.CreateCurrent(txApp, user.Id, auth)
		return err
	}); err != nil {
		t.Fatalf("save user with auth string: %v", err)
	}
	return user
}

func createCollectorTestNode(t *testing.T, app core.App, box *cryptobox.Box, apiURL string) *core.Record {
	t.Helper()
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes collection: %v", err)
	}
	secret, err := box.Encrypt("node-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "test node")
	node.Set("api_url", apiURL)
	node.Set("api_secret", secret)
	node.Set("poll_interval", 30)
	node.Set("enabled", true)
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return node
}

func onlineCountsForNode(t *testing.T, app core.App, nodeID string) map[string]int64 {
	t.Helper()
	records := onlineCountRecordsForNode(t, app, nodeID)
	out := make(map[string]int64, len(records))
	for _, record := range records {
		out[record.GetString("user")] = int64(record.GetInt("count"))
	}
	return out
}

func onlineCountRecordsForNode(t *testing.T, app core.App, nodeID string) []*core.Record {
	t.Helper()
	records, err := app.FindRecordsByFilter(
		"online_device_counts",
		"node = {:node}",
		"",
		0,
		0,
		map[string]any{"node": nodeID},
	)
	if err != nil {
		t.Fatalf("find online counts: %v", err)
	}
	return records
}
