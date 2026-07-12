package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/authstrings"
	"hysterical-panel/internal/cryptobox"
)

func TestLiveMergesLegacyAndStableNodeClientIDs(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "live-mixed@example.com", "LegacySecret")
	if err := app.RunInTransaction(func(txApp core.App) error {
		_, err := authstrings.Rotate(txApp, user.Id, "CurrentSecret")
		return err
	}); err != nil {
		t.Fatalf("rotate auth string: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dump/streams" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"streams":[
			{"auth":"LegacySecret","connection":1,"stream":1,"tx":10,"rx":20},
			{"auth":%q,"connection":2,"stream":1,"tx":30,"rx":40}
		]}`, user.Id)
	}))
	defer server.Close()

	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes: %v", err)
	}
	secret, err := box.Encrypt("node-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "mixed node")
	node.Set("api_url", server.URL)
	node.Set("api_secret", secret)
	node.Set("enabled", true)
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}
	h := &Handlers{app: app, box: box}

	userRequest := httptest.NewRequest("GET", "/api/panel/users/"+user.Id+"/live", nil)
	userRequest.SetPathValue("id", user.Id)
	userResponse := httptest.NewRecorder()
	userEvent := &core.RequestEvent{App: app, Event: router.Event{Request: userRequest, Response: userResponse}}
	if err := h.userLive(userEvent); err != nil {
		t.Fatalf("userLive: %v", err)
	}
	var userBody struct {
		ActiveStreams int `json:"active_streams"`
	}
	if err := json.Unmarshal(userResponse.Body.Bytes(), &userBody); err != nil {
		t.Fatalf("decode user live: %v", err)
	}
	if userBody.ActiveStreams != 2 {
		t.Fatalf("user active_streams = %d, want 2", userBody.ActiveStreams)
	}

	nodeRequest := httptest.NewRequest("GET", "/api/panel/nodes/"+node.Id+"/live", nil)
	nodeRequest.SetPathValue("id", node.Id)
	nodeResponse := httptest.NewRecorder()
	nodeEvent := &core.RequestEvent{App: app, Event: router.Event{Request: nodeRequest, Response: nodeResponse}}
	if err := h.nodeLive(nodeEvent); err != nil {
		t.Fatalf("nodeLive: %v", err)
	}
	var nodeBody struct {
		ByUser []struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			Streams []json.RawMessage `json:"streams"`
		} `json:"by_user"`
	}
	if err := json.Unmarshal(nodeResponse.Body.Bytes(), &nodeBody); err != nil {
		t.Fatalf("decode node live: %v", err)
	}
	if len(nodeBody.ByUser) != 1 || nodeBody.ByUser[0].User.ID != user.Id || len(nodeBody.ByUser[0].Streams) != 2 {
		t.Fatalf("node by_user = %+v, want one merged User with two streams", nodeBody.ByUser)
	}
}
