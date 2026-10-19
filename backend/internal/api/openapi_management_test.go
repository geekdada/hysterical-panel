package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pocketbase/pocketbase/apis"
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

func TestManagementSpecRouteIsGated(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app, publicConfig: PanelConfigResponse{APIURL: "https://panel.example.com"}}
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	h.bindManagementRoutes(r.Group("/api/mgmt"))
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	get := func(authorization string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/mgmt/openapi.json", nil)
		req.Header.Set("Authorization", authorization)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}

	if res := get("Bearer good-token"); res.Code != http.StatusNotFound {
		t.Fatalf("disabled feature status = %d, want 404", res.Code)
	}
	settings, err := h.settingsRecord()
	if err != nil {
		t.Fatalf("settingsRecord: %v", err)
	}
	settings.Set("management_api_enabled", true)
	settings.Set("management_api_token_hash", hashManagementAPIToken("good-token"))
	if err := app.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if res := get("Bearer wrong"); res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want 401", res.Code)
	}
	res := get("Bearer good-token")
	if res.Code != http.StatusOK {
		t.Fatalf("good token status = %d, want 200", res.Code)
	}
	var body struct {
		OpenAPI string `json:"openapi"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if body.OpenAPI != "3.1.0" || len(body.Servers) != 1 || body.Servers[0].URL != "https://panel.example.com" {
		t.Fatalf("spec = %+v", body)
	}
}

func TestPanelManagementSpecWorksWhileFeatureDisabled(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	event, res := jsonRequestEvent(t, app, http.MethodGet, "/api/panel/management-api/openapi.json", nil)
	if err := h.managementOpenAPISpec(event); err != nil {
		t.Fatalf("managementOpenAPISpec: %v", err)
	}
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var body struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if body.Paths["/api/mgmt/nodes"] == nil {
		t.Fatalf("spec paths = %v, want /api/mgmt/nodes", body.Paths)
	}
}

func TestOpenAPIDescribesManagementSpecRoute(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("build spec: %v", err)
	}
	item := spec.Paths.Find("/api/panel/management-api/openapi.json")
	if item == nil || item.Get == nil || item.Get.OperationID != "getManagementAPISpec" {
		t.Fatalf("missing getManagementAPISpec, item = %+v", item)
	}
	for path := range spec.Paths.Map() {
		if strings.HasPrefix(path, "/api/mgmt") {
			t.Fatalf("public spec must not include %s", path)
		}
	}
}
