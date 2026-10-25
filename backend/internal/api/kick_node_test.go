package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

type fakeNodeKicker struct {
	online    map[string]int64
	onlineErr error
	kicked    []string
	kickCalls int
}

func (f *fakeNodeKicker) Online(context.Context) (map[string]int64, error) {
	return f.online, f.onlineErr
}

func (f *fakeNodeKicker) Kick(_ context.Context, ids []string) error {
	f.kickCalls++
	f.kicked = append(f.kicked, ids...)
	return nil
}

func TestKickOnlineClientsKicksEveryOnlineID(t *testing.T) {
	cl := &fakeNodeKicker{online: map[string]int64{"user-a": 2, "user-b": 1}}
	n, err := kickOnlineClients(context.Background(), cl)
	if err != nil {
		t.Fatalf("kickOnlineClients: %v", err)
	}
	slices.Sort(cl.kicked)
	if n != 2 || cl.kickCalls != 1 || !slices.Equal(cl.kicked, []string{"user-a", "user-b"}) {
		t.Fatalf("kicked %d ids %v in %d calls", n, cl.kicked, cl.kickCalls)
	}
}

func TestKickOnlineClientsSkipsKickWhenNobodyIsOnline(t *testing.T) {
	cl := &fakeNodeKicker{online: map[string]int64{}}
	if n, err := kickOnlineClients(context.Background(), cl); err != nil || n != 0 {
		t.Fatalf("kickOnlineClients = %d, %v", n, err)
	}
	if cl.kickCalls != 0 {
		t.Fatalf("kick called %d times with nobody online", cl.kickCalls)
	}
}

func TestKickOnlineClientsReportsOnlineFailure(t *testing.T) {
	cl := &fakeNodeKicker{onlineErr: errors.New("boom")}
	if _, err := kickOnlineClients(context.Background(), cl); err == nil {
		t.Fatal("kickOnlineClients succeeded despite /online failure")
	}
	if cl.kickCalls != 0 {
		t.Fatalf("kick called %d times after /online failed", cl.kickCalls)
	}
}

// TestDisablingNodeKicksOnlineClients covers the PATCH path: only the
// enabled -> disabled transition reaches the node's /kick.
func TestDisablingNodeKicksOnlineClients(t *testing.T) {
	app, h := newMgmtNodeTestHandlers(t)
	kicks := make(chan []string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/online":
			_ = json.NewEncoder(w).Encode(map[string]int64{"user-a": 1, "user-b": 3})
		case "/kick":
			var ids []string
			_ = json.NewDecoder(r.Body).Decode(&ids)
			kicks <- ids
		}
	}))
	t.Cleanup(server.Close)

	body := mgmtNodeCall(t, app, h.createNode, http.MethodPost, "", map[string]any{
		"name": "edge", "api_url": server.URL, "api_secret": "node-secret",
	}, http.StatusCreated)
	id := body["id"].(string)

	mgmtNodeCall(t, app, h.updateNode, http.MethodPatch, id, map[string]any{"enabled": false}, http.StatusOK)
	select {
	case ids := <-kicks:
		slices.Sort(ids)
		if !slices.Equal(ids, []string{"user-a", "user-b"}) {
			t.Fatalf("kicked ids = %v", ids)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disabling the node never reached /kick")
	}

	mgmtNodeCall(t, app, h.updateNode, http.MethodPatch, id, map[string]any{"enabled": false}, http.StatusOK)
	mgmtNodeCall(t, app, h.updateNode, http.MethodPatch, id, map[string]any{"name": "renamed"}, http.StatusOK)
	select {
	case ids := <-kicks:
		t.Fatalf("already-disabled node kicked again: %v", ids)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestUpdateNodeRejectsBlankName(t *testing.T) {
	app, h := newMgmtNodeTestHandlers(t)
	body := mgmtNodeCall(t, app, h.createNode, http.MethodPost, "", map[string]any{
		"name": "edge", "api_url": "https://node.example.com:9999", "api_secret": "node-secret",
	}, http.StatusCreated)
	id := body["id"].(string)

	mgmtNodeCall(t, app, h.updateNode, http.MethodPatch, id, map[string]any{"name": "  "}, http.StatusBadRequest)
	body = mgmtNodeCall(t, app, h.updateNode, http.MethodPatch, id, map[string]any{"name": "renamed"}, http.StatusOK)
	if body["name"] != "renamed" {
		t.Fatalf("rename body = %v", body)
	}
}
