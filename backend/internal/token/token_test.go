package token

import (
	"regexp"
	"testing"
)

var urlSafe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func TestNewIsURLSafe(t *testing.T) {
	for _, n := range []int{16, 18, 24, 32} {
		tok, err := New(n)
		if err != nil {
			t.Fatalf("New(%d) error: %v", n, err)
		}
		if !urlSafe.MatchString(tok) {
			t.Errorf("New(%d) = %q is not URL-safe", n, tok)
		}
		// base64 raw-url length is ceil(n*4/3).
		want := (n*8 + 5) / 6
		if len(tok) != want {
			t.Errorf("New(%d) length = %d, want %d", n, len(tok), want)
		}
	}
}

func TestNewIsRandom(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok, err := New(18)
		if err != nil {
			t.Fatalf("New error: %v", err)
		}
		if seen[tok] {
			t.Fatalf("collision after %d iterations: %q", i, tok)
		}
		seen[tok] = true
	}
}

func TestNewRejectsNonPositive(t *testing.T) {
	if _, err := New(0); err == nil {
		t.Error("New(0) expected error, got nil")
	}
	if _, err := New(-1); err == nil {
		t.Error("New(-1) expected error, got nil")
	}
}
