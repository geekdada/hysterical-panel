package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func linkOAuthIdentity(t *testing.T, app core.App, user *core.Record, provider, providerID string) *core.ExternalAuth {
	t.Helper()
	link := core.NewExternalAuth(app)
	link.SetCollectionRef(user.Collection().Id)
	link.SetRecordRef(user.Id)
	link.SetProvider(provider)
	link.SetProviderId(providerID)
	if err := app.Save(link); err != nil {
		t.Fatalf("save oauth link: %v", err)
	}
	return link
}

func TestListOAuthLinksReturnsOnlyTheUsersLinks(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	owner := newUsersTestRecord(t, app, "owner@example.com", "OwnerSecret1234")
	other := newUsersTestRecord(t, app, "other@example.com", "OtherSecret1234")
	linkOAuthIdentity(t, app, owner, "gitlab", "owner-gitlab")
	linkOAuthIdentity(t, app, other, "github", "other-github")

	event, response := jsonRequestEvent(t, app, http.MethodGet, "/api/panel/users/"+owner.Id+"/oauth-links", nil)
	event.Request.SetPathValue("id", owner.Id)
	if err := h.listOAuthLinks(event); err != nil {
		t.Fatalf("listOAuthLinks: %v", err)
	}
	var links []OAuthLink
	if err := json.Unmarshal(response.Body.Bytes(), &links); err != nil {
		t.Fatalf("decode links: %v", err)
	}
	if len(links) != 1 || links[0].Provider != "gitlab" || links[0].Created == "" {
		t.Fatalf("links = %+v, want the owner's gitlab link", links)
	}
}

func TestDeleteOAuthLinkRequiresTheLinkToBelongToTheUser(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	owner := newUsersTestRecord(t, app, "owner@example.com", "OwnerSecret1234")
	other := newUsersTestRecord(t, app, "other@example.com", "OtherSecret1234")
	link := linkOAuthIdentity(t, app, owner, "gitlab", "owner-gitlab")

	deleteAs := func(userID string) error {
		event, _ := jsonRequestEvent(t, app, http.MethodDelete, "/api/panel/users/"+userID+"/oauth-links/"+link.Id, nil)
		event.Request.SetPathValue("id", userID)
		event.Request.SetPathValue("linkId", link.Id)
		return h.deleteOAuthLink(event)
	}

	var apiErr *router.ApiError
	if err := deleteAs(other.Id); !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("delete through another user err = %v, want 404", err)
	}
	if _, err := app.FindRecordById(core.CollectionNameExternalAuths, link.Id); err != nil {
		t.Fatalf("link removed through another user's path: %v", err)
	}

	if err := deleteAs(owner.Id); err != nil {
		t.Fatalf("delete own link: %v", err)
	}
	if _, err := app.FindRecordById(core.CollectionNameExternalAuths, link.Id); err == nil {
		t.Fatal("link still exists after delete")
	}
}
