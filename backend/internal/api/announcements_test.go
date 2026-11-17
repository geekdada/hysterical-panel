package api

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

var announcementDoc = map[string]any{"type": "doc", "content": []any{}}

func newAnnouncementAdmin(t *testing.T, app core.App) *core.Record {
	t.Helper()
	admin := newUsersTestRecord(t, app, "announcement-admin@example.com", "AnnounceAdmin123")
	admin.Set("role", "admin")
	if err := app.Save(admin); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	return admin
}

func createTestAnnouncement(t *testing.T, h *Handlers, admin *core.Record, body map[string]any) ManagedAnnouncementDetail {
	t.Helper()
	e, rec := panelEvent(t, h.app, http.MethodPost, "/api/panel/announcements", "", body, admin)
	if err := h.createAnnouncement(e); err != nil {
		t.Fatalf("create announcement: %v", err)
	}
	return decodeBody[ManagedAnnouncementDetail](t, rec)
}

func announcementBodyFor(title string) map[string]any {
	return map[string]any{"title": title, "html": "<p>Hello</p>", "content": announcementDoc}
}

func TestCreateAnnouncementReturnsSanitizedDetail(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	h := &Handlers{app: app}
	body := announcementBodyFor("Maintenance")
	body["html"] = `<p onclick="x()">Hello</p><script>alert(1)</script>`
	body["pinned"] = true

	got := createTestAnnouncement(t, h, admin, body)
	if got.HTML != "<p>Hello</p>" || !got.Pinned || got.Hidden || got.Content["type"] != "doc" {
		t.Fatalf("detail = %+v", got)
	}
	stored, err := app.FindRecordById("announcements", got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetString("created_by") != admin.Id {
		t.Errorf("created_by = %q, want %q", stored.GetString("created_by"), admin.Id)
	}
}

func TestCreateAnnouncementRejectsInvalidInput(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	h := &Handlers{app: app}
	cases := map[string]struct {
		mutate  func(map[string]any)
		message string
	}{
		"blank title":   {func(b map[string]any) { b["title"] = " " }, "title is required"},
		"empty body":    {func(b map[string]any) { b["html"] = "<p></p>" }, "body is empty"},
		"no content":    {func(b map[string]any) { delete(b, "content") }, "editor content must be a json object"},
		"pinned hidden": {func(b map[string]any) { b["pinned"], b["hidden"] = true, true }, "a hidden announcement cannot be pinned"},
	}
	for name, tc := range cases {
		body := announcementBodyFor("t")
		tc.mutate(body)
		e, _ := panelEvent(t, app, http.MethodPost, "/api/panel/announcements", "", body, admin)
		err := h.createAnnouncement(e)
		if err == nil {
			t.Errorf("%s: created", name)
			continue
		}
		assertAPIError(t, err, http.StatusBadRequest, tc.message)
	}
}

func TestUpdateAnnouncement(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	h := &Handlers{app: app}
	created := createTestAnnouncement(t, h, admin, announcementBodyFor("Original"))

	patch := func(body map[string]any) (*httptest.ResponseRecorder, error) {
		e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/announcements/"+created.ID, created.ID, body, admin)
		return rec, h.updateAnnouncement(e)
	}

	_, err := patch(map[string]any{"html": "<p>New</p>"})
	assertAPIError(t, err, http.StatusBadRequest, "html and content must be sent together")

	rec, err := patch(map[string]any{"title": "Changed", "html": "<p>New</p>", "content": announcementDoc, "pinned": true})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeBody[ManagedAnnouncementDetail](t, rec); got.Title != "Changed" || got.HTML != "<p>New</p>" || !got.Pinned {
		t.Fatalf("after edit = %+v", got)
	}

	rec, err = patch(map[string]any{"hidden": true})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeBody[ManagedAnnouncementDetail](t, rec); !got.Hidden || got.Pinned {
		t.Fatalf("hiding did not unpin: %+v", got)
	}

	_, err = patch(map[string]any{"pinned": true})
	assertAPIError(t, err, http.StatusBadRequest, "a hidden announcement cannot be pinned")

	e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/announcements/missing", "missing", map[string]any{}, admin)
	assertAPIError(t, h.updateAnnouncement(e), http.StatusNotFound, "announcement not found")
}

func TestReaderAnnouncementEndpointsSkipHidden(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	reader := newUsersTestRecord(t, app, "announcement-reader@example.com", "AnnounceReader123")
	h := &Handlers{app: app}
	pinned := announcementBodyFor("Pinned")
	pinned["pinned"] = true
	visible := createTestAnnouncement(t, h, admin, pinned)
	hiddenBody := announcementBodyFor("Hidden")
	hiddenBody["hidden"] = true
	hidden := createTestAnnouncement(t, h, admin, hiddenBody)

	e, rec := panelEvent(t, app, http.MethodGet, "/api/panel/announcements/pinned", "", nil, reader)
	if err := h.listPinnedAnnouncements(e); err != nil {
		t.Fatal(err)
	}
	if got := decodeBody[PinnedAnnouncementsResponse](t, rec); !got.HasPublished || len(got.Items) != 1 || got.Items[0].ID != visible.ID {
		t.Fatalf("pinned = %+v", got)
	}

	e, rec = panelEvent(t, app, http.MethodGet, "/api/panel/announcements?page=0", "", nil, reader)
	if err := h.listAnnouncements(e); err != nil {
		t.Fatal(err)
	}
	list := decodeBody[AnnouncementListResponse](t, rec)
	if list.Total != 1 || list.Page != 1 || list.PerPage != announcementsPerPage || len(list.Items) != 1 {
		t.Fatalf("list = %+v", list)
	}

	e, rec = panelEvent(t, app, http.MethodGet, "/api/panel/announcements/"+visible.ID, visible.ID, nil, reader)
	if err := h.getAnnouncement(e); err != nil {
		t.Fatal(err)
	}
	if got := decodeBody[Announcement](t, rec); got.HTML != "<p>Hello</p>" {
		t.Fatalf("detail = %+v", got)
	}

	e, _ = panelEvent(t, app, http.MethodGet, "/api/panel/announcements/"+hidden.ID, hidden.ID, nil, reader)
	assertAPIError(t, h.getAnnouncement(e), http.StatusNotFound, "announcement not found")
}

func TestDeleteAnnouncementRemovesRecord(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	h := &Handlers{app: app}
	created := createTestAnnouncement(t, h, admin, announcementBodyFor("Gone"))
	e, _ := panelEvent(t, app, http.MethodDelete, "/api/panel/announcements/"+created.ID, created.ID, nil, admin)
	if err := h.deleteAnnouncement(e); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindRecordById("announcements", created.ID); err == nil {
		t.Fatal("announcement still exists")
	}
}

func newAnnouncementMux(t *testing.T, app core.App) http.Handler {
	t.Helper()
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	box, _ := cryptobox.New("announcement-test-master-key")
	Register(&core.ServeEvent{App: app, Router: r}, app, box, nil, nil, nil, nil, PanelConfigResponse{})
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("mux: %v", err)
	}
	return mux
}

func serveAs(mux http.Handler, method, path, token string) int {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res.Code
}

// The handlers above are called directly, so this goes through the real
// router to prove the route-level guards.
func TestAnnouncementRouteGuards(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	reader := newUsersTestRecord(t, app, "announcement-guard@example.com", "AnnounceGuard123")
	token, err := reader.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	visible := createTestAnnouncement(t, &Handlers{app: app}, admin, announcementBodyFor("Visible"))
	mux := newAnnouncementMux(t, app)

	readerRoutes := []string{"/api/panel/announcements", "/api/panel/announcements/pinned", "/api/panel/announcements/" + visible.ID}
	for _, path := range readerRoutes {
		if code := serveAs(mux, http.MethodGet, path, token); code != http.StatusOK {
			t.Errorf("GET %s as reader = %d, want 200", path, code)
		}
		if code := serveAs(mux, http.MethodGet, path, ""); code != http.StatusUnauthorized {
			t.Errorf("GET %s anonymously = %d, want 401", path, code)
		}
	}
	adminRoutes := []struct{ method, path string }{
		{http.MethodGet, "/api/panel/announcements/manage"},
		{http.MethodGet, "/api/panel/announcements/manage/" + visible.ID},
		{http.MethodPost, "/api/panel/announcements"},
		{http.MethodPatch, "/api/panel/announcements/" + visible.ID},
		{http.MethodDelete, "/api/panel/announcements/" + visible.ID},
		{http.MethodPost, "/api/panel/files"},
	}
	for _, route := range adminRoutes {
		if code := serveAs(mux, route.method, route.path, token); code != http.StatusForbidden {
			t.Errorf("%s %s as reader = %d, want 403", route.method, route.path, code)
		}
	}
	assertUnusableReadersRejected(t, app, mux, reader, token, readerRoutes)
}

// A token issued before the account changed stays valid, so the reader routes
// must check the account on every request.
func assertUnusableReadersRejected(t *testing.T, app core.App, mux http.Handler, reader *core.Record, token string, routes []string) {
	t.Helper()
	for _, change := range []struct {
		field string
		value any
	}{{"status", "disabled"}, {"verified", false}} {
		fresh, err := app.FindRecordById("users", reader.Id)
		if err != nil {
			t.Fatal(err)
		}
		fresh.Set("status", "active")
		fresh.Set("verified", true)
		fresh.Set(change.field, change.value)
		if err := app.Save(fresh); err != nil {
			t.Fatal(err)
		}
		for _, path := range routes {
			if code := serveAs(mux, http.MethodGet, path, token); code != http.StatusForbidden {
				t.Errorf("GET %s with %s=%v = %d, want 403", path, change.field, change.value, code)
			}
		}
	}
}

func multipartUpload(t *testing.T, name, contentType string, content io.Reader) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, uploadFormField, name))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

// The upload goes through the router so the test also covers lifting
// PocketBase's default 32 MiB body limit on this route.
func TestUploadFileAcceptsBodiesAboveDefaultLimit(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	token, err := admin.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	mux := newAnnouncementMux(t, app)
	size := apis.DefaultMaxBodySize + 1<<20
	body, contentType := multipartUpload(t, "Big Video.mp4", "video/mp4", io.LimitReader(zeroReader{}, size))

	req := httptest.NewRequest(http.MethodPost, "/api/panel/files", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("upload = %d: %s", res.Code, res.Body.String())
	}
	got := decodeBody[UploadedFile](t, res)
	if got.Name != "Big Video.mp4" || got.MIME != "video/mp4" || got.Size != size {
		t.Fatalf("uploaded = %+v", got)
	}
	if !strings.HasPrefix(got.Path, "/api/files/files/"+got.ID+"/big_video_") {
		t.Fatalf("path = %q", got.Path)
	}
}

func TestUploadFileRequiresFile(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newAnnouncementAdmin(t, app)
	h := &Handlers{app: app}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/panel/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	e, _ := panelEvent(t, app, http.MethodPost, "/api/panel/files", "", nil, admin)
	e.Request = req
	assertAPIError(t, h.uploadFile(e), http.StatusBadRequest, "file is required")
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
