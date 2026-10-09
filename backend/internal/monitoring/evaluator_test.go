package monitoring

import (
	"math"
	"testing"
	"time"
)

func TestWeightedAverage(t *testing.T) {
	points := []Observation{{TxBytes: 300, RxBytes: 300, ElapsedSeconds: 10}, {TxBytes: 600, RxBytes: 600, ElapsedSeconds: 30}}
	avg, ok := WeightedAverage(points)
	if !ok || avg != 45 {
		t.Fatalf("WeightedAverage() = %d, %v; want 45, true", avg, ok)
	}
}

func TestWeightedAverageNoData(t *testing.T) {
	if _, ok := WeightedAverage(nil); ok {
		t.Fatal("WeightedAverage(nil) should be unknown")
	}
}

func TestHighTrafficState(t *testing.T) {
	cases := []struct {
		name      string
		firing    bool
		average   int64
		threshold int64
		want      Decision
	}{
		{"open above threshold", false, 101, 100, DecisionFire},
		{"stay idle at threshold", false, 100, 100, DecisionKeep},
		{"hold in hysteresis", true, 95, 100, DecisionKeep},
		{"resolve at ninety percent", true, 90, 100, DecisionResolve},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideHighTraffic(tc.firing, tc.average, tc.threshold); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecideCondition(t *testing.T) {
	cases := []struct {
		name   string
		firing bool
		met    bool
		want   Decision
	}{
		{"open when met", false, true, DecisionFire},
		{"stay idle when clear", false, false, DecisionKeep},
		{"hold while met", true, true, DecisionKeep},
		{"resolve when cleared", true, false, DecisionResolve},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideCondition(tc.firing, tc.met); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsLowAllowance(t *testing.T) {
	cases := []struct {
		name      string
		remaining int64
		allowance int64
		percent   int64
		want      bool
	}{
		{"exactly at threshold is not low", 10, 100, 10, false},
		{"one byte below threshold is low", 9, 100, 10, true},
		{"rounds the threshold up", 10, 101, 10, true},
		{"exhausted is low", 0, 100, 10, true},
		{"over allowance is low", -5, 100, 10, true},
		{"top-up restores", 105, 200, 10, false},
		{"huge allowance does not overflow", math.MaxInt64 / 10, math.MaxInt64, 10, true},
		{"huge allowance above threshold", math.MaxInt64/10 + 1, math.MaxInt64, 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLowAllowance(tc.remaining, tc.allowance, tc.percent); got != tc.want {
				t.Fatalf("IsLowAllowance(%d, %d, %d) = %v, want %v", tc.remaining, tc.allowance, tc.percent, got, tc.want)
			}
		})
	}
}

func TestIsExpiring(t *testing.T) {
	week := 7 * 24 * time.Hour
	if IsExpiring(week, 7, false) {
		t.Fatal("exactly seven days left should not be expiring")
	}
	if !IsExpiring(week-time.Second, 7, false) {
		t.Fatal("under seven days left should be expiring")
	}
	if IsExpiring(time.Hour, 7, true) {
		t.Fatal("a queued successor should prevent expiring")
	}
}
