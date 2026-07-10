package monitoring

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
