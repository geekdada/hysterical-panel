package api

import "testing"

func TestOpenAPIDescribesResourceLinks(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	operations := []struct {
		path   string
		method string
		id     string
	}{
		{"/api/panel/resource-links", "GET", "listResourceLinks"},
		{"/api/panel/resource-links/manage", "GET", "listManagedResourceLinks"},
		{"/api/panel/resource-links", "POST", "createResourceLink"},
		{"/api/panel/resource-links/order", "PUT", "reorderResourceLinks"},
		{"/api/panel/resource-links/{id}", "PATCH", "updateResourceLink"},
		{"/api/panel/resource-links/{id}", "DELETE", "deleteResourceLink"},
		{"/api/resource-links/{id}/visit", "POST", "visitResourceLink"},
		{"/api/resource-links/{id}/favicon", "GET", "getResourceLinkFavicon"},
	}
	for _, tc := range operations {
		item := spec.Paths.Find(tc.path)
		if item == nil {
			t.Fatalf("missing path %s", tc.path)
		}
		op := item.GetOperation(tc.method)
		if op == nil {
			t.Fatalf("missing %s %s", tc.method, tc.path)
		}
		if op.OperationID != tc.id {
			t.Fatalf("%s %s OperationID = %q, want %q", tc.method, tc.path, op.OperationID, tc.id)
		}
		switch tc.id {
		case "visitResourceLink", "getResourceLinkFavicon":
			if op.Security == nil || len(*op.Security) != 0 {
				t.Fatalf("%s must be public, Security = %+v", tc.id, op.Security)
			}
		case "listResourceLinks":
			if op.Security != nil {
				t.Fatalf("listResourceLinks must inherit auth, Security = %+v", op.Security)
			}
		}
	}

	reorder := spec.Paths.Find("/api/panel/resource-links/order").Put
	if reorder.Responses.Status(409) == nil {
		t.Fatal("reorderResourceLinks must document 409")
	}

	favicon := spec.Paths.Find("/api/resource-links/{id}/favicon").Get
	if favicon.Responses.Status(304) == nil || favicon.Responses.Status(404) == nil {
		t.Fatal("getResourceLinkFavicon must document 304 and 404")
	}
	icon := favicon.Responses.Status(200).Value.Content["image/*"]
	if icon == nil || icon.Schema == nil || icon.Schema.Value == nil ||
		icon.Schema.Value.Type == nil || !icon.Schema.Value.Type.Includes("string") ||
		icon.Schema.Value.Format != "binary" {
		t.Fatalf("favicon 200 content = %+v, want image/* binary string", icon)
	}

	for _, name := range []string{
		"ResourceLink",
		"ManagedResourceLink",
		"ResourceLinkCreateRequest",
		"ResourceLinkUpdateRequest",
		"ResourceLinkOrderRequest",
		"ResourceLinkVisitResponse",
	} {
		if spec.Components.Schemas[name] == nil {
			t.Fatalf("missing schema %s", name)
		}
	}
	managed := spec.Components.Schemas["ManagedResourceLink"]
	if managed.Value.Properties["url"] == nil || managed.Value.Properties["id"] == nil {
		t.Fatalf("ManagedResourceLink must inline the public fields and url, got %+v", managed.Value.Properties)
	}
}
