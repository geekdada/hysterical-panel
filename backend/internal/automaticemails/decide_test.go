package automaticemails

import (
	"reflect"
	"testing"
)

func TestDecide(t *testing.T) {
	cases := []struct {
		name      string
		prev, cur Condition
		want      []Kind
	}{
		{"becomes low", Condition{}, Condition{Low: true}, []Kind{KindLowAllowance}},
		{"jumps to exhausted", Condition{}, Condition{Low: true, Exhausted: true}, []Kind{KindAllowanceExhausted}},
		{"low to exhausted", Condition{Low: true}, Condition{Low: true, Exhausted: true}, []Kind{KindAllowanceExhausted}},
		{"restored but still low", Condition{Low: true, Exhausted: true}, Condition{Low: true}, []Kind{KindAllowanceRestored}},
		{"restored fully", Condition{Low: true, Exhausted: true}, Condition{}, []Kind{KindAllowanceRestored}},
		{"low recovers silently", Condition{Low: true}, Condition{}, []Kind{}},
		{"unchanged low", Condition{Low: true}, Condition{Low: true}, []Kind{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.prev, tc.cur); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Decide(%+v, %+v) = %v, want %v", tc.prev, tc.cur, got, tc.want)
			}
		})
	}
}
