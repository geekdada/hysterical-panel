# Management API OpenAPI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve a Management API OpenAPI spec generated in Go, fetchable with the mgmt token or by an admin, and render `/settings/management-api` from it.

**Architecture:** A route table `managementRoutes()` drives both `/api/mgmt` registration and a drift test against `BuildManagementOpenAPISpec()`. The spec reuses the DTO schema generator from `openapi.go`, and its examples are typed Go DTO values. Two handlers serve the same spec: one inside the token-gated `/api/mgmt` group, one under `/api/panel` for admins. The frontend fetches the panel copy and renders it with small local helpers.

**Tech Stack:** Go, PocketBase v0.39.0 router, kin-openapi (`openapi3`, `openapi3gen`), TanStack Start, react-query, Paraglide, `openapi-typescript` (type-only import).

**Spec:** `docs/superpowers/specs/2026-10-19-management-api-openapi-design.md`

## Global Constraints

- `/api/mgmt/*` stays out of `/api/openapi.json`. Only the admin route `/api/panel/management-api/openapi.json` enters the public spec.
- Operation summaries and descriptions are English plain text, no Markdown. Multi-rule descriptions use one `- ` line per rule.
- Example values are fake: `rec_8f3k2j1d0s9a7gx`, `user@example.com`, `k2m9x7c4v1b8n3q`, `tokyo-1.example.com`. No real token, secret or auth string.
- Page chrome copy goes through Paraglide in both `en` and `zh-cn`. Alert copy has no trailing `.` or `。`.
- Backend: `go build ./...`, `go vet ./...`, `make test` all clean. Frontend: `pnpm typecheck`, `pnpm i18n:check`, `pnpm build` all clean.
- Datetimes in examples are UTC ISO strings with `Z`.

## Review Focus

1. Feature disabled: `GET /api/mgmt/openapi.json` returns 404, like every other mgmt route. Pinned in Task 2 with a router-level test.
2. `PANEL_BACKEND_URL_BASE` unset: the spec has no `servers` entry, and the page's curl commands still use `usePanelApiOrigin()`. Pinned in Task 1.
3. A DTO gains a field or changes a type: typed examples follow the struct, and the example-validation test catches type or nullability mismatches. Pinned in Task 1.
4. An admin opens the page while mgmt is disabled: the panel route still returns the spec. Pinned in Task 2.
5. The spec request fails on the page (network, 403): the page shows an `ErrorAlert`, and the title and auth sections still render. Checked manually in Task 3, since the frontend has no test runner.

---

### Task 1: Management spec builder and route table

**Files:**
- Create: `backend/internal/api/openapi_management.go`
- Create: `backend/internal/api/openapi_management_test.go`
- Modify: `backend/internal/api/openapi.go` (extract the per-type schema loop at lines 110-121)
- Modify: `backend/internal/api/dto.go` (add exported mgmt user DTOs)
- Modify: `backend/internal/api/management_api.go` (use the exported DTOs)
- Modify: `backend/internal/api/api.go:232-245` (register mgmt routes from the table)

**Interfaces:**
- Produces:
  - `func generateSchemas(defs map[string]any) (openapi3.Schemas, error)` in `openapi.go`. `BuildOpenAPISpec` calls it in place of the inline loop.
  - `type ManagementUserCreateRequest struct { Email string \`json:"email"\` }` and `type ManagementUserCreateResponse struct { ID, Email, Status string }` (json `id`, `email`, `status`) in `dto.go`. Delete `mgmtCreateUserRequest` / `mgmtCreateUserResponse`.
  - `type managementRoute struct { method, path string; handler func(*core.RequestEvent) error }`. `path` is relative to `/api/mgmt`.
  - `func (h *Handlers) managementRoutes() []managementRoute` lists the 8 existing routes plus `{GET, "/openapi.json", h.managementOpenAPISpec}`. `h.managementOpenAPISpec` is defined in Task 2. Until then, add a stub in `openapi_management.go` that returns `apis.NewNotFoundError("", nil)`.
  - `func (h *Handlers) bindManagementRoutes(group *router.RouterGroup[*core.RequestEvent])` binds `h.requireMgmtToken()` on the group and calls `group.Route(r.method, r.path, r.handler)` for each entry. `api.go` creates the `/api/mgmt` group and calls it.
  - `func BuildManagementOpenAPISpec(serverURL string) (*openapi3.T, error)`.
  - `func exampleJSON(v any) any` marshals a typed value and unmarshals it into `any`, so kin-openapi validates plain JSON values.

- [ ] **Step 1: Write the failing tests** in `openapi_management_test.go`

```go
func TestManagementSpecMatchesRoutes(t *testing.T) {
	spec, err := BuildManagementOpenAPISpec("")
	// require no error
	routes := (&Handlers{}).managementRoutes()
	for _, r := range routes {
		item := spec.Paths.Find("/api/mgmt" + r.path)
		// item != nil && item.GetOperation(r.method) != nil, else t.Fatalf naming method+path
	}
	ops := 0
	for _, item := range spec.Paths.Map() { ops += len(item.Operations()) }
	// ops == len(routes)
}

func TestManagementSpecExamplesMatchSchemas(t *testing.T) {
	spec, _ := BuildManagementOpenAPISpec("https://panel.example.com")
	raw, _ := json.Marshal(spec)
	doc, err := openapi3.NewLoader().LoadFromData(raw) // resolves $ref
	// require no error; doc.Validate(context.Background()) == nil
	// For every operation: each requestBody and response media type with a non-nil
	// Example must pass media.Schema.Value.VisitJSON(media.Example).
	// Every 2xx JSON response must carry an Example (fail otherwise).
	// Every operation must have non-empty Summary, Description and exactly one tag.
}

func TestManagementSpecServers(t *testing.T) {
	without, _ := BuildManagementOpenAPISpec("")
	// len(without.Servers) == 0
	with, _ := BuildManagementOpenAPISpec("https://panel.example.com")
	// len(with.Servers) == 1 && with.Servers[0].URL == "https://panel.example.com"
}

func TestManagementSpecUsesBearerAuth(t *testing.T) {
	spec, _ := BuildManagementOpenAPISpec("")
	// spec.Components.SecuritySchemes["bearerAuth"].Value.Type == "http"
	// and .Scheme == "bearer"; spec.Security == [{"bearerAuth": {}}]
	// spec.OpenAPI == "3.1.0"; spec.Info.Title == "Hysterical Panel Management API"
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/api -run TestManagementSpec`
Expected: FAIL to compile (`BuildManagementOpenAPISpec` undefined).

- [ ] **Step 3: Extract `generateSchemas`, move the mgmt user DTOs and add the route table**

`api.go` keeps the existing comment above the group. Update its last sentence: the routes stay out of `openapi.go` and have their own spec in `openapi_management.go`.

- [ ] **Step 4: Implement `BuildManagementOpenAPISpec` in `openapi_management.go`**

Schemas, via `generateSchemas`: `Node`, `NodeCreateRequest`, `NodeUpdateRequest`, `NodeTestResponse`, `PanelUser`, `RecentConnection`, `ManagementUserCreateRequest`, `ManagementUserCreateResponse`, `DeleteResponse`, `ErrorResponse`. Set `ErrorResponse.example` to `exampleJSON(ErrorResponse{Status: 401, Message: "invalid token", Data: map[string]any{}})`. Task 3 reads that example for the page's error-format block.

Document level:
- `info.title` `Hysterical Panel Management API`, `info.version` `version.Version`.
- `info.description`: `Every request must send Authorization: Bearer <token>.\n- While the Management API is turned off, every endpoint returns 404.\n- A missing or invalid token returns 401.`
- `servers`: one entry with `serverURL` when it is non-empty, otherwise none.
- Security scheme `bearerAuth` (`type: http`, `scheme: bearer`), applied globally.
- Tags, in this order: `Users` (no description), `Nodes` (`Node responses never include api_secret. Every node endpoint treats a deleted node as missing.`), `Specification` (no description).

Every operation documents `401` (`Missing or invalid token`) as `ErrorResponse`. Operations, in this order. Descriptions are exact copy:

| Method + path | operationId | Tag | Success | Errors (status: description) |
|---|---|---|---|---|
| `GET /api/mgmt/users` | `mgmtGetUser` | Users | 200 `PanelUser` | 400: `More than one parameter, or none, was sent`; 404: `No user matches` |
| `POST /api/mgmt/users` | `mgmtCreateUser` | Users | 201 `ManagementUserCreateResponse` | 400: `Invalid body, missing email, or email already taken` |
| `GET /api/mgmt/nodes` | `mgmtListNodes` | Nodes | 200 array of `Node` | none |
| `POST /api/mgmt/nodes` | `mgmtCreateNode` | Nodes | 201 `Node` | 400: `A required field is missing or a value is invalid` |
| `GET /api/mgmt/nodes/{id}` | `mgmtGetNode` | Nodes | 200 `Node` | 404: `The node does not exist or has been deleted` |
| `PATCH /api/mgmt/nodes/{id}` | `mgmtUpdateNode` | Nodes | 200 `Node` | 400: `api_url changed without api_secret, api_secret is an empty string, or a value is invalid`; 404: as above |
| `DELETE /api/mgmt/nodes/{id}` | `mgmtDeleteNode` | Nodes | 200 `DeleteResponse` | 404: as above |
| `POST /api/mgmt/nodes/{id}/test` | `mgmtTestNode` | Nodes | 200 `NodeTestResponse` | 404: as above |
| `GET /api/mgmt/openapi.json` | `mgmtGetOpenAPISpec` | Specification | 200 generic object (no example needed; exempt it in the Step 1 example check by operationId) | none |

Summaries:
- `Look up one user`
- `Create a user`
- `List nodes`
- `Register a node`
- `Get a node`
- `Update, enable or disable a node`
- `Delete a node`
- `Test a node's Traffic Stats API`
- `Get this OpenAPI document`

Descriptions:
- `mgmtGetUser`: `Look up a single user by User ID (id), email, or Current Auth String (auth_string).\n- Provide exactly one of the three.\n- Retired Auth Strings do not match.\n- Connected Apps receive the User ID as sub, so look their users up by id.`
- `mgmtCreateUser`: `Create a user from an email.\n- The password and auth_string are generated server-side and never returned.\n- The account is created active and verified with the user role.\n- Send the new user through the password reset flow to grant access.`
- `mgmtListNodes`: `List every node that has not been deleted, newest first.`
- `mgmtCreateNode`: `Register a node.\n- name, api_url and api_secret are required.\n- poll_interval is in seconds and defaults to 30.\n- enabled defaults to true.`
- `mgmtGetNode`: `Look up a single node by ID.`
- `mgmtUpdateNode`: `Change any of name, api_url, api_secret, poll_interval and enabled. Omitted fields keep their values.\n- A new api_url needs a new api_secret in the same request, so the panel never sends the old secret to a different address.\n- api_secret cannot be an empty string.\n- Send enabled false to disable a node and enabled true to enable it again. The panel stops polling a disabled node, hides it from Users and leaves it out of kicks. Its Current Speed and Online Device Count drop to 0.\n- Clients can still connect through a disabled node until you stop the node service itself.`
- `mgmtDeleteNode`: `Soft-delete a node. The panel keeps its Traffic history, but every endpoint treats the node as missing from then on. A deleted node cannot be restored.`
- `mgmtTestNode`: `Check that the panel can reach the node's Traffic Stats API with the stored api_url and api_secret.\n- Disabled nodes can be tested too.\n- A failed check still returns 200, with ok set to false and the reason in error.`
- `mgmtGetOpenAPISpec`: `Return the OpenAPI document for the Management API as currently deployed.`

Parameters:
- `GET /users` query `id`, `email` (`example: "user@example.com"`), `auth_string`, all optional strings.
- `{id}` path parameter on the node item routes, `example: "k2m9x7c4v1b8n3q"`.

Examples, all built with `exampleJSON` from typed DTO values:
- `PanelUser`: `id` `rec_8f3k2j1d0s9a7gx`, `email` `user@example.com`, `role` `user`, `auth_string` `qZ4yN8tR2vL6wB1k`, `subscription_required` true, `status` `active`, `created` `2026-06-01T08:30:00.000Z`, `last_connected_at` `2026-06-02T10:15:00.000Z`, `recent_connections` one entry `{ip: "203.0.113.7", last_seen_at: "2026-06-02T10:15:00.000Z"}`, `used_tx` 1048576, `used_rx` 5242880.
- `ManagementUserCreateRequest` `{email: "user@example.com"}`. Response `{id: "rec_8f3k2j1d0s9a7gx", email: "user@example.com", status: "active"}`.
- `Node`: `id` `k2m9x7c4v1b8n3q`, `name` `Tokyo 1`, `api_url` `https://tokyo-1.example.com:25413`, `poll_interval` 30, `enabled` true, `last_polled_at` `2026-06-01T08:30:00.000Z`, `last_error` empty, `health` `ok`, speeds 131072 / 524288, `online_devices` 12. The list example is `[]Node{that}`.
- `NodeCreateRequest` `{name: "Tokyo 1", api_url: "https://tokyo-1.example.com:25413", api_secret: "NODE_API_SECRET"}`.
- `NodeUpdateRequest` `{enabled: false}`. The PATCH response is the node with `enabled` false, speeds 0 and `online_devices` 0.
- `DeleteResponse{Deleted: true}`, `NodeTestResponse{OK: true, LatencyMs: 42}`.

If `VisitJSON` rejects a value that openapi3gen typed differently (a nil slice as `null`, a pointer), fix the example value or the schema in the builder. Do not loosen the test.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/api -run 'TestManagementSpec|TestManagementAPI|TestOpenAPI' && go build ./... && go vet ./...`
Expected: PASS, no vet output. The existing `TestManagementAPI*` tests still pass after the DTO rename.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api
git commit -m "feat(backend): build an openapi spec for the management api"
```

### Task 2: Serve the spec and update backend docs

**Files:**
- Modify: `backend/internal/api/openapi_management.go` (replace the stub handler)
- Modify: `backend/internal/api/openapi.go` (shared JSON writer; add the panel route to the public spec)
- Modify: `backend/internal/api/api.go:97-100` (panel route)
- Modify: `backend/internal/api/openapi_management_test.go`
- Modify: `backend/README.md:250-271`
- Modify: `AGENTS.md` (decision #8, decision #10, the public-endpoint line about `/api/mgmt`, directory tree)

**Interfaces:**
- Consumes: `BuildManagementOpenAPISpec`, `bindManagementRoutes`, `managementRoutes` from Task 1.
- Produces:
  - `func (h *Handlers) managementOpenAPISpec(e *core.RequestEvent) error` builds the spec with `h.publicConfig.APIURL` and writes it as indented JSON. It uses the same headers as `handleOpenAPISpec` (`Content-Type: application/json`, `Cache-Control: no-cache`).
  - `func writeSpecJSON(e *core.RequestEvent, spec *openapi3.T) error` in `openapi.go`, shared by both handlers.
  - Panel route `GET /api/panel/management-api/openapi.json`, `adminOnly`, handler `h.managementOpenAPISpec`.
  - Public spec operation `getManagementAPISpec` on that path: 200 is an object schema with `additionalProperties: true`, plus 401 and 403. Task 3 calls it as `apiClient.GET("/api/panel/management-api/openapi.json")`.

- [ ] **Step 1: Write the failing tests**

```go
func TestManagementSpecRouteIsGated(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app, publicConfig: PanelConfigResponse{APIURL: "https://panel.example.com"}}
	r, _ := apis.NewRouter(app)
	h.bindManagementRoutes(r.Group("/api/mgmt"))
	mux, _ := r.BuildMux()
	get := func(auth string) *httptest.ResponseRecorder { /* GET /api/mgmt/openapi.json via mux */ }
	// feature disabled, good token -> 404
	// enable feature + token hash "good-token" (same setup as TestManagementAPIGateHidesDisabledFeatureAndRejectsBadTokens)
	// "Bearer wrong" -> 401
	// "Bearer good-token" -> 200, body decodes with openapi "3.1.0" and servers[0].url "https://panel.example.com"
}

func TestPanelManagementSpecWorksWhileFeatureDisabled(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}
	event, res := jsonRequestEvent(t, app, http.MethodGet, "/api/panel/management-api/openapi.json", nil)
	// h.managementOpenAPISpec(event) == nil; res.Code == 200; body has paths["/api/mgmt/nodes"]
}

func TestOpenAPIDescribesManagementSpecRoute(t *testing.T) {
	spec, _ := BuildOpenAPISpec()
	// spec.Paths.Find("/api/panel/management-api/openapi.json").Get.OperationID == "getManagementAPISpec"
	// no path in the public spec starts with "/api/mgmt"
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/api -run 'TestManagementSpecRoute|TestPanelManagementSpec|TestOpenAPIDescribesManagementSpecRoute'`
Expected: FAIL. The stub returns 404 on the 200 cases, and the public spec lacks the path.

- [ ] **Step 3: Implement `managementOpenAPISpec`, `writeSpecJSON`, the panel route and the public spec entry**

- [ ] **Step 4: Run the full backend checks**

Run: `cd backend && make test && go build ./... && go vet ./... && make openapi`
Expected: all pass. `openapi.json` contains `/api/panel/management-api/openapi.json` and no `/api/mgmt` path.

- [ ] **Step 5: Update docs**

- `backend/README.md`: under the Management API heading, keep the opening paragraph, the bearer and 404/401 paragraph, and the enable/rotate paragraph. Delete the endpoint table and the paragraph about node handlers reusing panel handlers. Add one paragraph after the opening one: the endpoint list, rules and examples come from `GET /api/mgmt/openapi.json` (same bearer token), and admins can read the rendered version at `/settings/management-api`. Drop the claim that the API is kept out of `openapi.json` "like `/api/hysteria/auth`" and say it has its own spec instead.
- `AGENTS.md` decision #8: replace `/api/mgmt/*` in the "intentionally kept out of the schema" list with a sentence saying `/api/mgmt/*` has its own spec from `BuildManagementOpenAPISpec()` (`openapi_management.go`), served at `/api/mgmt/openapi.json` (token) and `/api/panel/management-api/openapi.json` (admin), and still not part of `/api/openapi.json`.
- `AGENTS.md` decision #10: replace "前端文档页 `/settings/management-api` 与 backend/README.md 需同步维护" with: routes are declared in `managementRoutes()`; changing a mgmt route means updating the spec in `openapi_management.go`; tests check that routes and spec match and that examples fit their schemas.
- `AGENTS.md` public-endpoint section: change "契约见 backend/README.md" to "契约见 `GET /api/mgmt/openapi.json`".
- `AGENTS.md` directory tree: add `openapi_management.go  Management API 独立 OpenAPI spec + mgmt 路由表`.

- [ ] **Step 6: Commit**

```bash
git add backend AGENTS.md
git commit -m "feat(backend): serve the management api spec to token holders and admins"
```

### Task 3: Render the docs page from the spec

**Files:**
- Create: `frontend/src/lib/openapi-doc.ts`
- Create: `frontend/src/components/openapi-operation.tsx`
- Modify: `frontend/src/routes/settings/management-api.tsx`
- Modify: `frontend/src/api/queries.ts`
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`
- Modify: `AGENTS.md` directory tree (frontend `components/` and `lib/` lines)

**Interfaces:**
- Consumes: `GET /api/panel/management-api/openapi.json` from Task 2, after `pnpm api:sync`.
- Produces:
  - `queries.ts`: `fetchManagementApiSpec(): Promise<OpenAPI3>`, which calls `apiRequest` with `m.mgmt_api_spec_load_error()`. Also `managementApiSpecQueryOptions()` with key `queryKeys.managementApiSpec()` = `[...queryKeys.all, "management-api-spec"]`. `OpenAPI3` and the other OpenAPI types come from `import type { ... } from "openapi-typescript"` (already a devDependency; type-only, so nothing ships).
  - `openapi-doc.ts`:
    - `type HttpMethod = "get" | "post" | "put" | "patch" | "delete"`
    - `type DocOperation = { method: HttpMethod; path: string; operation: OperationObject }`
    - `type DocGroup = { name: string; description?: string; operations: DocOperation[] }`
    - `function resolveRef<T>(doc: OpenAPI3, value: T | ReferenceObject): T` follows local `#/components/...` refs until it reaches a non-ref value.
    - `function groupOperations(doc: OpenAPI3): DocGroup[]` groups by each operation's first tag. Groups follow the order of `doc.tags`, and operations keep the spec's path order and then method order (get, post, put, patch, delete).
    - `function curlCommand(origin: string, entry: DocOperation, doc: OpenAPI3): string`
    - `function exampleText(value: unknown): string`, which is `JSON.stringify(value, null, 2)`.
  - `openapi-operation.tsx`: `function OpenApiOperation({ doc, entry, origin }: { doc: OpenAPI3; entry: DocOperation; origin: string })`.

`curlCommand` must produce the same layout the page uses today, for example:

```text
curl -X PATCH "https://panel.example.com/api/mgmt/nodes/k2m9x7c4v1b8n3q" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled":false}'
```

Rules: omit `-X` for GET. Replace each path parameter with its `example`. Append a query string built from the query parameters that have an `example` (so `GET /users` gets `?email=user@example.com`). Add the `Content-Type` and `-d` lines only when the request body has an example, and serialize it with `JSON.stringify(example)`.

- [ ] **Step 1: Sync API types**

Run: `cd frontend && pnpm api:sync`
Expected: `src/api/schema.d.ts` has a `"/api/panel/management-api/openapi.json"` entry.

- [ ] **Step 2: Add the query and `openapi-doc.ts`**

- [ ] **Step 3: Build `OpenApiOperation`**

Move `Endpoint` and `CodeBlock` from the route file into this component, keeping their current classes. Inside an operation, render these in order:
1. Method badge (uppercase) and path.
2. Summary as the lead line.
3. Description in a `whitespace-pre-line` paragraph.
4. Parameters table (name, location, required, type, description), using the existing `Th` / `Td`, only when the operation has parameters.
5. Request body field table (name, required, type, description), only when there is a body. Resolve one level of `$ref`.
6. Request curl block.
7. One response example block per 2xx response that has an example, labeled `m.mgmt_api_label_response({ status })`.
8. The errors line, `m.mgmt_api_errors_prefix()` followed by each non-2xx response as `status description`, joined with `. `. Skip `401` because the auth section already covers it.

- [ ] **Step 4: Rewrite the route page**

Order:
1. Title and intro (unchanged).
2. Auth section (unchanged).
3. New spec section: heading `m.mgmt_api_spec_heading()`, paragraph `m.mgmt_api_spec_intro()`, and a `CodeBlock` with `curl "<origin>/api/mgmt/openapi.json" \` + newline + `  -H "Authorization: Bearer YOUR_TOKEN"`.
4. While loading, `TableSkeleton`. On error, `ErrorAlert` with the query error message.
5. For each `DocGroup`: a `GroupHeading` with the tag name, the tag description in a muted paragraph if present, then an `OpenApiOperation` per entry. Skip the `Specification` group, because section 3 already shows that request. Filter it by tag name in the route file.
6. Error-format section (unchanged copy), with the example taken from `resolveRef(doc, doc.components.schemas.ErrorResponse).example`. Render it only after the spec loads.

Delete the hard-coded JSON constants and per-endpoint curl strings.

- [ ] **Step 5: Update messages in both locales**

Delete: `mgmt_api_get_summary`, `mgmt_api_get_errors`, `mgmt_api_post_summary`, `mgmt_api_post_errors`, `mgmt_api_users_heading`, `mgmt_api_nodes_heading`, `mgmt_api_nodes_intro`, `mgmt_api_nodes_list_summary`, `mgmt_api_nodes_get_summary`, `mgmt_api_nodes_not_found_errors`, `mgmt_api_nodes_create_summary`, `mgmt_api_nodes_create_errors`, `mgmt_api_nodes_update_summary`, `mgmt_api_nodes_update_enabled`, `mgmt_api_nodes_update_errors`, `mgmt_api_nodes_delete_summary`, `mgmt_api_nodes_test_summary`, `mgmt_api_label_response_200`, `mgmt_api_label_response_201`.

Add:

| Key | en | zh-cn |
|---|---|---|
| `mgmt_api_spec_heading` | `OpenAPI spec` | `OpenAPI 规范` |
| `mgmt_api_spec_intro` | `Request this spec with your token to get the same document this page shows. Feed it to code generators, API clients or AI agents.` | `用你的 token 请求这份规范，拿到的就是本页展示的文档，可以直接交给代码生成器、API 客户端或 AI agent。` |
| `mgmt_api_spec_load_error` | `Couldn't load the API spec` | `无法加载 API 规范` |
| `mgmt_api_label_response` | `Response · {status}` | `响应 · {status}` |
| `mgmt_api_label_parameters` | `Parameters` | `参数` |
| `mgmt_api_label_body` | `Request body` | `请求体` |
| `mgmt_api_col_name` | `Name` | `名称` |
| `mgmt_api_col_location` | `In` | `位置` |
| `mgmt_api_col_required` | `Required` | `必填` |
| `mgmt_api_col_type` | `Type` | `类型` |
| `mgmt_api_col_description` | `Description` | `说明` |
| `mgmt_api_required_yes` | `Yes` | `是` |

Before deleting, grep `src/` to confirm the deleted keys are used only in `management-api.tsx`.

- [ ] **Step 6: Run frontend checks**

Run: `cd frontend && pnpm i18n:check && pnpm typecheck && pnpm build`
Expected: all pass.

- [ ] **Step 7: Check the page in a running app**

Start with `pnpm dev` from the repo root and sign in as admin. Then check:
- With mgmt disabled, the page renders the Users and Nodes groups with all 8 operations.
- The PATCH node curl matches the example above.
- The `GET /users` curl includes `?email=user@example.com`.
- Switching to zh-cn changes the chrome only, and operation text stays English.
- Dark mode is legible.
- With the backend stopped, a reload shows the error alert under the auth section, and the auth section is still visible.
- `curl -H "Authorization: Bearer <token>" http://localhost:8090/api/mgmt/openapi.json` returns 404 while disabled, and returns the spec after enabling and using the new token.

- [ ] **Step 8: Update the AGENTS.md tree and commit**

Add `openapi-operation.tsx` to the `components/` list. The `lib/` line already says "工具 helper", so leave it unless `openapi-doc.ts` needs naming.

```bash
git add frontend AGENTS.md
git commit -m "feat(frontend): render the management api page from its openapi spec"
```
