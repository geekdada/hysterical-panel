package api

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

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

// newRacingResourceLinkPageServer serves a titled page. Before replying to the
// page request it bumps visit_count by 5 and sets sort_order to 7, which acts
// as a visit and a reorder committed during the metadata fetch.
func newRacingResourceLinkPageServer(t *testing.T, app core.App, linkID string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/page" {
			http.NotFound(w, r)
			return
		}
		_, err := app.DB().NewQuery(
			"UPDATE resource_links SET visit_count = visit_count + 5, sort_order = 7 WHERE id = {:id}",
		).Bind(dbx.Params{"id": linkID}).Execute()
		if err != nil {
			t.Errorf("concurrent update: %v", err)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><head><title>Raced</title></head></html>`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateResourceLinkKeepsChangesMadeDuringFetch(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	created := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	page := newRacingResourceLinkPageServer(t, app, created.ID)

	e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/resource-links/"+created.ID, created.ID,
		map[string]any{"url": page.URL + "/page"}, admin)
	if err := h.updateResourceLink(e); err != nil {
		t.Fatalf("update URL: %v", err)
	}
	updated := decodeBody[ManagedResourceLink](t, rec)
	if updated.FetchedTitle != "Raced" || updated.VisitCount != 5 {
		t.Fatalf("update response = %+v", updated)
	}
	stored, err := app.FindRecordById("resource_links", created.ID)
	if err != nil {
		t.Fatalf("find link: %v", err)
	}
	if stored.GetInt("visit_count") != 5 || stored.GetInt("sort_order") != 7 {
		t.Fatalf("visit_count = %d, sort_order = %d, want 5 and 7",
			stored.GetInt("visit_count"), stored.GetInt("sort_order"))
	}
	if stored.GetString("url") != page.URL+"/page" || stored.GetString("fetched_title") != "Raced" {
		t.Fatalf("url = %q, fetched_title = %q", stored.GetString("url"), stored.GetString("fetched_title"))
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
// router to prove the route-level guards. The public visit and favicon routes
// must answer anonymous requests.
func TestResourceLinkRoutesGuards(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "resource-guard@example.com", "GuardSecret123")
	sessionToken, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	admin := newResourceLinkAdmin(t, app)
	page := newResourceLinkPageServer(t)
	link := createResourceLink(t, &Handlers{app: app}, admin, map[string]any{"url": page.URL + "/"})
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
		public       bool
	}{
		{http.MethodGet, "/api/panel/resource-links", false, false},
		{http.MethodGet, "/api/panel/resource-links/manage", true, false},
		{http.MethodPost, "/api/panel/resource-links", true, false},
		{http.MethodPut, "/api/panel/resource-links/order", true, false},
		{http.MethodPatch, "/api/panel/resource-links/any", true, false},
		{http.MethodDelete, "/api/panel/resource-links/any", true, false},
		{http.MethodPost, "/api/resource-links/" + link.ID + "/visit", false, true},
		{http.MethodGet, "/api/resource-links/" + link.ID + "/favicon", false, true},
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

		wantAnon := http.StatusUnauthorized
		if route.public {
			wantAnon = http.StatusOK
		}
		anon := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		anon.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		mux.ServeHTTP(res, anon)
		if res.Code != wantAnon {
			t.Errorf("%s %s without a token = %d, want %d", route.method, route.path, res.Code, wantAnon)
		}
	}
}

func TestVisitResourceLinkCountsAndReturnsTarget(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	link := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})

	before := time.Now().UTC().Add(-time.Minute)
	e, rec := panelEvent(t, app, http.MethodPost, "/api/resource-links/"+link.ID+"/visit", link.ID, nil, nil)
	if err := h.visitResourceLink(e); err != nil {
		t.Fatalf("visit: %v", err)
	}
	if got := decodeBody[ResourceLinkVisitResponse](t, rec); got.URL != link.URL {
		t.Fatalf("url = %q, want %q", got.URL, link.URL)
	}

	stored, err := app.FindRecordById("resource_links", link.ID)
	if err != nil {
		t.Fatalf("find link: %v", err)
	}
	if stored.GetInt("visit_count") != 1 {
		t.Fatalf("visit_count = %d, want 1", stored.GetInt("visit_count"))
	}
	visitedAt, err := types.ParseDateTime(stored.GetString("last_visited_at"))
	if err != nil {
		t.Fatalf("parse last_visited_at %q: %v", stored.GetString("last_visited_at"), err)
	}
	if visitedAt.Time().Location() != time.UTC {
		t.Fatalf("last_visited_at location = %v, want UTC", visitedAt.Time().Location())
	}
	if visitedAt.Time().Before(before) || visitedAt.Time().After(time.Now().UTC().Add(time.Minute)) {
		t.Fatalf("last_visited_at = %v, want around now", visitedAt.Time())
	}
}

func TestVisitResourceLinkCountsConcurrentHits(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	link := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})

	const hits = 20
	events := make([]*core.RequestEvent, hits)
	for i := range events {
		events[i], _ = panelEvent(t, app, http.MethodPost, "/api/resource-links/"+link.ID+"/visit", link.ID, nil, nil)
	}

	errs := make([]error, hits)
	var wg sync.WaitGroup
	for i := range events {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = h.visitResourceLink(events[i])
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("hit %d: %v", i, err)
		}
	}

	stored, err := app.FindRecordById("resource_links", link.ID)
	if err != nil {
		t.Fatalf("find link: %v", err)
	}
	if stored.GetInt("visit_count") != hits {
		t.Fatalf("visit_count = %d, want %d", stored.GetInt("visit_count"), hits)
	}
}

func TestVisitResourceLinkUnknownID(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}

	e, _ := panelEvent(t, app, http.MethodPost, "/api/resource-links/missing/visit", "missing", nil, nil)
	assertAPIError(t, h.visitResourceLink(e), http.StatusNotFound, "resource link not found")
}

func TestResourceLinkFaviconServesStoredIconWithCacheHeaders(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	page := newResourceLinkPageServer(t)
	link := createResourceLink(t, h, admin, map[string]any{"url": page.URL + "/"})
	if link.FaviconVersion == "" {
		t.Fatalf("link has no stored favicon: %+v", link)
	}

	e, rec := panelEvent(t, app, http.MethodGet, "/api/resource-links/"+link.ID+"/favicon", link.ID, nil, nil)
	if err := h.resourceLinkFavicon(e); err != nil {
		t.Fatalf("favicon: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), resourceLinkTestIcon) {
		t.Fatalf("body = %q, want the stored icon", rec.Body.Bytes())
	}
	headers := map[string]string{
		"Content-Type":           "image/png",
		"Cache-Control":          "public, max-age=86400",
		"ETag":                   `"` + link.FaviconVersion + `"`,
		"X-Content-Type-Options": "nosniff",
	}
	for name, want := range headers {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestResourceLinkFaviconNotModified(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	page := newResourceLinkPageServer(t)
	link := createResourceLink(t, h, admin, map[string]any{"url": page.URL + "/"})

	e, rec := panelEvent(t, app, http.MethodGet, "/api/resource-links/"+link.ID+"/favicon", link.ID, nil, nil)
	e.Request.Header.Set("If-None-Match", `"`+link.FaviconVersion+`"`)
	if err := h.resourceLinkFavicon(e); err != nil {
		t.Fatalf("favicon: %v", err)
	}
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.Bytes())
	}
}

func TestResourceLinkFaviconMissing(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newResourceLinkAdmin(t, app)
	h := &Handlers{app: app}
	link := createResourceLink(t, h, admin, map[string]any{"url": closedResourceLinkServerURL(t)})
	if link.FaviconVersion != "" {
		t.Fatalf("link unexpectedly has an icon: %+v", link)
	}

	e, _ := panelEvent(t, app, http.MethodGet, "/api/resource-links/"+link.ID+"/favicon", link.ID, nil, nil)
	assertAPIError(t, h.resourceLinkFavicon(e), http.StatusNotFound, "resource link not found")

	e, _ = panelEvent(t, app, http.MethodGet, "/api/resource-links/missing/favicon", "missing", nil, nil)
	assertAPIError(t, h.resourceLinkFavicon(e), http.StatusNotFound, "resource link not found")
}
