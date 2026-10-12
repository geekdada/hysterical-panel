package oidc

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestRevokeUserGrantsLeavesOtherUsers(t *testing.T) {
	f := newRefreshFixture(t)
	bystander := newTestUser(t, f.app, "bystander@example.com")
	kept := exchangedTokens(t, f.app, f.box, f.client, bystander, offlineScope)

	if err := RevokeUserGrants(f.app, f.user.Id, RevokeReasonUserDisabled, exchangedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	grants, err := f.app.FindAllRecords(GrantsCollection)
	if err != nil || len(grants) != 2 {
		t.Fatalf("grants = %d, %v", len(grants), err)
	}
	for _, grant := range grants {
		revoked := !grant.GetDateTime("revoked_at").IsZero()
		if mine := grant.GetString("user") == f.user.Id; revoked != mine ||
			(mine && grant.GetString("revoke_reason") != RevokeReasonUserDisabled) {
			t.Errorf("grant of %s = %v", grant.GetString("user"), grant.PublicExport())
		}
	}
	if _, _, err := LookupAccessToken(f.app, kept.AccessToken, exchangedAt); err != nil {
		t.Fatalf("the bystander lost access: %v", err)
	}
}

func TestListAndRevokeActiveGrants(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "grants@example.com")
	other := newTestUser(t, app, "other-grants@example.com")
	grant, err := upsertGrant(app, user.Id, client.Id, []string{ScopeOpenID, ScopeEmail})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	otherGrant, err := upsertGrant(app, other.Id, client.Id, []string{ScopeOpenID})
	if err != nil {
		t.Fatalf("other grant: %v", err)
	}

	listed, err := ListActiveGrants(app, user.Id)
	if err != nil || len(listed) != 1 || listed[0].ID != grant.Id || listed[0].ClientName != client.GetString("name") ||
		!slices.Equal(listed[0].Scopes, []string{ScopeOpenID, ScopeEmail}) || listed[0].Created.IsZero() || !listed[0].LastUsedAt.IsZero() {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	if err := RevokeGrant(app, user.Id, otherGrant.Id, testNow); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("revoking another user's grant: %v", err)
	}
	if err := RevokeGrant(app, user.Id, grant.Id, testNow); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := RevokeGrant(app, user.Id, grant.Id, testNow); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
	stored, err := app.FindRecordById(GrantsCollection, grant.Id)
	if err != nil || stored.GetString("revoke_reason") != RevokeReasonUserRevoked || stored.GetDateTime("revoked_at").IsZero() {
		t.Fatalf("stored grant = %v, %v", stored, err)
	}
	if listed, err := ListActiveGrants(app, user.Id); err != nil || len(listed) != 0 {
		t.Fatalf("after revoke = %+v, %v", listed, err)
	}
	if listed, err := ListActiveGrants(app, other.Id); err != nil || len(listed) != 1 {
		t.Fatalf("other user's grants = %+v, %v", listed, err)
	}
}
