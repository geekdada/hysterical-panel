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

var alnum = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func TestAlphanumericShape(t *testing.T) {
	for _, n := range []int{1, 16, 32, 100} {
		tok, err := Alphanumeric(n)
		if err != nil {
			t.Fatalf("Alphanumeric(%d) error: %v", n, err)
		}
		if len(tok) != n {
			t.Errorf("Alphanumeric(%d) length = %d, want %d", n, len(tok), n)
		}
		if !alnum.MatchString(tok) {
			t.Errorf("Alphanumeric(%d) = %q contains non-alphanumeric chars", n, tok)
		}
	}
}

func TestAlphanumericIsRandom(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok, err := Alphanumeric(16)
		if err != nil {
			t.Fatalf("Alphanumeric error: %v", err)
		}
		if seen[tok] {
			t.Fatalf("collision after %d iterations: %q", i, tok)
		}
		seen[tok] = true
	}
}

func TestAlphanumericRejectsNonPositive(t *testing.T) {
	if _, err := Alphanumeric(0); err == nil {
		t.Error("Alphanumeric(0) expected error, got nil")
	}
	if _, err := Alphanumeric(-1); err == nil {
		t.Error("Alphanumeric(-1) expected error, got nil")
	}
}

func TestSha256Hex(t *testing.T) {
	// Known vectors from the SHA-256 spec; must match hex(sha256(x)) exactly,
	// lowercase and 64 chars, since this is what anytls clients send.
	cases := map[string]string{
		"":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc":   "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"hello": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
	}
	for in, want := range cases {
		if got := Sha256Hex(in); got != want {
			t.Errorf("Sha256Hex(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Sha256Hex("abc"); len(got) != 64 {
		t.Errorf("Sha256Hex length = %d, want 64", len(got))
	}
}
