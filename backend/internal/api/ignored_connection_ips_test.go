package api

import "testing"

func TestNormalizeIgnoredConnectionIPRequest(t *testing.T) {
	tests := []struct {
		in  string
		ok  bool
		out string
	}{
		{in: "8.8.8.8", ok: true, out: "8.8.8.8"},
		{in: "  2001:db8::1  ", ok: true, out: "2001:db8::1"},
		{in: "bad", ok: false},
		{in: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := normalizeStoredConnectionIP(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.out {
				t.Fatalf("ip = %q, want %q", got, tt.out)
			}
		})
	}
}
