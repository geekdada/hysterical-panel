package collector

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

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
