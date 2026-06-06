package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrafficRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 34, 56, 789000000, time.FixedZone("test", 2*60*60))
	got := trafficRetentionCutoff(now)
	want := "2026-05-07 10:34:56.000Z"
	if got != want {
		t.Fatalf("trafficRetentionCutoff() = %q, want %q", got, want)
	}
}

func TestDatabaseStorageStats(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.db"), []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.db-wal"), []byte{1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := databaseStorageStats(dir)
	if err != nil {
		t.Fatal(err)
	}

	if stats.TotalBytes != 5 {
		t.Fatalf("TotalBytes = %d, want 5", stats.TotalBytes)
	}
	if len(stats.Files) != 3 {
		t.Fatalf("len(Files) = %d, want 3", len(stats.Files))
	}

	sizes := map[string]int64{}
	for _, file := range stats.Files {
		sizes[file.Name] = file.Bytes
	}
	if sizes["data.db"] != 3 || sizes["data.db-wal"] != 2 || sizes["data.db-shm"] != 0 {
		t.Fatalf("sizes = %#v", sizes)
	}
}
