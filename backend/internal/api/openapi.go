package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/pocketbase/pocketbase/core"
)

// BuildOpenAPISpec constructs an OpenAPI 3.1 document describing every
// /api/panel/* route. The spec is self-contained (no external refs) and can
// be served as JSON or written to a file for offline consumption.
func BuildOpenAPISpec() (*openapi3.T, error) {

	// ── schemas from DTO structs ──────────────────────────────────────────
	schemaDefs := map[string]any{
		"Node":                  Node{},
		"NodeCreateRequest":     NodeCreateRequest{},
		"NodeUpdateRequest":     NodeUpdateRequest{},
		"NodeTestResponse":      NodeTestResponse{},
		"PanelUser":             PanelUser{},
		"UserCreateRequest":     UserCreateRequest{},
		"UserUpdateRequest":     UserUpdateRequest{},
		"TrafficSummaryResponse": TrafficSummaryResponse{},
		"TrafficSeriesResponse":  TrafficSeriesResponse{},
		"LiveResponse":          LiveResponse{},
		"DeleteResponse":        DeleteResponse{},
		"ErrorResponse":         ErrorResponse{},
	}

	// Generate each schema with its own generator to avoid shared internal
	// schema objects leaking enums between types.
	schemas := make(openapi3.Schemas, len(schemaDefs))
	for name, typ := range schemaDefs {
		g := openapi3gen.NewGenerator()
		ref, err := g.NewSchemaRefForValue(typ, nil)
		if err != nil {
			return nil, fmt.Errorf("schema %q: %w", name, err)
		}
		schemas[name] = ref
	}

	// Add enum constraints that struct tags can't express.
	// openapi3gen reuses the same *SchemaRef for same-type fields, so we must
	// replace the Properties map entry rather than mutating the shared pointer.
	setEnum := func(props openapi3.Schemas, field string, enums []any) {
		p, ok := props[field]
		if !ok || p == nil || p.Value == nil {
			return
		}
		props[field] = &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type: p.Value.Type,
				Enum: enums,
			},
		}
	}
	if s, ok := schemas["Node"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "health", []any{"ok", "error", "never"})
	}
	if s, ok := schemas["PanelUser"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "role", []any{"admin"})
		setEnum(s.Value.Properties, "status", []any{"active", "disabled"})
	}
	if s, ok := schemas["TrafficSeriesResponse"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "granularity", []any{"hourly", "daily"})
	}

	// ── doc skeleton ──────────────────────────────────────────────────────
	t := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:   "Hysterical Panel API",
			Version: "0.1.0",
		},
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: schemas,
			SecuritySchemes: openapi3.SecuritySchemes{
				"cookieAuth": &openapi3.SecuritySchemeRef{
					Value: &openapi3.SecurityScheme{
						Type: "apiKey",
						Name: "pb_auth",
						In:   "cookie",
					},
				},
			},
		},
		Security: openapi3.SecurityRequirements{
			{"cookieAuth": {}},
		},
	}

	// ── helpers ───────────────────────────────────────────────────────────
	ref := func(name string) *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Ref: fmt.Sprintf("#/components/schemas/%s", name)}
	}

	content := func(schemaRef *openapi3.SchemaRef) openapi3.Content {
		return openapi3.Content{
			"application/json": &openapi3.MediaType{Schema: schemaRef},
		}
	}

	arrayRef := func(name string) *openapi3.SchemaRef {
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:  &openapi3.Types{"array"},
				Items: &openapi3.SchemaRef{Ref: fmt.Sprintf("#/components/schemas/%s", name)},
			},
		}
	}

	errRef := func(code int, desc string) *openapi3.ResponseRef {
		return &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: &desc,
				Content:     content(ref("ErrorResponse")),
			},
		}
	}
	unauthorized := errRef(401, "Authentication required")
	forbidden := errRef(403, "Admin role required")
	notFound := errRef(404, "Not found")
	badRequest := errRef(400, "Bad request")

	idParam := func(desc string) *openapi3.ParameterRef {
		return &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:     "id",
				In:       "path",
				Required: true,
				Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
			},
		}
	}

	withAuth := func(op *openapi3.Operation) {
		if op.Responses == nil {
			op.Responses = openapi3.NewResponses()
		}
		op.Responses.Set("401", unauthorized)
		op.Responses.Set("403", forbidden)
	}

	// ── /nodes ────────────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/nodes", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listNodes",
				Summary:     "List all nodes",
				Tags:        []string{"nodes"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Node list"),
						Content:     content(arrayRef("Node")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "createNode",
				Summary:     "Create a new node",
				Tags:        []string{"nodes"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("NodeCreateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Created node"),
						Content:     content(ref("Node")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /nodes/{id} ───────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/{id}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Node ID")},
		Patch: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "updateNode",
				Summary:     "Update a node",
				Tags:        []string{"nodes"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("NodeUpdateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Updated node"),
						Content:     content(ref("Node")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
		Delete: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "deleteNode",
				Summary:     "Delete a node",
				Tags:        []string{"nodes"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Deletion confirmed"),
						Content:     content(ref("DeleteResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /nodes/{id}/test ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/{id}/test", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Node ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "testNode",
				Summary:     "Test node connectivity",
				Tags:        []string{"nodes"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Test result"),
						Content:     content(ref("NodeTestResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /users ────────────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/users", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listUsers",
				Summary:     "List all users",
				Tags:        []string{"users"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("User list"),
						Content:     content(arrayRef("PanelUser")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "createUser",
				Summary:     "Create a new user",
				Tags:        []string{"users"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("UserCreateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Created user"),
						Content:     content(ref("PanelUser")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /users/{id} ───────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/users/{id}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "getUser",
				Summary:     "Get user detail",
				Tags:        []string{"users"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("User detail"),
						Content:     content(ref("PanelUser")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
		Patch: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "updateUser",
				Summary:     "Update a user",
				Tags:        []string{"users"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("UserUpdateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Updated user"),
						Content:     content(ref("PanelUser")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
		Delete: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "deleteUser",
				Summary:     "Delete a user",
				Tags:        []string{"users"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Deletion confirmed"),
						Content:     content(ref("DeleteResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /users/{id}/traffic/summary ───────────────────────────────────────
	t.Paths.Set("/api/panel/users/{id}/traffic/summary", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "trafficSummary",
				Summary:     "Get user traffic summary",
				Tags:        []string{"traffic"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Traffic summary"),
						Content:     content(ref("TrafficSummaryResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /users/{id}/traffic/series ────────────────────────────────────────
	t.Paths.Set("/api/panel/users/{id}/traffic/series", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "trafficSeries",
				Summary:     "Get user traffic time series",
				Tags:        []string{"traffic"},
				Parameters: openapi3.Parameters{
					{
						Value: &openapi3.Parameter{
							Name:        "granularity",
							In:          "query",
							Description: "Time bucket granularity",
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
									Enum: []any{"hourly", "daily"},
								},
							},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "from",
							In:          "query",
							Description: "Start datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "to",
							In:          "query",
							Description: "End datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "node",
							In:          "query",
							Description: "Filter by node ID",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
						},
					},
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Traffic series"),
						Content:     content(ref("TrafficSeriesResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /users/{id}/live ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/users/{id}/live", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "userLive",
				Summary:     "Real-time diagnostics for a user",
				Tags:        []string{"live"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Live diagnostics"),
						Content:     content(ref("LiveResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	return t, nil
}

// MarshalOpenAPISpec builds the spec and returns it as pretty-printed JSON.
func MarshalOpenAPISpec() ([]byte, error) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(spec, "", "  ")
}

func ptr(s string) *string { return &s }

// handleOpenAPISpec serves the generated OpenAPI JSON document.
func handleOpenAPISpec(e *core.RequestEvent) error {
	b, err := MarshalOpenAPISpec()
	if err != nil {
		return err
	}
	e.Response.Header().Set("Content-Type", "application/json")
	e.Response.Header().Set("Cache-Control", "no-cache")
	e.Response.WriteHeader(http.StatusOK)
	_, err = e.Response.Write(b)
	return err
}
