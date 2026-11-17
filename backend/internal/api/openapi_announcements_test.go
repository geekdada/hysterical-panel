package api

import "testing"

func TestOpenAPIDescribesAnnouncements(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	operations := []struct{ path, method, id string }{
		{"/api/panel/announcements", "GET", "listAnnouncements"},
		{"/api/panel/announcements", "POST", "createAnnouncement"},
		{"/api/panel/announcements/pinned", "GET", "listPinnedAnnouncements"},
		{"/api/panel/announcements/{id}", "GET", "getAnnouncement"},
		{"/api/panel/announcements/{id}", "PATCH", "updateAnnouncement"},
		{"/api/panel/announcements/{id}", "DELETE", "deleteAnnouncement"},
		{"/api/panel/announcements/manage", "GET", "listManagedAnnouncements"},
		{"/api/panel/announcements/manage/{id}", "GET", "getManagedAnnouncement"},
		{"/api/panel/files", "POST", "uploadFile"},
	}
	for _, tc := range operations {
		item := spec.Paths.Find(tc.path)
		if item == nil {
			t.Fatalf("missing path %s", tc.path)
		}
		op := item.GetOperation(tc.method)
		if op == nil || op.OperationID != tc.id {
			t.Fatalf("%s %s = %+v, want operation %s", tc.method, tc.path, op, tc.id)
		}
		if op.Security != nil {
			t.Errorf("%s must inherit auth, Security = %+v", tc.id, op.Security)
		}
		if op.Responses.Status(401) == nil || op.Responses.Status(403) == nil {
			t.Errorf("%s must document 401 and 403", tc.id)
		}
	}

	upload := spec.Paths.Find("/api/panel/files").Post
	form := upload.RequestBody.Value.Content["multipart/form-data"]
	if form == nil || form.Schema.Value.Properties[uploadFormField] == nil {
		t.Fatalf("uploadFile must take a multipart file field, got %+v", upload.RequestBody.Value.Content)
	}
	if upload.Responses.Status(413) == nil {
		t.Fatal("uploadFile must document 413")
	}

	pinned := spec.Components.Schemas["PinnedAnnouncementsResponse"].Value
	if pinned.Properties["items"].Value.Items.Ref != "#/components/schemas/AnnouncementSummary" {
		t.Fatalf("pinned items = %+v, want AnnouncementSummary refs", pinned.Properties["items"].Value.Items)
	}
	detail := spec.Components.Schemas["ManagedAnnouncementDetail"].Value
	for _, field := range []string{"id", "pinned", "hidden", "html", "content"} {
		if detail.Properties[field] == nil {
			t.Errorf("ManagedAnnouncementDetail lacks %s", field)
		}
	}
}
