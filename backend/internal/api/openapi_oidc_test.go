package api

import (
	"slices"
	"testing"
)

func TestOpenAPIDescribesConnectedApps(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	for path, methods := range map[string][]string{
		"/api/panel/oidc-clients":                    {"GET", "POST"},
		"/api/panel/oidc-clients/{id}":               {"PATCH", "DELETE"},
		"/api/panel/oidc-clients/{id}/rotate-secret": {"POST"},
	} {
		item := spec.Paths.Find(path)
		if item == nil {
			t.Fatalf("missing path %s", path)
		}
		for _, method := range methods {
			if item.GetOperation(method) == nil {
				t.Fatalf("missing %s %s", method, path)
			}
		}
	}
	schemas := spec.Components.Schemas
	client := schemas["OIDCClient"]
	if client == nil || !slices.Contains(client.Value.Required, "client_id") || client.Value.Properties["secret_hash"] != nil {
		t.Fatalf("OIDCClient schema = %+v", client)
	}
	// The generator inlines nested structs without their required list, which
	// would type every field of the returned client as optional.
	if ref := schemas["OIDCClientSecretResponse"].Value.Properties["client"]; ref == nil || ref.Ref != "#/components/schemas/OIDCClient" {
		t.Fatalf("OIDCClientSecretResponse.client = %+v, want a $ref to OIDCClient", ref)
	}
	reason := schemas["SettingsResponse"].Value.Properties["oidc_unavailable_reason"]
	if reason == nil || len(reason.Value.Enum) != 4 {
		t.Fatalf("oidc_unavailable_reason enum = %+v", reason)
	}
}
