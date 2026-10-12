package oidc

import (
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
