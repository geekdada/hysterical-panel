package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

func TestManagementAPIGateHidesDisabledFeatureAndRejectsBadTokens(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	gate := h.requireMgmtToken()

	call := func(authorization string) int {
		event, _ := jsonRequestEvent(t, app, http.MethodGet, "/api/mgmt/nodes", nil)
		if authorization != "" {
			event.Request.Header.Set("Authorization", authorization)
		}
		return mgmtStatus(t, gate.Func(event))
	}

	if status := call("Bearer good-token"); status != http.StatusNotFound {
		t.Fatalf("disabled feature status = %d, want 404", status)
	}
	settings, err := h.settingsRecord()
	if err != nil {
		t.Fatalf("settingsRecord: %v", err)
	}
	settings.Set("management_api_enabled", true)
	settings.Set("management_api_token_hash", hashManagementAPIToken("good-token"))
	if err := app.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	cases := map[string]int{
		"":                  http.StatusUnauthorized,
		"Bearer wrong":      http.StatusUnauthorized,
		"Bearer good-token": http.StatusOK,
	}
	for authorization, want := range cases {
		if status := call(authorization); status != want {
			t.Fatalf("Authorization %q: status = %d, want %d", authorization, status, want)
		}
	}
}

func TestManagementAPINodeLifecycle(t *testing.T) {
	app, h := newMgmtNodeTestHandlers(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	body := mgmtNodeCall(t, app, h.createNode, http.MethodPost, "", map[string]any{
		"name": "edge", "api_url": server.URL, "api_secret": "node-secret",
	}, http.StatusCreated)
	id, _ := body["id"].(string)
	if id == "" || body["enabled"] != true {
		t.Fatalf("create body = %v", body)
	}
	if _, leaked := body["api_secret"]; leaked {
		t.Fatalf("create response leaked api_secret: %v", body)
	}
	mgmtNodeCall(t, app, h.getNode, http.MethodGet, id, nil, http.StatusOK)

	node, err := app.FindRecordById("nodes", id)
	if err != nil {
		t.Fatalf("find node: %v", err)
	}
	node.Set("current_tx_speed", 100)
	node.Set("online_devices", 3)
	if err := app.Save(node); err != nil {
		t.Fatalf("seed node projections: %v", err)
	}

	body = mgmtNodeCall(t, app, h.mgmtUpdateNode, http.MethodPatch, id, map[string]any{"enabled": false}, http.StatusOK)
	if body["enabled"] != false || body["current_tx_speed"] != float64(0) || body["online_devices"] != float64(0) {
		t.Fatalf("disable body = %v", body)
	}
	body = mgmtNodeCall(t, app, h.testNode, http.MethodPost, id, nil, http.StatusOK)
	if body["ok"] != true {
		t.Fatalf("test on disabled node = %v", body)
	}
	body = mgmtNodeCall(t, app, h.mgmtUpdateNode, http.MethodPatch, id, map[string]any{"enabled": true}, http.StatusOK)
	if body["enabled"] != true {
		t.Fatalf("enable body = %v", body)
	}

	mgmtNodeCall(t, app, h.deleteNode, http.MethodDelete, id, nil, http.StatusOK)
	mgmtNodeCall(t, app, h.getNode, http.MethodGet, id, nil, http.StatusNotFound)
	mgmtNodeCall(t, app, h.mgmtUpdateNode, http.MethodPatch, id, map[string]any{"enabled": true}, http.StatusNotFound)
	mgmtNodeCall(t, app, h.deleteNode, http.MethodDelete, id, nil, http.StatusNotFound)
	mgmtNodeCall(t, app, h.testNode, http.MethodPost, id, nil, http.StatusNotFound)
}

// TestManagementAPINodeURLChangeRequiresSecret covers ADR-0011. The Collector
// sends the stored secret to the node's api_url, so moving a node must replace it.
func TestManagementAPINodeURLChangeRequiresSecret(t *testing.T) {
	app, h := newMgmtNodeTestHandlers(t)
	body := mgmtNodeCall(t, app, h.createNode, http.MethodPost, "", map[string]any{
		"name": "edge", "api_url": "https://node.example.com:9999", "api_secret": "old-secret",
	}, http.StatusCreated)
	id := body["id"].(string)

	mgmtNodeCall(t, app, h.mgmtUpdateNode, http.MethodPatch, id, map[string]any{
		"api_url": "https://attacker.example.com",
	}, http.StatusBadRequest)
	mgmtNodeCall(t, app, h.mgmtUpdateNode, http.MethodPatch, id, map[string]any{
		"name": "renamed", "api_url": "https://node.example.com:9999",
	}, http.StatusOK)
	body = mgmtNodeCall(t, app, h.mgmtUpdateNode, http.MethodPatch, id, map[string]any{
		"api_url": "https://moved.example.com", "api_secret": "new-secret",
	}, http.StatusOK)
	if body["api_url"] != "https://moved.example.com" || body["name"] != "renamed" {
		t.Fatalf("moved body = %v", body)
	}

	node, err := app.FindRecordById("nodes", id)
	if err != nil {
		t.Fatalf("find node: %v", err)
	}
	secret, err := h.box.Decrypt(node.GetString("api_secret"))
	if err != nil || secret != "new-secret" {
		t.Fatalf("stored secret = %q, err = %v", secret, err)
	}
}

func newMgmtNodeTestHandlers(t *testing.T) (core.App, *Handlers) {
	t.Helper()
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatalf("cryptobox.New: %v", err)
	}
	return app, &Handlers{app: app, box: box}
}

func mgmtNodeCall(t *testing.T, app core.App, handler func(*core.RequestEvent) error, method, id string, body any, want int) map[string]any {
	t.Helper()
	event, res := jsonRequestEvent(t, app, method, "/api/mgmt/nodes/"+id, body)
	event.Request.SetPathValue("id", id)
	err := handler(event)
	status := mgmtStatus(t, err)
	if err == nil {
		status = res.Code
	}
	if status != want {
		t.Fatalf("%s %s: status = %d, want %d (body %s)", method, id, status, want, res.Body.String())
	}
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s response: %v", method, err)
	}
	return out
}
