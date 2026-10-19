package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestManagementSpecMatchesRoutes(t *testing.T) {
	spec, err := BuildManagementOpenAPISpec("")
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	routes := (&Handlers{}).managementRoutes()
	for _, r := range routes {
		item := spec.Paths.Find("/api/mgmt" + r.path)
		if item == nil || item.GetOperation(r.method) == nil {
			t.Fatalf("route %s /api/mgmt%s has no operation in the spec", r.method, r.path)
		}
	}
	operations := 0
	for path, item := range spec.Paths.Map() {
		if !strings.HasPrefix(path, "/api/mgmt/") {
			t.Fatalf("spec path %s is outside /api/mgmt", path)
		}
		operations += len(item.Operations())
	}
	if operations != len(routes) {
		t.Fatalf("spec has %d operations, routes table has %d", operations, len(routes))
	}
}

func TestManagementSpecExamplesMatchSchemas(t *testing.T) {
	spec, err := BuildManagementOpenAPISpec("https://panel.example.com")
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate spec: %v", err)
	}

	checkMedia := func(where string, media *openapi3.MediaType) {
		if media == nil || media.Example == nil {
			return
		}
		if err := media.Schema.Value.VisitJSON(media.Example); err != nil {
			t.Fatalf("%s example does not match its schema: %v", where, err)
		}
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			where := method + " " + path
			if op.Summary == "" || op.Description == "" || len(op.Tags) != 1 {
				t.Fatalf("%s needs a summary, a description and exactly one tag", where)
			}
			if op.RequestBody != nil {
				checkMedia(where+" request", op.RequestBody.Value.Content.Get("application/json"))
			}
			for status, response := range op.Responses.Map() {
				media := response.Value.Content.Get("application/json")
				checkMedia(where+" "+status, media)
				isSuccess := strings.HasPrefix(status, "2")
				if isSuccess && op.OperationID != "mgmtGetOpenAPISpec" && (media == nil || media.Example == nil) {
					t.Fatalf("%s %s response has no example", where, status)
				}
			}
		}
	}
	errorSchema := doc.Components.Schemas["ErrorResponse"].Value
	if errorSchema.Example == nil {
		t.Fatal("ErrorResponse schema needs an example")
	}
	if err := errorSchema.VisitJSON(errorSchema.Example); err != nil {
		t.Fatalf("ErrorResponse example does not match its schema: %v", err)
	}
}

func TestManagementSpecServers(t *testing.T) {
	without, err := BuildManagementOpenAPISpec("")
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	if len(without.Servers) != 0 {
		t.Fatalf("servers = %+v, want none without a backend URL", without.Servers)
	}
	with, err := BuildManagementOpenAPISpec("https://panel.example.com")
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	if len(with.Servers) != 1 || with.Servers[0].URL != "https://panel.example.com" {
		t.Fatalf("servers = %+v, want https://panel.example.com", with.Servers)
	}
}

func TestManagementSpecUsesBearerAuth(t *testing.T) {
	spec, err := BuildManagementOpenAPISpec("")
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	if spec.OpenAPI != "3.1.0" || spec.Info.Title != "Hysterical Panel Management API" {
		t.Fatalf("openapi = %q, title = %q", spec.OpenAPI, spec.Info.Title)
	}
	scheme := spec.Components.SecuritySchemes["bearerAuth"]
	if scheme == nil || scheme.Value.Type != "http" || scheme.Value.Scheme != "bearer" {
		t.Fatalf("bearerAuth scheme = %+v", scheme)
	}
	if len(spec.Security) != 1 {
		t.Fatalf("security = %+v, want bearerAuth only", spec.Security)
	}
	if _, ok := spec.Security[0]["bearerAuth"]; !ok {
		t.Fatalf("security = %+v, want bearerAuth", spec.Security)
	}
}
