// Package automaticemails holds the pure rules for Automatic Emails: which
// Kinds fire on a condition change and how templates are validated and rendered.
package automaticemails

type Kind string

const (
	KindLowAllowance       Kind = "low_allowance"
	KindAllowanceExhausted Kind = "allowance_exhausted"
	KindAllowanceRestored  Kind = "allowance_restored"
)

var Kinds = []Kind{KindLowAllowance, KindAllowanceExhausted, KindAllowanceRestored}

func ParseKind(s string) (Kind, bool) {
	for _, kind := range Kinds {
		if string(kind) == s {
			return kind, true
		}
	}
	return "", false
}
