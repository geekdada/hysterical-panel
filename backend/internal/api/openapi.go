package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/version"
)

// BuildOpenAPISpec constructs an OpenAPI 3.1 document describing every
// /api/panel/* route. The spec is self-contained (no external refs) and can
// be served as JSON or written to a file for offline consumption.
func BuildOpenAPISpec() (*openapi3.T, error) {

	// ── schemas from DTO structs ──────────────────────────────────────────
	schemaDefs := map[string]any{
		"Node":                              Node{},
		"NodeCreateRequest":                 NodeCreateRequest{},
		"NodeUpdateRequest":                 NodeUpdateRequest{},
		"NodeTestResponse":                  NodeTestResponse{},
		"NodeAPISecretResetResponse":        NodeAPISecretResetResponse{},
		"NotificationChannel":               NotificationChannel{},
		"NotificationChannelCreateRequest":  NotificationChannelCreateRequest{},
		"NotificationChannelUpdateRequest":  NotificationChannelUpdateRequest{},
		"NotificationChannelTestResponse":   NotificationChannelTestResponse{},
		"NotificationChannelRevealResponse": NotificationChannelRevealResponse{},
		"PanelUser":                         PanelUser{},
		"RecentConnection":                  RecentConnection{},
		"UserListResponse":                  UserListResponse{},
		"UserStatsResponse":                 UserStatsResponse{},
		"UserCreateRequest":                 UserCreateRequest{},
		"UserUpdateRequest":                 UserUpdateRequest{},
		"Passkey":                           Passkey{},
		"PasskeyOptionsResponse":            PasskeyOptionsResponse{},
		"PasskeyFinishRequest":              PasskeyFinishRequest{},
		"PanelAuthResponse":                 PanelAuthResponse{},
		"PanelTrafficResponse":              PanelTrafficResponse{},
		"PanelNodeTrafficResponse":          PanelNodeTrafficResponse{},
		"TrafficSummaryResponse":            TrafficSummaryResponse{},
		"TrafficSeriesResponse":             TrafficSeriesResponse{},
		"NodeTrafficSummaryResponse":        NodeTrafficSummaryResponse{},
		"DatabaseStatsResponse":             DatabaseStatsResponse{},
		"DatabasePruneResponse":             DatabasePruneResponse{},
		"LiveResponse":                      LiveResponse{},
		"NodeLiveResponse":                  NodeLiveResponse{},
		"DeleteResponse":                    DeleteResponse{},
		"ErrorResponse":                     ErrorResponse{},
		"PanelConfigResponse":               PanelConfigResponse{},
		"RegisterRequest":                   RegisterRequest{},
		"RegisterResponse":                  RegisterResponse{},
		"Invitation":                        Invitation{},
		"InvitationCreateRequest":           InvitationCreateRequest{},
		"IgnoredConnectionIP":               IgnoredConnectionIP{},
		"IgnoredConnectionIPCreateRequest":  IgnoredConnectionIPCreateRequest{},
		"SettingsResponse":                  SettingsResponse{},
		"SettingsUpdateRequest":             SettingsUpdateRequest{},
		"ManagementAPITokenResponse":        ManagementAPITokenResponse{},
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
		setEnum(s.Value.Properties, "role", []any{"admin", "user"})
		setEnum(s.Value.Properties, "status", []any{"active", "disabled"})
	}
	if s, ok := schemas["TrafficSeriesResponse"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "granularity", []any{"hourly", "daily"})
	}
	if s, ok := schemas["NotificationChannel"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "service", []any{
			"generic", "bark", "discord", "gotify", "googlechat", "ifttt", "join", "lark",
			"mattermost", "matrix", "mqtt", "ntfy", "opsgenie", "pushbullet", "pushover",
			"rocketchat", "signal", "slack", "teams", "telegram", "twilio", "wecom", "zulip",
		})
		setEnum(s.Value.Properties, "last_test_status", []any{"never", "succeeded", "failed"})
		setEnum(s.Value.Properties, "last_test_error", []any{"timed_out", "delivery_failed"})
	}
	if s, ok := schemas["NotificationChannelTestResponse"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "status", []any{"succeeded", "failed"})
		setEnum(s.Value.Properties, "error", []any{"timed_out", "delivery_failed"})
	}
	// ── doc skeleton ──────────────────────────────────────────────────────
	t := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:   "Hysterical Panel API",
			Version: version.Version,
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
	forbidden := errRef(403, "Forbidden")
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
	passkeyIdParam := &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "passkeyId",
			In:       "path",
			Required: true,
			Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		},
	}
	dateTimeQueryParam := func(name, desc string, required bool) *openapi3.ParameterRef {
		return &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        name,
				In:          "query",
				Required:    required,
				Description: desc,
				Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
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

	// ── /traffic ──────────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/traffic", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "panelTraffic",
				Summary:     "Get global traffic total for a UTC datetime range",
				Tags:        []string{"traffic"},
				Parameters: openapi3.Parameters{
					{
						Value: &openapi3.Parameter{
							Name:        "from",
							In:          "query",
							Required:    true,
							Description: "Start datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "to",
							In:          "query",
							Required:    true,
							Description: "End datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
				},
				Responses: openapi3.NewResponses(
					openapi3.WithStatus(200, &openapi3.ResponseRef{
						Value: &openapi3.Response{
							Description: ptr("Global traffic total for the requested range"),
							Content:     content(ref("PanelTrafficResponse")),
						},
					}),
					openapi3.WithStatus(400, badRequest),
				),
			}
			withAuth(op)
			return op
		}(),
	})

	// ── /traffic/series ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/traffic/series", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "panelTrafficSeries",
				Summary:     "Get global traffic time series",
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
							Required:    true,
							Description: "Start datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "to",
							In:          "query",
							Required:    true,
							Description: "End datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
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
			withAuth(op)
			return op
		}(),
	})

	// ── /nodes/traffic/summary ───────────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/traffic/summary", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "panelNodeTrafficSummary",
				Summary:     "Get per-node traffic totals for a UTC datetime range",
				Tags:        []string{"traffic"},
				Parameters: openapi3.Parameters{
					{
						Value: &openapi3.Parameter{
							Name:        "from",
							In:          "query",
							Required:    true,
							Description: "Start datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "to",
							In:          "query",
							Required:    true,
							Description: "End datetime (UTC, inclusive)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("date-time")},
						},
					},
				},
				Responses: openapi3.NewResponses(
					openapi3.WithStatus(200, &openapi3.ResponseRef{
						Value: &openapi3.Response{
							Description: ptr("Per-node traffic totals for the requested range"),
							Content:     content(ref("PanelNodeTrafficResponse")),
						},
					}),
					openapi3.WithStatus(400, badRequest),
				),
			}
			withAuth(op)
			return op
		}(),
	})

	// ── /database/stats ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/database/stats", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "databaseStats",
				Summary:     "Get database storage and traffic table statistics",
				Tags:        []string{"database"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Database statistics"),
						Content:     content(ref("DatabaseStatsResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /database/prune ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/database/prune", &openapi3.PathItem{
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "pruneDatabaseTraffic",
				Summary:     "Delete traffic data older than 30 days",
				Tags:        []string{"database"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Deleted row counts"),
						Content:     content(ref("DatabasePruneResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

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
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "getNode",
				Summary:     "Get node detail",
				Tags:        []string{"nodes"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Node detail"),
						Content:     content(ref("Node")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
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

	// ── /nodes/{id}/reset-api-secret ──────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/{id}/reset-api-secret", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Node ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "resetNodeAPISecret",
				Summary:     "Regenerate a node's Hysteria Stats API secret (admin)",
				Tags:        []string{"nodes"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("New api_secret (shown once) and updated node"),
						Content:     content(ref("NodeAPISecretResetResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /nodes/{id}/traffic/summary ───────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/{id}/traffic/summary", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Node ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "nodeTrafficSummary",
				Summary:     "Get node-wide traffic summary for a UTC datetime range",
				Tags:        []string{"traffic"},
				Parameters: openapi3.Parameters{
					dateTimeQueryParam("from", "Start datetime (UTC, inclusive)", true),
					dateTimeQueryParam("to", "End datetime (UTC, inclusive)", true),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Node traffic summary"),
						Content:     content(ref("NodeTrafficSummaryResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /nodes/{id}/traffic/series ────────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/{id}/traffic/series", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Node ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "nodeTrafficSeries",
				Summary:     "Get node-wide traffic time series",
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

	// ── /nodes/{id}/live ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/nodes/{id}/live", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Node ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "nodeLive",
				Summary:     "On-demand diagnostics for a node",
				Tags:        []string{"live"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Live diagnostics"),
						Content:     content(ref("NodeLiveResponse")),
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
				Summary:     "List users (paginated)",
				Tags:        []string{"users"},
				Parameters: openapi3.Parameters{
					{
						Value: &openapi3.Parameter{
							Name:        "page",
							In:          "query",
							Description: "Page number (1-based, default 1)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema().WithDefault(1)},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "per_page",
							In:          "query",
							Description: "Page size (25, 50, or 100; default 25)",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema().WithDefault(25)},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "search",
							In:          "query",
							Description: "Filter by email, role, or status substring; auth_string exact match only",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
						},
					},
					{
						Value: &openapi3.Parameter{
							Name:        "sort",
							In:          "query",
							Description: "Sort field (default created). Prefix with - for descending.",
							Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithDefault("created")},
						},
					},
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Paginated user list"),
						Content:     content(ref("UserListResponse")),
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

	t.Paths.Set("/api/panel/users/stats", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "getUserStats",
				Summary:     "Get user count totals",
				Tags:        []string{"users"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("User totals"),
						Content:     content(ref("UserStatsResponse")),
					},
				})),
			}
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
				Summary:     "Get user detail (admin or self)",
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

	// ── /users/{id}/reset-auth-string ─────────────────────────────────────
	t.Paths.Set("/api/panel/users/{id}/reset-auth-string", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "resetUserAuthString",
				Summary:     "Regenerate a user's Hysteria auth key (admin)",
				Tags:        []string{"users"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Updated user with the new auth_string"),
						Content:     content(ref("PanelUser")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── passkeys ─────────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/passkeys/login/options", &openapi3.PathItem{
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "passkeyLoginOptions",
				Summary:     "Start passkey login",
				Tags:        []string{"passkeys"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Passkey login challenge"),
						Content:     content(ref("PasskeyOptionsResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Security = &openapi3.SecurityRequirements{}
			return op
		}(),
	})

	t.Paths.Set("/api/panel/passkeys/login/finish", &openapi3.PathItem{
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "passkeyLoginFinish",
				Summary:     "Finish passkey login",
				Tags:        []string{"passkeys"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("PasskeyFinishRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("PocketBase auth response"),
						Content:     content(ref("PanelAuthResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("403", forbidden)
			op.Security = &openapi3.SecurityRequirements{}
			return op
		}(),
	})

	t.Paths.Set("/api/panel/users/{id}/passkeys", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listPasskeys",
				Summary:     "List passkeys for a user (admin or self)",
				Tags:        []string{"passkeys"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Passkey list"),
						Content:     content(arrayRef("Passkey")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/users/{id}/passkeys/registration/options", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "passkeyRegistrationOptions",
				Summary:     "Start self passkey enrollment",
				Tags:        []string{"passkeys"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Passkey registration challenge"),
						Content:     content(ref("PasskeyOptionsResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/users/{id}/passkeys/registration/finish", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "passkeyRegistrationFinish",
				Summary:     "Finish self passkey enrollment",
				Tags:        []string{"passkeys"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("PasskeyFinishRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Created passkey"),
						Content:     content(ref("Passkey")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/users/{id}/passkeys/{passkeyId}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID"), passkeyIdParam},
		Delete: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "deletePasskey",
				Summary:     "Delete a passkey (admin or self)",
				Tags:        []string{"passkeys"},
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
				Summary:     "Get user traffic summary for a UTC datetime range (admin or self)",
				Tags:        []string{"traffic"},
				Parameters: openapi3.Parameters{
					dateTimeQueryParam("from", "Start datetime (UTC, inclusive)", true),
					dateTimeQueryParam("to", "End datetime (UTC, inclusive)", true),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Traffic summary"),
						Content:     content(ref("TrafficSummaryResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
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
				Summary:     "Get user traffic time series (admin or self)",
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

	// ── /config ─────────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/config", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "panelConfig",
				Summary:     "Public panel configuration",
				Tags:        []string{"config"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Public panel URLs and version"),
						Content:     content(ref("PanelConfigResponse")),
					},
				})),
			}
			op.Security = &openapi3.SecurityRequirements{}
			return op
		}(),
	})

	// ── /users/{id}/live ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/users/{id}/live", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "userLive",
				Summary:     "Real-time diagnostics for a user (admin)",
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

	// ── /register (public) ─────────────────────────────────────────────────
	t.Paths.Set("/api/panel/register", &openapi3.PathItem{
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "register",
				Summary:     "Self-service registration",
				Tags:        []string{"registration"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("RegisterRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Auth response, or requires_verification when an email must be confirmed"),
						Content:     content(ref("RegisterResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("403", forbidden)
			op.Security = &openapi3.SecurityRequirements{}
			return op
		}(),
	})

	// ── /settings ──────────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/settings", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "getSettings",
				Summary:     "Get registration / invitation feature flags",
				Tags:        []string{"settings"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Current settings"),
						Content:     content(ref("SettingsResponse")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Patch: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "updateSettings",
				Summary:     "Update registration / invitation feature flags",
				Tags:        []string{"settings"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("SettingsUpdateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Updated settings"),
						Content:     content(ref("SettingsResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /management-api/rotate ───────────────────────────────────────────
	t.Paths.Set("/api/panel/management-api/rotate", &openapi3.PathItem{
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "rotateManagementAPIToken",
				Summary:     "Generate a new Management API token, replacing any existing one",
				Tags:        []string{"settings"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("New management API token (shown once)"),
						Content:     content(ref("ManagementAPITokenResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /notification-channels ────────────────────────────────────────────
	t.Paths.Set("/api/panel/notification-channels", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listNotificationChannels",
				Summary:     "List notification channel metadata",
				Tags:        []string{"notification channels"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Notification channels without URLs or credentials"),
						Content:     content(arrayRef("NotificationChannel")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "createNotificationChannel",
				Summary:     "Create an encrypted notification channel",
				Tags:        []string{"notification channels"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("NotificationChannelCreateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Created channel metadata without its URL"),
						Content:     content(ref("NotificationChannel")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/notification-channels/{id}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Notification channel ID")},
		Patch: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "updateNotificationChannel",
				Summary:     "Update notification channel metadata or replace its URL",
				Tags:        []string{"notification channels"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("NotificationChannelUpdateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Updated channel metadata without its URL"),
						Content:     content(ref("NotificationChannel")),
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
				OperationID: "deleteNotificationChannel",
				Summary:     "Permanently delete a notification channel",
				Tags:        []string{"notification channels"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Deleted channel confirmation"),
						Content:     content(ref("DeleteResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/notification-channels/{id}/test", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Notification channel ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "testNotificationChannel",
				Summary:     "Send a fixed notification-channel test message",
				Tags:        []string{"notification channels"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Persisted test outcome; provider delivery failures are reported in the body"),
						Content:     content(ref("NotificationChannelTestResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/notification-channels/{id}/reveal/options", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Notification channel ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "notificationChannelRevealOptions",
				Summary:     "Start a passkey verification to reveal one notification channel URL",
				Tags:        []string{"notification channels", "passkeys"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Scoped passkey challenge"),
						Content:     content(ref("PasskeyOptionsResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/notification-channels/{id}/reveal/finish", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Notification channel ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "notificationChannelRevealFinish",
				Summary:     "Finish a scoped passkey verification and reveal one URL",
				Tags:        []string{"notification channels", "passkeys"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("PasskeyFinishRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("The selected channel URL after a fresh passkey assertion"),
						Content:     content(ref("NotificationChannelRevealResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	// ── /invitations ───────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/invitations", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listInvitations",
				Summary:     "List invitations",
				Tags:        []string{"invitations"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Invitation list"),
						Content:     content(arrayRef("Invitation")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "createInvitation",
				Summary:     "Create an invitation (optionally email it)",
				Tags:        []string{"invitations"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(false).
						WithJSONSchemaRef(ref("InvitationCreateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Created invitation"),
						Content:     content(ref("Invitation")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /invitations/{id} ──────────────────────────────────────────────────
	t.Paths.Set("/api/panel/invitations/{id}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Invitation ID")},
		Delete: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "deleteInvitation",
				Summary:     "Delete an invitation",
				Tags:        []string{"invitations"},
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

	// ── /ignored-connection-ips ────────────────────────────────────────────
	t.Paths.Set("/api/panel/ignored-connection-ips", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listIgnoredConnectionIPs",
				Summary:     "List globally ignored connection IPs",
				Tags:        []string{"ignored-connection-ips"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Ignored connection IP list"),
						Content:     content(arrayRef("IgnoredConnectionIP")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "createIgnoredConnectionIP",
				Summary:     "Ignore a connection IP globally",
				Tags:        []string{"ignored-connection-ips"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("IgnoredConnectionIPCreateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Ignored connection IP (existing or newly created)"),
						Content:     content(ref("IgnoredConnectionIP")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	// ── /ignored-connection-ips/{id} ───────────────────────────────────────
	t.Paths.Set("/api/panel/ignored-connection-ips/{id}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Ignored connection IP ID")},
		Delete: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "deleteIgnoredConnectionIP",
				Summary:     "Remove a globally ignored connection IP",
				Tags:        []string{"ignored-connection-ips"},
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
