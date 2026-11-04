package automaticemails

// Condition is the allowance state of one User Subscription at one scan.
type Condition struct{ Low, Exhausted bool }

// Decide returns at most one Kind per transition. Exhausted wins over Low so a
// User who jumps straight to exhausted gets one email, not two.
func Decide(prev, cur Condition) []Kind {
	kinds := []Kind{}
	if cur.Exhausted && !prev.Exhausted {
		kinds = append(kinds, KindAllowanceExhausted)
	} else if cur.Low && !prev.Low {
		kinds = append(kinds, KindLowAllowance)
	}
	if !cur.Exhausted && prev.Exhausted {
		kinds = append(kinds, KindAllowanceRestored)
	}
	return kinds
}
