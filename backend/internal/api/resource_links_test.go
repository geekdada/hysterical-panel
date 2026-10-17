package api

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

var resourceLinkTestIcon = []byte("\x89PNG\r\n\x1a\nresource-link-icon")

func newResourceLinkAdmin(t *testing.T, app core.App) *core.Record {
	t.Helper()
	admin := newUsersTestRecord(t, app, "resource-admin@example.com", "AdminSecret123")
	admin.Set("role", "admin")
	if err := app.Save(admin); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	return admin
}

// newResourceLinkPageServer serves a page with a title and a linked PNG icon.
func newResourceLinkPageServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><head><title>Docs</title><link rel="icon" href="/icon.png"></head><body></body></html>`)
	})
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(resourceLinkTestIcon)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func closedResourceLinkServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return url
}

func createResourceLinkRaw(t *testing.T, h *Handlers, admin *core.Record, body map[string]any) (ManagedResourceLink, string) {
	t.Helper()
	e, rec := panelEvent(t, h.app, http.MethodPost, "/api/panel/resource-links", "", body, admin)
	if err := h.createResourceLink(e); err != nil {
		t.Fatalf("create resource link: %v", err)
	}
	return decodeBody[ManagedResourceLink](t, rec), rec.Body.String()
}

func createResourceLink(t *testing.T, h *Handlers, admin *core.Record, body map[string]any) ManagedResourceLink {
	t.Helper()
	created, _ := createResourceLinkRaw(t, h, admin, body)
	return created
}

func TestResourceLinkDisplayTitle(t *testing.T) {
	cases := []struct{ manual, fetched, rawURL, want string }{
		{"Mine", "Fetched", "https://a.example/x", "Mine"},
		{"", "Fetched", "https://a.example/x", "Fetched"},
		{"", "", "https://a.example/x", "a.example"},
	}
	for _, c := range cases {
		if got := resourceLinkDisplayTitle(c.manual, c.fetched, c.rawURL); got != c.want {
			t.Errorf("display title(%q, %q, %q) = %q, want %q", c.manual, c.fetched, c.rawURL, got, c.want)
		}
	}
}

func TestNormalizeResourceLinkURL(t *testing.T) {
	if got, err := normalizeResourceLinkURL(" https://a.example/p?q=1 "); err != nil || got != "https://a.example/p?q=1" {
		t.Fatalf("normalized = %q, err = %v, want the trimmed URL", got, err)
	}

	prefix := "https://a.example/"
	longURL := prefix + strings.Repeat("a", 2049-len(prefix))
	if len(longURL) != 2049 {
		t.Fatalf("test URL is %d bytes, want 2049", len(longURL))
	}
	for _, raw := range []string{"javascript:alert(1)", "ftp://a.example", "https://", "", longURL} {
		if _, err := normalizeResourceLinkURL(raw); err == nil {
			t.Errorf("normalizeResourceLinkURL(%q) accepted an invalid URL", raw)
		}
	}
}

func TestCreateResourceLinkExtractsMetadataAndAppends(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	page := newResourceLinkPageServer(t)

	first, firstBody := createResourceLinkRaw(t, h, admin, map[string]any{"url": page.URL + "/"})
	second, secondBody := createResourceLinkRaw(t, h, admin, map[string]any{"url": page.URL + "/other"})
	if first.FetchedTitle != "Docs" || first.Title != "Docs" || first.FaviconVersion == "" {
		t.Fatalf("first = %+v", first)
	}

	firstRec, err := app.FindRecordById("resource_links", first.ID)
	if err != nil {
		t.Fatalf("find first: %v", err)
	}
	secondRec, err := app.FindRecordById("resource_links", second.ID)
	if err != nil {
		t.Fatalf("find second: %v", err)
	}
	if secondRec.GetInt("sort_order") <= firstRec.GetInt("sort_order") {
		t.Fatalf("sort_order = %d then %d, want the second higher", firstRec.GetInt("sort_order"), secondRec.GetInt("sort_order"))
	}
	icon, err := base64.StdEncoding.DecodeString(firstRec.GetString("favicon_data"))
	if err != nil || !bytes.Equal(icon, resourceLinkTestIcon) {
		t.Fatalf("stored favicon = %q, err = %v", firstRec.GetString("favicon_data"), err)
	}
	for _, body := range []string{firstBody, secondBody} {
		if strings.Contains(body, firstRec.GetString("favicon_data")) {
			t.Fatalf("response leaked the stored favicon: %s", body)
		}
	}
}

func TestCreateResourceLinkSavesWhenFetchFails(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	deadURL := closedResourceLinkServerURL(t)

	created := createResourceLink(t, h, admin, map[string]any{"url": deadURL})
	parsed, err := url.Parse(deadURL)
	if err != nil {
		t.Fatalf("parse dead URL: %v", err)
	}
	if created.Title != parsed.Hostname() || created.FetchedTitle != "" || created.FaviconVersion != "" {
		t.Fatalf("created = %+v", created)
	}
}

func TestUpdateResourceLinkRefetchesOnlyOnURLChange(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	page := newResourceLinkPageServer(t)
	created := createResourceLink(t, h, admin, map[string]any{"url": page.URL})
	if created.FetchedTitle != "Docs" || created.FaviconVersion == "" {
		t.Fatalf("created = %+v", created)
	}
	page.Close()

	e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/resource-links/"+created.ID, created.ID,
		map[string]any{"title": "Mine"}, admin)
	if err := h.updateResourceLink(e); err != nil {
		t.Fatalf("update title: %v", err)
	}
	updated := decodeBody[ManagedResourceLink](t, rec)
	if updated.ManualTitle != "Mine" || updated.Title != "Mine" || updated.FetchedTitle != "Docs" || updated.FaviconVersion == "" {
		t.Fatalf("title-only update = %+v", updated)
	}

	deadURL := closedResourceLinkServerURL(t)
	e, rec = panelEvent(t, app, http.MethodPatch, "/api/panel/resource-links/"+created.ID, created.ID,
		map[string]any{"url": deadURL}, admin)
	if err := h.updateResourceLink(e); err != nil {
		t.Fatalf("update URL: %v", err)
	}
	updated = decodeBody[ManagedResourceLink](t, rec)
	if updated.FetchedTitle != "" || updated.FaviconVersion != "" || updated.Title != "Mine" {
		t.Fatalf("URL update = %+v", updated)
	}
}

func TestListResourceLinksHidesAdminFields(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	user := newUsersTestRecord(t, app, "resource-user@example.com", "UserSecret123")
	h := &Handlers{app: app}
	page := newResourceLinkPageServer(t)
	first := createResourceLink(t, h, admin, map[string]any{"url": page.URL + "/first"})
	second := createResourceLink(t, h, admin, map[string]any{"url": page.URL + "/second"})

	e, rec := panelEvent(t, app, http.MethodGet, "/api/panel/resource-links", "", nil, user)
	if err := h.listResourceLinks(e); err != nil {
		t.Fatalf("list: %v", err)
	}
	body := rec.Body.String()
	host, err := url.Parse(page.URL)
	if err != nil {
		t.Fatalf("parse page URL: %v", err)
	}
	for _, forbidden := range []string{`"url"`, "visit_count", host.Host} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list leaked %q: %s", forbidden, body)
		}
	}
	list := decodeBody[[]ResourceLink](t, rec)
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestReorderResourceLinksRewritesOrder(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	a := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	b := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	c := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	want := []string{c.ID, a.ID, b.ID}

	e, rec := panelEvent(t, app, http.MethodPut, "/api/panel/resource-links/order", "",
		map[string]any{"ids": want}, admin)
	if err := h.reorderResourceLinks(e); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	ordered := decodeBody[[]ManagedResourceLink](t, rec)
	if len(ordered) != len(want) {
		t.Fatalf("reorder response = %+v", ordered)
	}
	for i, id := range want {
		if ordered[i].ID != id {
			t.Fatalf("reorder response[%d] = %q, want %q", i, ordered[i].ID, id)
		}
		stored, err := app.FindRecordById("resource_links", id)
		if err != nil {
			t.Fatalf("find %s: %v", id, err)
		}
		if stored.GetInt("sort_order") != i {
			t.Fatalf("%s sort_order = %d, want %d", id, stored.GetInt("sort_order"), i)
		}
	}

	e, rec = panelEvent(t, app, http.MethodGet, "/api/panel/resource-links", "", nil, admin)
	if err := h.listResourceLinks(e); err != nil {
		t.Fatalf("list after reorder: %v", err)
	}
	list := decodeBody[[]ResourceLink](t, rec)
	for i, id := range want {
		if list[i].ID != id {
			t.Fatalf("listed[%d] = %q, want %q", i, list[i].ID, id)
		}
	}
}

func TestReorderResourceLinksRejectsMismatchedSet(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	a := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	b := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	c := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})

	for _, ids := range [][]string{
		{a.ID, b.ID},
		{a.ID, b.ID, c.ID, "missing"},
		{a.ID, a.ID, b.ID},
	} {
		e, _ := panelEvent(t, app, http.MethodPut, "/api/panel/resource-links/order", "",
			map[string]any{"ids": ids}, admin)
		assertAPIError(t, h.reorderResourceLinks(e), http.StatusConflict, "resource link order is out of date")
	}

	for i, id := range []string{a.ID, b.ID, c.ID} {
		stored, err := app.FindRecordById("resource_links", id)
		if err != nil {
			t.Fatalf("find %s: %v", id, err)
		}
		if stored.GetInt("sort_order") != i {
			t.Fatalf("%s sort_order = %d after rejected reorders, want %d", id, stored.GetInt("sort_order"), i)
		}
	}
}

// The handlers above are called directly, so this goes through the real
// router to prove the route-level guards. Task 3 extends this table with the
// public visit and favicon routes.
func TestResourceLinkRoutesGuards(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "resource-guard@example.com", "GuardSecret123")
	sessionToken, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	box, _ := cryptobox.New("resource-link-test-master-key")
	Register(&core.ServeEvent{App: app, Router: r}, app, box, nil, nil, nil, nil, PanelConfigResponse{})
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("mux: %v", err)
	}

	routes := []struct {
		method, path string
		adminOnly    bool
	}{
		{http.MethodGet, "/api/panel/resource-links", false},
		{http.MethodGet, "/api/panel/resource-links/manage", true},
		{http.MethodPost, "/api/panel/resource-links", true},
		{http.MethodPut, "/api/panel/resource-links/order", true},
		{http.MethodPatch, "/api/panel/resource-links/any", true},
		{http.MethodDelete, "/api/panel/resource-links/any", true},
	}
	for _, route := range routes {
		want := http.StatusForbidden
		if !route.adminOnly {
			want = http.StatusOK
		}
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", sessionToken)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != want {
			t.Errorf("%s %s as plain User = %d, want %d: %s", route.method, route.path, res.Code, want, res.Body.String())
		}

		anon := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		anon.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, anon)
		if res.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", route.method, route.path, res.Code)
		}
	}
}
