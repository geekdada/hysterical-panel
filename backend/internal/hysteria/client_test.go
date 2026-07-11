package hysteria

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKickSuccess(t *testing.T) {
	var gotAuth string
	var gotBody []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/kick" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = append(gotBody, string(b))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "topsecret", time.Second)
	if err := c.Kick(context.Background(), []string{"wang", "joe"}); err != nil {
		t.Fatalf("Kick returned error: %v", err)
	}

	if gotAuth != "topsecret" {
		t.Fatalf("Authorization header = %q, want topsecret", gotAuth)
	}
	if len(gotBody) != 1 {
		t.Fatalf("got %d bodies, want 1", len(gotBody))
	}
	var ids []string
	if err := json.Unmarshal([]byte(gotBody[0]), &ids); err != nil {
		t.Fatalf("body is not a JSON string array: %q (%v)", gotBody[0], err)
	}
	if len(ids) != 2 || ids[0] != "wang" || ids[1] != "joe" {
		t.Fatalf("ids = %v, want [wang joe]", ids)
	}
}

func TestTrafficTreatsMissingDirectionAsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/traffic" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"receive-only":{"rx":1024},"transmit-only":{"tx":2048},"idle":{}}`)
	}))
	defer srv.Close()

	traffic, err := New(srv.URL, "secret", time.Second).Traffic(context.Background())
	if err != nil {
		t.Fatalf("Traffic returned error: %v", err)
	}
	if got := traffic["receive-only"]; got.Tx != 0 || got.Rx != 1024 {
		t.Fatalf("receive-only = %+v, want tx=0 rx=1024", got)
	}
	if got := traffic["transmit-only"]; got.Tx != 2048 || got.Rx != 0 {
		t.Fatalf("transmit-only = %+v, want tx=2048 rx=0", got)
	}
	if got := traffic["idle"]; got.Tx != 0 || got.Rx != 0 {
		t.Fatalf("idle = %+v, want tx=0 rx=0", got)
	}
}

func TestKickNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", time.Second)
	err := c.Kick(context.Background(), []string{"wang"})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v, want a 503 error", err)
	}
}

func TestKickEmptyListStillPosts(t *testing.T) {
	var sawRequest bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sawRequest = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", time.Second)
	if err := c.Kick(context.Background(), nil); err != nil {
		t.Fatalf("Kick(nil) returned error: %v", err)
	}
	if !sawRequest {
		t.Fatalf("expected POST /kick even for an empty id list")
	}
}
