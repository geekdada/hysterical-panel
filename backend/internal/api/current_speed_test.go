package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func TestUserCurrentSpeedListsSpeedingEnabledNodes(t *testing.T) {
	app := newMigratedTestApp(t)
	user := createOnlineTestUser(t, app, "user@example.com", "user-auth")
	fast := createOnlineTestNode(t, app, "fast", true, true, 0)
	idle := createOnlineTestNode(t, app, "idle", true, true, 0)
	disabled := createOnlineTestNode(t, app, "disabled", false, true, 0)
	createSpeedTestCursor(t, app, user.Id, fast.Id, 100, 200)
	createSpeedTestCursor(t, app, user.Id, idle.Id, 0, 0)
	createSpeedTestCursor(t, app, user.Id, disabled.Id, 500, 500)

	var body UserCurrentSpeedResponse
	callSpeedHandler(t, app, (&Handlers{app: app}).userCurrentSpeed, user.Id, &body)

	if body.Total != (ByteCount{Tx: 100, Rx: 200}) {
		t.Fatalf("total = %+v, want 100/200", body.Total)
	}
	if len(body.ByNode) != 1 || body.ByNode[0].Node.ID != fast.Id || body.ByNode[0].Tx != 100 || body.ByNode[0].Rx != 200 {
		t.Fatalf("by_node = %+v, want only the fast node", body.ByNode)
	}
}

func TestNodeCurrentSpeedReturnsTopTenUsers(t *testing.T) {
	app := newMigratedTestApp(t)
	node := createOnlineTestNode(t, app, "node", true, true, 0)
	node.Set("current_tx_speed", 999)
	node.Set("current_rx_speed", 888)
	if err := app.Save(node); err != nil {
		t.Fatal(err)
	}
	idle := createOnlineTestUser(t, app, "idle@example.com", "idle-auth")
	createSpeedTestCursor(t, app, idle.Id, node.Id, 0, 0)
	for i := 1; i <= 12; i++ {
		user := createOnlineTestUser(t, app, fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("auth-%d", i))
		createSpeedTestCursor(t, app, user.Id, node.Id, int64(i), int64(i))
	}

	var body NodeCurrentSpeedResponse
	callSpeedHandler(t, app, (&Handlers{app: app}).nodeCurrentSpeed, node.Id, &body)

	if body.Total != (ByteCount{Tx: 999, Rx: 888}) {
		t.Fatalf("total = %+v, want node fields 999/888", body.Total)
	}
	if len(body.TopUsers) != currentSpeedTopUsers {
		t.Fatalf("top_users length = %d, want %d", len(body.TopUsers), currentSpeedTopUsers)
	}
	if first, last := body.TopUsers[0], body.TopUsers[len(body.TopUsers)-1]; first.User.Email != "u12@example.com" || last.User.Email != "u3@example.com" {
		t.Fatalf("top_users order = %s .. %s, want u12 .. u3", first.User.Email, last.User.Email)
	}
}

func TestSaveNodeClearingProjectionsZeroesUserCurrentSpeed(t *testing.T) {
	app := newMigratedTestApp(t)
	user := createOnlineTestUser(t, app, "user@example.com", "user-auth")
	node := createOnlineTestNode(t, app, "node", true, true, 0)
	cursor := createSpeedTestCursor(t, app, user.Id, node.Id, 100, 200)
	node.Set("enabled", false)

	if err := (&Handlers{app: app}).saveNodeClearingProjections(node); err != nil {
		t.Fatalf("saveNodeClearingProjections: %v", err)
	}
	got, err := app.FindRecordById("traffic_cursor", cursor.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetInt("current_tx_speed") != 0 || got.GetInt("current_rx_speed") != 0 {
		t.Fatalf("cursor speed = %d/%d after disable, want 0/0", got.GetInt("current_tx_speed"), got.GetInt("current_rx_speed"))
	}
}

func createSpeedTestCursor(t *testing.T, app core.App, userID, nodeID string, txSpeed, rxSpeed int64) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("traffic_cursor")
	if err != nil {
		t.Fatalf("find traffic_cursor collection: %v", err)
	}
	record := core.NewRecord(collection)
	record.Set("user", userID)
	record.Set("node", nodeID)
	record.Set("current_tx_speed", txSpeed)
	record.Set("current_rx_speed", rxSpeed)
	if err := app.Save(record); err != nil {
		t.Fatalf("save cursor: %v", err)
	}
	return record
}

func callSpeedHandler(t *testing.T, app core.App, handler func(*core.RequestEvent) error, id string, out any) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	request.SetPathValue("id", id)
	event := &core.RequestEvent{App: app, Event: router.Event{Request: request, Response: response}}
	if err := handler(event); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if err := json.Unmarshal(response.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
