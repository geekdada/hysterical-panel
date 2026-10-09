package monitoring

import "time"

type Observation struct {
	TxBytes        int64
	RxBytes        int64
	ElapsedSeconds int64
}

type Decision string

const (
	DecisionKeep    Decision = "keep"
	DecisionFire    Decision = "fire"
	DecisionResolve Decision = "resolve"
)

func WeightedAverage(points []Observation) (int64, bool) {
	var bytes, seconds int64
	for _, point := range points {
		if point.ElapsedSeconds <= 0 {
			continue
		}
		bytes += point.TxBytes + point.RxBytes
		seconds += point.ElapsedSeconds
	}
	if seconds == 0 {
		return 0, false
	}
	return bytes / seconds, true
}

func DecideHighTraffic(firing bool, average, threshold int64) Decision {
	if !firing && average > threshold {
		return DecisionFire
	}
	if firing && average <= threshold*9/10 {
		return DecisionResolve
	}
	return DecisionKeep
}

const (
	KindLowAllowance         = "low_allowance"
	KindExpiringSubscription = "expiring_subscription"
)

// IsUserKind reports whether a Monitor kind evaluates Users rather than Nodes.
func IsUserKind(kind string) bool {
	return kind == KindLowAllowance || kind == KindExpiringSubscription
}

// DecideCondition opens an Alert when the condition starts and resolves it
// when the condition clears. User Monitor values only grow back through
// admin actions or a new window, so they need no hysteresis.
func DecideCondition(firing, met bool) Decision {
	if !firing && met {
		return DecisionFire
	}
	if firing && !met {
		return DecisionResolve
	}
	return DecisionKeep
}

// IsLowAllowance reports whether remaining is below percent of allowance.
// The threshold is rounded up so the comparison matches exact arithmetic
// without overflowing on large allowances.
func IsLowAllowance(remaining, allowance, percent int64) bool {
	threshold := allowance/100*percent + (allowance%100*percent+99)/100
	return remaining < threshold
}

// IsExpiring reports whether a grant with no queued successor ends within days.
func IsExpiring(remaining time.Duration, days int64, queued bool) bool {
	return !queued && remaining < time.Duration(days)*24*time.Hour
}
