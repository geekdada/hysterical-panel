package collector

import (
	"testing"
	"time"
)

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
