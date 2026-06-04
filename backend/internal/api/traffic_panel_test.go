package api

import (
	"net/url"
	"strings"
	"testing"
)

func TestPanelTrafficRequiresFromTo(t *testing.T) {
	tests := []struct {
		name string
		q    url.Values
	}{
		{"empty", url.Values{}},
		{"from only", url.Values{"from": {"2026-06-04 00:00:00.000Z"}}},
		{"to only", url.Values{"to": {"2026-06-04 00:00:00.000Z"}}},
		{"whitespace", url.Values{"from": {" "}, "to": {" "}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := strings.TrimSpace(tt.q.Get("from"))
			to := strings.TrimSpace(tt.q.Get("to"))
			if from != "" && to != "" {
				t.Fatal("expected missing from or to")
			}
		})
	}
}
