package api

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/token"
)

func TestGetUserReturnsOnlineDeviceSumAcrossEnabledNodes(t *testing.T) {
	app := newMigratedTestApp(t)
	user := createOnlineTestUser(t, app, "user@example.com", "user-auth")
	first := createOnlineTestNode(t, app, "first", true, true, 2)
	second := createOnlineTestNode(t, app, "second", true, true, 3)
	disabled := createOnlineTestNode(t, app, "disabled", false, true, 9)
	createOnlineTestCount(t, app, user.Id, first.Id, 2)
	createOnlineTestCount(t, app, user.Id, second.Id, 3)
	createOnlineTestCount(t, app, user.Id, disabled.Id, 9)

	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/panel/users/"+user.Id, nil)
	request.SetPathValue("id", user.Id)
	event := &core.RequestEvent{App: app, Event: router.Event{Request: request, Response: response}}
	if err := (&Handlers{app: app}).getUser(event); err != nil {
		t.Fatalf("getUser: %v", err)
	}

	var body struct {
		OnlineDevices *int64 `json:"online_devices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.OnlineDevices == nil || *body.OnlineDevices != 5 {
		t.Fatalf("online_devices = %v, want 5", body.OnlineDevices)
	}
}

func TestOnlineDevicesForUserDistinguishesZeroFromNoObservation(t *testing.T) {
	app := newMigratedTestApp(t)
	user := createOnlineTestUser(t, app, "user@example.com", "user-auth")
	h := &Handlers{app: app}

	zero, err := h.onlineDevicesForUser(user.Id)
	if err != nil {
		t.Fatalf("no nodes: %v", err)
	}
	if zero == nil || *zero != 0 {
		t.Fatalf("no nodes = %v, want known zero", zero)
	}

	createOnlineTestNode(t, app, "never observed", true, false, 0)
	unknown, err := h.onlineDevicesForUser(user.Id)
	if err != nil {
		t.Fatalf("never observed: %v", err)
	}
	if unknown != nil {
		t.Fatalf("never observed = %d, want nil", *unknown)
	}

	createOnlineTestNode(t, app, "observed empty", true, true, 0)
	knownZero, err := h.onlineDevicesForUser(user.Id)
	if err != nil {
		t.Fatalf("observed empty: %v", err)
	}
	if knownZero == nil || *knownZero != 0 {
		t.Fatalf("observed empty = %v, want known zero", knownZero)
	}
}

func TestSaveNodeClearsOnlineDeviceProjection(t *testing.T) {
	app := newMigratedTestApp(t)
	user := createOnlineTestUser(t, app, "user@example.com", "user-auth")
	node := createOnlineTestNode(t, app, "node", true, true, 2)
	createOnlineTestCount(t, app, user.Id, node.Id, 2)
	node.Set("enabled", false)
	node.Set("online_devices", 0)
	node.Set("online_devices_observed_at", time.Now().UTC())

	if err := (&Handlers{app: app}).saveNodeClearingOnlineProjection(node); err != nil {
		t.Fatalf("saveNodeClearingOnlineProjection: %v", err)
	}
	counts, err := app.FindRecordsByFilter(
		"online_device_counts",
		"node = {:node}",
		"",
		0,
		0,
		map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatalf("find counts: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("online counts remain after disable: %d", len(counts))
	}
	got, err := app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	public := publicNode(got)
	if public["online_devices"] != int64(0) {
		t.Fatalf("public online_devices = %#v, want 0", public["online_devices"])
	}
}

func TestOpenAPIMovesOnlineDevicesOutOfLiveResponses(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("BuildOpenAPISpec: %v", err)
	}
	for _, name := range []string{"LiveResponse", "NodeLiveResponse"} {
		schema := spec.Components.Schemas[name]
		if schema == nil || schema.Value == nil {
			t.Fatalf("missing schema %s", name)
		}
		if _, exists := schema.Value.Properties["online_devices"]; exists {
			t.Fatalf("%s still exposes online_devices", name)
		}
	}
	for _, value := range []any{LiveNodeResult{}, NodeLiveUserResult{}} {
		if _, exists := reflect.TypeOf(value).FieldByName("OnlineDevices"); exists {
			t.Fatalf("%T still exposes OnlineDevices", value)
		}
	}
	if got := spec.Paths.Find("/api/panel/users/{id}").Get.Responses.Value("200").Value.Content.Get("application/json").Schema.Ref; got != "#/components/schemas/UserDetail" {
		t.Fatalf("get user response schema = %q, want UserDetail", got)
	}
}

func createOnlineTestUser(t *testing.T, app core.App, email, auth string) *core.Record {
	t.Helper()
	user := newUsersTestRecord(t, app, email, auth)
	user.Set("auth_string_anytls_hash", token.Sha256Hex(auth))
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

func createOnlineTestNode(t *testing.T, app core.App, name string, enabled, observed bool, total int64) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes collection: %v", err)
	}
	node := core.NewRecord(collection)
	node.Set("name", name)
	node.Set("api_url", "http://127.0.0.1:9999")
	node.Set("api_secret", "encrypted")
	node.Set("poll_interval", 30)
	node.Set("enabled", enabled)
	node.Set("online_devices", total)
	if observed {
		node.Set("online_devices_observed_at", time.Now().UTC())
	}
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return node
}

func createOnlineTestCount(t *testing.T, app core.App, userID, nodeID string, count int64) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("online_device_counts")
	if err != nil {
		t.Fatalf("find online counts collection: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("user", userID)
	record.Set("node", nodeID)
	record.Set("count", count)
	if err := app.Save(record); err != nil {
		t.Fatalf("save online count: %v", err)
	}
	return record
}
