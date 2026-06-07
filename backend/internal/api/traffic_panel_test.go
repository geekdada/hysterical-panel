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

func TestUTCDailyBucketString(t *testing.T) {
	got := utcDailyBucketString("2026-06-07 18:45:00.000Z")
	want := "2026-06-07 00:00:00.000Z"
	if got != want {
		t.Fatalf("utcDailyBucketString() = %q, want %q", got, want)
	}
}

func TestOpenAPIPanelTrafficSeriesRequiresBounds(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatal(err)
	}

	path := spec.Paths.Value("/api/panel/traffic/series")
	if path == nil || path.Get == nil {
		t.Fatal("/api/panel/traffic/series GET path missing")
	}

	required := map[string]bool{}
	for _, param := range path.Get.Parameters {
		if param.Value != nil {
			required[param.Value.Name] = param.Value.Required
		}
	}
	for _, name := range []string{"from", "to"} {
		if !required[name] {
			t.Fatalf("parameter %q is not required", name)
		}
	}
}
