package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

type mgmtSubscriptionFixture struct {
	app  core.App
	h    *Handlers
	user *core.Record
	typ  *core.Record
}

func TestManagementAPISubscriptionTypesListOnlyVisibleFields(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	newMgmtSubscriptionType(t, app, "Monthly", false)
	annual := newMgmtSubscriptionType(t, app, "Annual", false)
	newMgmtSubscriptionType(t, app, "Hidden", true)

	event, response := jsonRequestEvent(t, app, http.MethodGet, "/api/mgmt/subscription-types", nil)
	requireMgmtSubscriptionStatus(t, h.mgmtListSubscriptionTypes(event), response, http.StatusOK)

	var body []map[string]any
	decodeMgmtSubscriptionResponse(t, response, &body)
	if len(body) != 2 || body[0]["id"] != annual.Id {
		t.Fatalf("visible types = %v, want Annual then Monthly", body)
	}
	for _, item := range body {
		if len(item) != 4 || item["name"] == "" || item["allowance_bytes"] != float64(100) || item["reset_days"] != float64(30) {
			t.Fatalf("type fields = %v, want exactly id, name, allowance_bytes and reset_days", item)
		}
	}
}

func TestManagementAPISubscriptionGrantQueuesAndEndsLegacyExemption(t *testing.T) {
	fixture := newMgmtSubscriptionFixture(t, "mgmt-grant@example.com")
	first := fixture.grant(t, fixture.typ.Id, http.StatusOK)
	if first.Status != "current" {
		t.Fatalf("first grant status = %q, want current", first.Status)
	}
	storedUser, err := fixture.app.FindRecordById("users", fixture.user.Id)
	if err != nil || !storedUser.GetBool("subscription_required") {
		t.Fatalf("first grant did not end legacy exemption: %v", err)
	}

	second := fixture.grant(t, fixture.typ.Id, http.StatusOK)
	if second.Status != "queued" || second.StartsAt != first.EndsAt {
		t.Fatalf("queued grant = %+v, want start at current end %q", second, first.EndsAt)
	}
	fixture.grant(t, fixture.typ.Id, http.StatusConflict)

	// The 409 mapping belongs to Management API, not the panel contract.
	fixture.grantWith(t, fixture.h.grantSubscription, fixture.typ.Id, http.StatusBadRequest)
}

func TestManagementAPISubscriptionGrantRejectsHiddenType(t *testing.T) {
	fixture := newMgmtSubscriptionFixture(t, "mgmt-hidden-type@example.com")
	hidden := newMgmtSubscriptionType(t, fixture.app, "Hidden", true)
	fixture.grant(t, hidden.Id, http.StatusBadRequest)
}

func TestManagementAPISubscriptionListsFullHistory(t *testing.T) {
	fixture := newMgmtSubscriptionFixture(t, "mgmt-history@example.com")
	now := time.Now().UTC().Truncate(time.Millisecond)
	collection, err := fixture.app.FindCollectionByNameOrId("user_subscriptions")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		status     string
		startsAt   time.Time
		terminated bool
	}{
		{status: "expired", startsAt: now.Add(-720 * 24 * time.Hour)},
		{status: "terminated", startsAt: now.Add(-30 * 24 * time.Hour), terminated: true},
		{status: "current", startsAt: now.Add(-24 * time.Hour)},
		{status: "queued", startsAt: now.Add(359 * 24 * time.Hour)},
	}
	for _, tc := range cases {
		grant := core.NewRecord(collection)
		grant.Set("user", fixture.user.Id)
		grant.Set("subscription_type", fixture.typ.Id)
		grant.Set("starts_at", tc.startsAt)
		grant.Set("ends_at", tc.startsAt.Add(360*24*time.Hour))
		if tc.terminated {
			grant.Set("terminated_at", now.Add(-time.Hour))
		}
		if err := fixture.app.Save(grant); err != nil {
			t.Fatalf("save %s grant: %v", tc.status, err)
		}
	}

	event, response := jsonRequestEvent(t, fixture.app, http.MethodGet, "/api/mgmt/users/"+fixture.user.Id+"/subscriptions", nil)
	event.Request.SetPathValue("id", fixture.user.Id)
	requireMgmtSubscriptionStatus(t, fixture.h.listUserSubscriptions(event), response, http.StatusOK)
	var body []UserSubscription
	decodeMgmtSubscriptionResponse(t, response, &body)
	if len(body) != len(cases) {
		t.Fatalf("history length = %d, want %d", len(body), len(cases))
	}
	for i, want := range []string{"queued", "current", "terminated", "expired"} {
		if body[i].Status != want {
			t.Fatalf("history[%d].status = %q, want %q", i, body[i].Status, want)
		}
	}

	missing, missingResponse := jsonRequestEvent(t, fixture.app, http.MethodGet, "/api/mgmt/users/missing/subscriptions", nil)
	missing.Request.SetPathValue("id", "missing")
	requireMgmtSubscriptionStatus(t, fixture.h.listUserSubscriptions(missing), missingResponse, http.StatusNotFound)
}

func TestManagementAPISubscriptionTopUpRetriesAndStateRules(t *testing.T) {
	fixture := newMgmtSubscriptionFixture(t, "mgmt-top-up@example.com")
	current := fixture.grant(t, fixture.typ.Id, http.StatusOK)
	updated := fixture.topUp(t, fixture.user.Id, current.ID, 40, http.StatusOK)
	if updated.AllowanceBytes != 140 || updated.UsedBytes != 0 {
		t.Fatalf("first top-up = %+v, want allowance 140 and unchanged usage", updated)
	}
	updated = fixture.topUp(t, fixture.user.Id, current.ID, 40, http.StatusOK)
	if updated.AllowanceBytes != 180 {
		t.Fatalf("repeated top-up allowance = %d, want 180", updated.AllowanceBytes)
	}
	fixture.topUp(t, fixture.user.Id, current.ID, 0, http.StatusBadRequest)
	fixture.topUp(t, fixture.user.Id, current.ID, -5, http.StatusBadRequest)

	queued := fixture.grant(t, fixture.typ.Id, http.StatusOK)
	fixture.topUp(t, fixture.user.Id, queued.ID, 10, http.StatusConflict)
	other := newUsersTestRecord(t, fixture.app, "mgmt-other@example.com", "MgmtOtherKey")
	fixture.topUp(t, other.Id, current.ID, 10, http.StatusNotFound)
	fixture.topUp(t, fixture.user.Id, "missing", 10, http.StatusNotFound)
}

func TestManagementAPISubscriptionTerminatesCurrentWithKick(t *testing.T) {
	fixture := newMgmtSubscriptionFixture(t, "mgmt-terminate@example.com")
	received := startKickTargetNode(t, fixture.app, fixture.h.box)
	current := fixture.grant(t, fixture.typ.Id, http.StatusOK)

	event, response := mgmtSubscriptionEvent(t, fixture, http.MethodDelete, fixture.user.Id, current.ID, nil)
	requireMgmtSubscriptionStatus(t, fixture.h.mgmtTerminateSubscription(event), response, http.StatusOK)
	var deleted DeleteResponse
	decodeMgmtSubscriptionResponse(t, response, &deleted)
	if !deleted.Deleted {
		t.Fatal("terminate response did not confirm deletion")
	}
	expectKick(t, received, fixture.user.Id)
	stored, err := fixture.app.FindRecordById("user_subscriptions", current.ID)
	if err != nil || stored.GetDateTime("terminated_at").IsZero() {
		t.Fatalf("terminated history was not preserved: %v", err)
	}

	repeated, repeatedResponse := mgmtSubscriptionEvent(t, fixture, http.MethodDelete, fixture.user.Id, current.ID, nil)
	requireMgmtSubscriptionStatus(t, fixture.h.mgmtTerminateSubscription(repeated), repeatedResponse, http.StatusConflict)
}

func TestManagementAPISubscriptionCancelsQueuedWithoutKick(t *testing.T) {
	fixture := newMgmtSubscriptionFixture(t, "mgmt-cancel@example.com")
	received := startKickTargetNode(t, fixture.app, fixture.h.box)
	fixture.grant(t, fixture.typ.Id, http.StatusOK)
	queued := fixture.grant(t, fixture.typ.Id, http.StatusOK)

	event, response := mgmtSubscriptionEvent(t, fixture, http.MethodDelete, fixture.user.Id, queued.ID, nil)
	requireMgmtSubscriptionStatus(t, fixture.h.mgmtTerminateSubscription(event), response, http.StatusOK)
	select {
	case ids := <-received:
		t.Fatalf("cancelling queued grant kicked %v", ids)
	case <-time.After(100 * time.Millisecond):
	}
	stored, err := fixture.app.FindRecordById("user_subscriptions", queued.ID)
	if err != nil || stored.GetDateTime("terminated_at").IsZero() {
		t.Fatalf("cancelled history was not preserved: %v", err)
	}
}

func newMgmtSubscriptionFixture(t *testing.T, email string) *mgmtSubscriptionFixture {
	t.Helper()
	app := newMigratedTestApp(t)
	box, err := cryptobox.New("test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	return &mgmtSubscriptionFixture{
		app:  app,
		h:    &Handlers{app: app, box: box},
		user: newUsersTestRecord(t, app, email, email+"-auth-key"),
		typ:  newMgmtSubscriptionType(t, app, "Monthly", false),
	}
}

func newMgmtSubscriptionType(t *testing.T, app core.App, name string, hidden bool) *core.Record {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("subscription_types")
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	record.Set("name", name)
	record.Set("allowance_bytes", 100)
	record.Set("reset_days", 30)
	record.Set("hidden", hidden)
	if err := app.Save(record); err != nil {
		t.Fatalf("save subscription type: %v", err)
	}
	return record
}

func (f *mgmtSubscriptionFixture) grant(t *testing.T, typeID string, want int) UserSubscription {
	t.Helper()
	return f.grantWith(t, f.h.mgmtGrantSubscription, typeID, want)
}

func (f *mgmtSubscriptionFixture) grantWith(t *testing.T, handler func(*core.RequestEvent) error, typeID string, want int) UserSubscription {
	t.Helper()
	event, response := mgmtSubscriptionEvent(t, f, http.MethodPost, f.user.Id, "", map[string]any{"subscription_type": typeID})
	requireMgmtSubscriptionStatus(t, handler(event), response, want)
	var body UserSubscription
	if want == http.StatusOK {
		decodeMgmtSubscriptionResponse(t, response, &body)
	}
	return body
}

func (f *mgmtSubscriptionFixture) topUp(t *testing.T, userID, subscriptionID string, amount int64, want int) UserSubscription {
	t.Helper()
	event, response := mgmtSubscriptionEvent(t, f, http.MethodPost, userID, subscriptionID, map[string]any{"allowance_bytes": amount})
	event.Request.URL.Path += "/top-up"
	requireMgmtSubscriptionStatus(t, f.h.mgmtTopUpSubscription(event), response, want)
	var body UserSubscription
	if want == http.StatusOK {
		decodeMgmtSubscriptionResponse(t, response, &body)
	}
	return body
}

func mgmtSubscriptionEvent(t *testing.T, fixture *mgmtSubscriptionFixture, method, userID, subscriptionID string, body any) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	event, response := jsonRequestEvent(t, fixture.app, method, "/api/mgmt/users/"+userID+"/subscriptions/"+subscriptionID, body)
	event.Request.SetPathValue("id", userID)
	event.Request.SetPathValue("subscriptionId", subscriptionID)
	return event, response
}

func requireMgmtSubscriptionStatus(t *testing.T, err error, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	status := mgmtStatus(t, err)
	if err == nil {
		status = response.Code
	}
	if status != want {
		t.Fatalf("status = %d, want %d (body %s, err %v)", status, want, response.Body.String(), err)
	}
}

func decodeMgmtSubscriptionResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, response.Body.String())
	}
}
