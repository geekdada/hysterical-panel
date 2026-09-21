package api

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"hysterical-panel/internal/subscriptions"
)

func TestMeteredUserWithoutSubscriptionCannotAuthenticateNode(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "requires-grant@example.com", "RequiresGrantKey")
	user.Set("subscription_required", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	e, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/hysteria/auth", map[string]any{"auth": "RequiresGrantKey"})
	err := (&Handlers{app: app}).handleNodeClientAuth(e, "test", func(string) (*core.Record, error) { return user, nil })
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no available subscription") {
		t.Fatalf("node auth without a grant = %v", err)
	}
}

func TestExpiryReconciliationPreservesSeamlessRenewal(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	user := newUsersTestRecord(t, app, "renewal@example.com", "RenewalKey")
	user.Set("subscription_required", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Annual")
	typ.Set("allowance_bytes", 100)
	typ.Set("reset_days", 360)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	start := time.Now().UTC().Truncate(time.Millisecond).Add(-361 * 24 * time.Hour)
	for i := 0; i < 2; i++ {
		grant := core.NewRecord(grants)
		grant.Set("user", user.Id)
		grant.Set("subscription_type", typ.Id)
		grant.Set("starts_at", start.Add(time.Duration(i)*360*24*time.Hour))
		grant.Set("ends_at", start.Add(time.Duration(i+1)*360*24*time.Hour))
		if err := app.Save(grant); err != nil {
			t.Fatal(err)
		}
	}
	h.processSubscriptionExpiry()
	previous, err := app.FindFirstRecordByFilter("user_subscriptions", "user = {:u} && starts_at = {:s}", map[string]any{"u": user.Id, "s": start.Format("2006-01-02 15:04:05.000Z")})
	if err != nil || !previous.GetBool("expiry_processed") {
		t.Fatalf("expired grant not reconciled: %v", err)
	}
	allowed, err := subscriptions.Allowed(app, user, time.Now().UTC())
	if err != nil || !allowed {
		t.Fatalf("renewed user access = %t, err = %v", allowed, err)
	}
}

func TestFirstGrantEndsLegacyExemptionAndQueuesOneRenewal(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "legacy-subscription@example.com", "LegacySubscriptionKey")
	h := &Handlers{app: app}
	typeEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/subscription-types", map[string]any{
		"name": "Monthly", "allowance_bytes": 100, "reset_days": 30,
	})
	if err := h.createSubscriptionType(typeEvent); err != nil {
		t.Fatal(err)
	}
	types, err := app.FindRecordsByFilter("subscription_types", "name = 'Monthly'", "", 1, 0)
	if err != nil || len(types) != 1 {
		t.Fatalf("types = %d, err = %v", len(types), err)
	}

	grantEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions", map[string]any{"subscription_type": types[0].Id})
	grantEvent.Request.SetPathValue("id", user.Id)
	if err := h.grantSubscription(grantEvent); err != nil {
		t.Fatal(err)
	}
	stored, err := app.FindRecordById("users", user.Id)
	if err != nil || !stored.GetBool("subscription_required") {
		t.Fatalf("first grant did not end exemption: %v", err)
	}
	state, err := subscriptions.Current(app, user.Id, stored.GetDateTime("created").Time().Add(24*60*60*1e9))
	if err != nil || state == nil {
		t.Fatalf("current grant = %v, err = %v", state, err)
	}

	secondEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions", map[string]any{"subscription_type": types[0].Id})
	secondEvent.Request.SetPathValue("id", user.Id)
	if err := h.grantSubscription(secondEvent); err != nil {
		t.Fatal(err)
	}
	grants, err := app.FindRecordsByFilter("user_subscriptions", "user = {:u}", "starts_at", 0, 0, map[string]any{"u": user.Id})
	if err != nil || len(grants) != 2 {
		t.Fatalf("grants = %d, err = %v", len(grants), err)
	}
	if !grants[1].GetDateTime("starts_at").Time().Equal(grants[0].GetDateTime("ends_at").Time()) {
		t.Fatal("renewal does not start at current grant's end")
	}
	thirdEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions", map[string]any{"subscription_type": types[0].Id})
	thirdEvent.Request.SetPathValue("id", user.Id)
	if err := h.grantSubscription(thirdEvent); err == nil {
		t.Fatal("third future grant was accepted")
	}
}

func TestConcurrentGrantsNeverCreateMoreThanOneQueuedSubscription(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "concurrent-grants@example.com", "ConcurrentGrantKey")
	h := &Handlers{app: app}
	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Monthly")
	typ.Set("allowance_bytes", 100)
	typ.Set("reset_days", 30)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 3; i++ {
		e, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions", map[string]any{"subscription_type": typ.Id})
		e.Request.SetPathValue("id", user.Id)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = h.grantSubscription(e)
		}()
	}
	close(start)
	wg.Wait()
	grants, err := app.FindRecordsByFilter("user_subscriptions", "user = {:u} && terminated_at = ''", "", 0, 0, map[string]any{"u": user.Id})
	if err != nil || len(grants) < 1 || len(grants) > 2 {
		t.Fatalf("concurrent grants = %d, err = %v; want one current and at most one queued", len(grants), err)
	}
	current, queued := 0, 0
	now := time.Now().UTC()
	for _, grant := range grants {
		if now.Before(grant.GetDateTime("starts_at").Time()) {
			queued++
		} else {
			current++
		}
	}
	if current != 1 || queued > 1 {
		t.Fatalf("concurrent grants: %d current, %d queued", current, queued)
	}
}

func TestTypeEditReplacesCurrentTopUpsWithoutChangingUsed(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "adjusted@example.com", "AdjustedKey")
	h := &Handlers{app: app}
	typeEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/subscription-types", map[string]any{"name": "Starter", "allowance_bytes": 100, "reset_days": 30})
	if err := h.createSubscriptionType(typeEvent); err != nil {
		t.Fatal(err)
	}
	types, _ := app.FindRecordsByFilter("subscription_types", "", "", 1, 0)
	grantEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions", map[string]any{"subscription_type": types[0].Id})
	grantEvent.Request.SetPathValue("id", user.Id)
	if err := h.grantSubscription(grantEvent); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindRecordsByFilter("user_subscriptions", "user = {:u}", "", 1, 0, map[string]any{"u": user.Id})
	for i := 0; i < 2; i++ {
		e, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions/"+grants[0].Id+"/top-up", map[string]any{"allowance_bytes": 100})
		e.Request.SetPathValue("id", user.Id)
		e.Request.SetPathValue("subscriptionId", grants[0].Id)
		if err := h.topUpSubscription(e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := subscriptions.AddUsage(app, user.Id, time.Now().UTC(), 120); err != nil {
		t.Fatal(err)
	}
	state, err := subscriptions.Current(app, user.Id, time.Now().UTC())
	if err != nil || state == nil || state.Allowance != 300 {
		t.Fatalf("before edit = %v, err = %v", state, err)
	}
	patch, _ := jsonRequestEvent(t, app, http.MethodPatch, "/api/panel/subscription-types/"+types[0].Id, map[string]any{"allowance_bytes": 200})
	patch.Request.SetPathValue("id", types[0].Id)
	if err := h.updateSubscriptionType(patch); err != nil {
		t.Fatal(err)
	}
	state, err = subscriptions.Current(app, user.Id, time.Now().UTC())
	if err != nil || state == nil || state.Allowance != 200 || state.Used != 120 {
		t.Fatalf("after edit = %v, err = %v; want allowance 200 used 120", state, err)
	}
	topUp, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions/"+grants[0].Id+"/top-up", map[string]any{"allowance_bytes": 200})
	topUp.Request.SetPathValue("id", user.Id)
	topUp.Request.SetPathValue("subscriptionId", grants[0].Id)
	if err := h.topUpSubscription(topUp); err != nil {
		t.Fatal(err)
	}
	state, err = subscriptions.Current(app, user.Id, time.Now().UTC())
	if err != nil || state == nil || state.Allowance != 400 || state.Used != 120 {
		t.Fatalf("after new top-up = %v, err = %v; want allowance 400 used 120", state, err)
	}
	unchanged, _ := jsonRequestEvent(t, app, http.MethodPatch, "/api/panel/subscription-types/"+types[0].Id, map[string]any{"name": "Renamed", "allowance_bytes": 200})
	unchanged.Request.SetPathValue("id", types[0].Id)
	if err := h.updateSubscriptionType(unchanged); err != nil {
		t.Fatal(err)
	}
	state, err = subscriptions.Current(app, user.Id, time.Now().UTC())
	if err != nil || state == nil || state.Allowance != 400 {
		t.Fatalf("no-op allowance edit cleared top-up: %v, err = %v", state, err)
	}
	changeReset, _ := jsonRequestEvent(t, app, http.MethodPatch, "/api/panel/subscription-types/"+types[0].Id, map[string]any{"reset_days": 360})
	changeReset.Request.SetPathValue("id", types[0].Id)
	if err := h.updateSubscriptionType(changeReset); err == nil {
		t.Fatal("assigned type reset interval changed")
	}
}

func TestTopUpAddsRequestedBytes(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "topup@example.com", "TopUpKey")
	h := &Handlers{app: app}
	typeEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/subscription-types", map[string]any{"name": "Starter", "allowance_bytes": 100, "reset_days": 30})
	if err := h.createSubscriptionType(typeEvent); err != nil {
		t.Fatal(err)
	}
	types, _ := app.FindRecordsByFilter("subscription_types", "", "", 1, 0)
	grantEvent, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions", map[string]any{"subscription_type": types[0].Id})
	grantEvent.Request.SetPathValue("id", user.Id)
	if err := h.grantSubscription(grantEvent); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindRecordsByFilter("user_subscriptions", "user = {:u}", "", 1, 0, map[string]any{"u": user.Id})
	topUp := func(bytes any) error {
		event, _ := jsonRequestEvent(t, app, http.MethodPost, "/api/panel/users/"+user.Id+"/subscriptions/"+grants[0].Id+"/top-up", map[string]any{"allowance_bytes": bytes})
		event.Request.SetPathValue("id", user.Id)
		event.Request.SetPathValue("subscriptionId", grants[0].Id)
		return h.topUpSubscription(event)
	}
	if err := topUp(40); err != nil {
		t.Fatal(err)
	}
	state, err := subscriptions.Current(app, user.Id, time.Now().UTC())
	if err != nil || state == nil || state.Allowance != 140 || state.Used != 0 {
		t.Fatalf("after top-up = %v, err = %v; want allowance 140", state, err)
	}
	if err := topUp(0); err == nil {
		t.Fatal("zero top-up accepted")
	}
	if err := topUp(-5); err == nil {
		t.Fatal("negative top-up accepted")
	}
	state, err = subscriptions.Current(app, user.Id, time.Now().UTC())
	if err != nil || state == nil || state.Allowance != 140 {
		t.Fatalf("rejected top-up changed allowance: %v, err = %v", state, err)
	}
}

func TestTerminatedSubscriptionExposesActualEndTime(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "terminated-grant@example.com", "TerminatedGrantKey")
	types, _ := app.FindCollectionByNameOrId("subscription_types")
	typ := core.NewRecord(types)
	typ.Set("name", "Annual")
	typ.Set("allowance_bytes", 100)
	typ.Set("reset_days", 360)
	if err := app.Save(typ); err != nil {
		t.Fatal(err)
	}
	grants, _ := app.FindCollectionByNameOrId("user_subscriptions")
	grant := core.NewRecord(grants)
	start := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)
	grant.Set("user", user.Id)
	grant.Set("subscription_type", typ.Id)
	grant.Set("starts_at", start)
	grant.Set("ends_at", start.Add(360*24*time.Hour))
	grant.Set("terminated_at", time.Now().UTC())
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	view, err := grantView(app, grant, time.Now().UTC())
	if err != nil || view.Status != "terminated" || view.TerminatedAt == "" {
		t.Fatalf("terminated grant view = %+v, err = %v", view, err)
	}
}
