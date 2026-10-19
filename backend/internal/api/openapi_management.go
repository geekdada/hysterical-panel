package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/version"
)

const managementAPIDescription = `Every request must send Authorization: Bearer <token>.
- While the Management API is turned off, every endpoint returns 404.
- A missing or invalid token returns 401.`

// mgmtOperation declares one documented /api/mgmt route. Examples are typed
// DTO values, so a renamed or retyped field changes the example with it.
type mgmtOperation struct {
	method         string
	path           string
	id             string
	tag            string
	summary        string
	description    string
	params         openapi3.Parameters
	bodySchema     string
	bodyExample    any
	status         int
	successDesc    string
	successSchema  *openapi3.SchemaRef
	successExample any
	errors         []mgmtError
}

type mgmtError struct {
	status      int
	description string
}

var (
	exampleNode = Node{
		ID:             "k2m9x7c4v1b8n3q",
		Name:           "Tokyo 1",
		APIURL:         "https://tokyo-1.example.com:25413",
		PollInterval:   30,
		Enabled:        true,
		LastPolledAt:   "2026-06-01T08:30:00.000Z",
		Health:         "ok",
		CurrentTxSpeed: 131072,
		CurrentRxSpeed: 524288,
		OnlineDevices:  int64Ptr(12),
	}
	exampleDisabledNode = func() Node {
		n := exampleNode
		n.Enabled = false
		n.CurrentTxSpeed = 0
		n.CurrentRxSpeed = 0
		n.OnlineDevices = int64Ptr(0)
		return n
	}()
	examplePanelUser = PanelUser{
		UserProfile: UserProfile{
			ID:                   "rec_8f3k2j1d0s9a7gx",
			Email:                "user@example.com",
			Role:                 "user",
			AuthString:           "qZ4yN8tR2vL6wB1k",
			SubscriptionRequired: true,
			Status:               "active",
			Created:              "2026-06-01T08:30:00.000Z",
			LastConnectedAt:      "2026-06-02T10:15:00.000Z",
			RecentConnections: []RecentConnection{
				{IP: "203.0.113.7", LastSeenAt: "2026-06-02T10:15:00.000Z"},
			},
		},
		UsedTx: 1048576,
		UsedRx: 5242880,
	}
)

var nodeNotFound = mgmtError{404, "The node does not exist or has been deleted"}

func managementOperations() []mgmtOperation {
	nodeID := pathParam("id", "Node ID", "k2m9x7c4v1b8n3q")
	return []mgmtOperation{
		{
			method: http.MethodGet, path: "/api/mgmt/users", id: "mgmtGetUser", tag: "Users",
			summary: "Look up one user",
			description: `Look up a single user by User ID (id), email, or Current Auth String (auth_string).
- Provide exactly one of the three.
- Retired Auth Strings do not match.
- Connected Apps receive the User ID as sub, so look their users up by id.`,
			params: openapi3.Parameters{
				queryParam("id", "User ID", nil),
				queryParam("email", "Exact email", "user@example.com"),
				queryParam("auth_string", "Current Auth String", nil),
			},
			status: http.StatusOK, successDesc: "The matching user",
			successSchema: schemaRef("PanelUser"), successExample: examplePanelUser,
			errors: []mgmtError{{400, "More than one parameter, or none, was sent"}, {404, "No user matches"}},
		},
		{
			method: http.MethodPost, path: "/api/mgmt/users", id: "mgmtCreateUser", tag: "Users",
			summary: "Create a user",
			description: `Create a user from an email.
- The password and auth_string are generated server-side and never returned.
- The account is created active and verified with the user role.
- Send the new user through the password reset flow to grant access.`,
			bodySchema:  "ManagementUserCreateRequest",
			bodyExample: ManagementUserCreateRequest{Email: "user@example.com"},
			status:      http.StatusCreated, successDesc: "The created user",
			successSchema:  schemaRef("ManagementUserCreateResponse"),
			successExample: ManagementUserCreateResponse{ID: "rec_8f3k2j1d0s9a7gx", Email: "user@example.com", Status: "active"},
			errors:         []mgmtError{{400, "Invalid body, missing email, or email already taken"}},
		},
		{
			method: http.MethodGet, path: "/api/mgmt/nodes", id: "mgmtListNodes", tag: "Nodes",
			summary:     "List nodes",
			description: "List every node that has not been deleted, newest first.",
			status:      http.StatusOK, successDesc: "Node list",
			successSchema: schemaArrayRef("Node"), successExample: []Node{exampleNode},
		},
		{
			method: http.MethodPost, path: "/api/mgmt/nodes", id: "mgmtCreateNode", tag: "Nodes",
			summary: "Register a node",
			description: `Register a node.
- name, api_url and api_secret are required.
- poll_interval is in seconds and defaults to 30.
- enabled defaults to true.`,
			bodySchema: "NodeCreateRequest",
			bodyExample: NodeCreateRequest{
				Name: "Tokyo 1", APIURL: "https://tokyo-1.example.com:25413", APISecret: "NODE_API_SECRET",
			},
			status: http.StatusCreated, successDesc: "The created node",
			successSchema: schemaRef("Node"), successExample: exampleNode,
			errors: []mgmtError{{400, "A required field is missing or a value is invalid"}},
		},
		{
			method: http.MethodGet, path: "/api/mgmt/nodes/{id}", id: "mgmtGetNode", tag: "Nodes",
			summary:     "Get a node",
			description: "Look up a single node by ID.",
			params:      openapi3.Parameters{nodeID},
			status:      http.StatusOK, successDesc: "The node",
			successSchema: schemaRef("Node"), successExample: exampleNode,
			errors: []mgmtError{nodeNotFound},
		},
		{
			method: http.MethodPatch, path: "/api/mgmt/nodes/{id}", id: "mgmtUpdateNode", tag: "Nodes",
			summary: "Update, enable or disable a node",
			description: `Change any of name, api_url, api_secret, poll_interval and enabled. Omitted fields keep their values.
- A new api_url needs a new api_secret in the same request, so the panel never sends the old secret to a different address.
- api_secret cannot be an empty string.
- Send enabled false to disable a node and enabled true to enable it again. The panel stops polling a disabled node, hides it from Users and leaves it out of kicks. Its Current Speed and Online Device Count drop to 0.
- Clients can still connect through a disabled node until you stop the node service itself.`,
			params:      openapi3.Parameters{nodeID},
			bodySchema:  "NodeUpdateRequest",
			bodyExample: NodeUpdateRequest{Enabled: boolPtr(false)},
			status:      http.StatusOK, successDesc: "The updated node",
			successSchema: schemaRef("Node"), successExample: exampleDisabledNode,
			errors: []mgmtError{
				{400, "api_url changed without api_secret, api_secret is an empty string, or a value is invalid"},
				nodeNotFound,
			},
		},
		{
			method: http.MethodDelete, path: "/api/mgmt/nodes/{id}", id: "mgmtDeleteNode", tag: "Nodes",
			summary:     "Delete a node",
			description: "Soft-delete a node. The panel keeps its Traffic history, but every endpoint treats the node as missing from then on. A deleted node cannot be restored.",
			params:      openapi3.Parameters{nodeID},
			status:      http.StatusOK, successDesc: "Deletion confirmed",
			successSchema: schemaRef("DeleteResponse"), successExample: DeleteResponse{Deleted: true},
			errors: []mgmtError{nodeNotFound},
		},
		{
			method: http.MethodPost, path: "/api/mgmt/nodes/{id}/test", id: "mgmtTestNode", tag: "Nodes",
			summary: "Test a node's Traffic Stats API",
			description: `Check that the panel can reach the node's Traffic Stats API with the stored api_url and api_secret.
- Disabled nodes can be tested too.
- A failed check still returns 200, with ok set to false and the reason in error.`,
			params: openapi3.Parameters{nodeID},
			status: http.StatusOK, successDesc: "Test result",
			successSchema: schemaRef("NodeTestResponse"), successExample: NodeTestResponse{OK: true, LatencyMs: 42},
			errors: []mgmtError{nodeNotFound},
		},
		{
			method: http.MethodGet, path: "/api/mgmt/openapi.json", id: "mgmtGetOpenAPISpec", tag: "Specification",
			summary:     "Get this OpenAPI document",
			description: "Return the OpenAPI document for the Management API as currently deployed.",
			status:      http.StatusOK, successDesc: "OpenAPI 3.1 document",
			successSchema: &openapi3.SchemaRef{Value: openapi3.NewObjectSchema().WithAnyAdditionalProperties()},
		},
	}
}

// BuildManagementOpenAPISpec constructs the OpenAPI 3.1 document for
// /api/mgmt/*. It is separate from BuildOpenAPISpec so the panel's generated
// client never includes Management API types. serverURL is omitted when empty.
func BuildManagementOpenAPISpec(serverURL string) (*openapi3.T, error) {
	schemas, err := generateSchemas(map[string]any{
		"Node":                         Node{},
		"NodeCreateRequest":            NodeCreateRequest{},
		"NodeUpdateRequest":            NodeUpdateRequest{},
		"NodeTestResponse":             NodeTestResponse{},
		"PanelUser":                    PanelUser{},
		"ManagementUserCreateRequest":  ManagementUserCreateRequest{},
		"ManagementUserCreateResponse": ManagementUserCreateResponse{},
		"DeleteResponse":               DeleteResponse{},
		"ErrorResponse":                ErrorResponse{},
	})
	if err != nil {
		return nil, err
	}
	// openapi3gen never marks fields required; these match the handlers' checks.
	schemas["NodeCreateRequest"].Value.Required = []string{"name", "api_url", "api_secret"}
	schemas["ManagementUserCreateRequest"].Value.Required = []string{"email"}
	schemas["ErrorResponse"].Value.Example = exampleJSON(ErrorResponse{
		Status: http.StatusUnauthorized, Message: "invalid token", Data: map[string]any{},
	})

	t := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:       "Hysterical Panel Management API",
			Version:     version.Version,
			Description: managementAPIDescription,
		},
		Paths: openapi3.NewPaths(),
		Tags: openapi3.Tags{
			{Name: "Users"},
			{Name: "Nodes", Description: "Node responses never include api_secret. Every node endpoint treats a deleted node as missing."},
			{Name: "Specification"},
		},
		Components: &openapi3.Components{
			Schemas: schemas,
			SecuritySchemes: openapi3.SecuritySchemes{
				"bearerAuth": &openapi3.SecuritySchemeRef{Value: openapi3.NewSecurityScheme().WithType("http").WithScheme("bearer")},
			},
		},
		Security: openapi3.SecurityRequirements{{"bearerAuth": {}}},
	}
	if serverURL != "" {
		t.Servers = openapi3.Servers{{URL: serverURL}}
	}
	for _, op := range managementOperations() {
		item := t.Paths.Value(op.path)
		if item == nil {
			item = &openapi3.PathItem{}
			t.Paths.Set(op.path, item)
		}
		item.SetOperation(op.method, op.build())
	}
	return t, nil
}

func (op mgmtOperation) build() *openapi3.Operation {
	responses := openapi3.NewResponses(openapi3.WithStatus(op.status, &openapi3.ResponseRef{
		Value: openapi3.NewResponse().
			WithDescription(op.successDesc).
			WithContent(jsonContent(op.successSchema, op.successExample)),
	}))
	errs := append([]mgmtError{{401, "Missing or invalid token"}}, op.errors...)
	for _, e := range errs {
		responses.Set(fmt.Sprint(e.status), &openapi3.ResponseRef{
			Value: openapi3.NewResponse().
				WithDescription(e.description).
				WithContent(jsonContent(schemaRef("ErrorResponse"), nil)),
		})
	}
	operation := &openapi3.Operation{
		OperationID: op.id,
		Tags:        []string{op.tag},
		Summary:     op.summary,
		Description: op.description,
		Parameters:  op.params,
		Responses:   responses,
	}
	if op.bodySchema != "" {
		operation.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().
			WithRequired(true).
			WithContent(jsonContent(schemaRef(op.bodySchema), op.bodyExample))}
	}
	return operation
}

func jsonContent(schema *openapi3.SchemaRef, example any) openapi3.Content {
	media := &openapi3.MediaType{Schema: schema}
	if example != nil {
		media.Example = exampleJSON(example)
	}
	return openapi3.Content{"application/json": media}
}

func schemaRef(name string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name}
}

func schemaArrayRef(name string) *openapi3.SchemaRef {
	array := openapi3.NewArraySchema()
	array.Items = schemaRef(name)
	return &openapi3.SchemaRef{Value: array}
}

func pathParam(name, description string, example any) *openapi3.ParameterRef {
	p := openapi3.NewPathParameter(name).WithDescription(description).WithSchema(openapi3.NewStringSchema())
	p.Example = example
	return &openapi3.ParameterRef{Value: p}
}

func queryParam(name, description string, example any) *openapi3.ParameterRef {
	p := openapi3.NewQueryParameter(name).WithDescription(description).WithSchema(openapi3.NewStringSchema())
	p.Example = example
	return &openapi3.ParameterRef{Value: p}
}

// exampleJSON round-trips a typed example through JSON so the spec carries
// exactly what the handler would serialize. The inputs are static DTO values,
// so a marshal failure is a programming error.
func exampleJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal OpenAPI example %T: %v", v, err))
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("unmarshal OpenAPI example %T: %v", v, err))
	}
	return out
}

func int64Ptr(v int64) *int64 { return &v }

func boolPtr(v bool) *bool { return &v }

// managementOpenAPISpec serves the Management API spec. It backs both the
// token-gated /api/mgmt/openapi.json and the admin-only panel route.
func (h *Handlers) managementOpenAPISpec(e *core.RequestEvent) error {
	spec, err := BuildManagementOpenAPISpec(h.publicConfig.APIURL)
	if err != nil {
		return err
	}
	return writeSpecJSON(e, spec)
}
