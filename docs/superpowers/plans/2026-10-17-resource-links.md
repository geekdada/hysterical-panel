# Resource Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admins curate an ordered list of off-panel Resource Links. Every User sees the list above the subscription area of their user page. Clicks go through a public `/l/:id` that counts the visit, and favicons are served from `/l/:id/favicon`.

**Architecture:** On save, the backend fetches the target page once to extract a title and favicon. Both are stored in a new `resource_links` collection, with the favicon bytes base64-encoded in a hidden field. Two public backend endpoints, `visit` and `favicon`, sit behind thin TanStack Start server routes on the frontend at `/l/$id` and `/l/$id/favicon`, so deployments need no reverse-proxy changes. The user page and the admin settings page both use HeroUI `ListBox`. The admin list reorders with React Aria `useDragAndDrop`.

**Tech Stack:** Go + PocketBase v0.39 (framework mode), `golang.org/x/net/html` (+ `html/charset`), kin-openapi; TanStack Start 1.168 server routes, HeroUI v3 `ListBox`, `react-aria-components` 1.18.0, TanStack Query/Form, Paraglide.

**Spec:** Inline below. It was agreed in the grill session of 2026-10-17. The domain term is in `CONTEXT.md` under **Resource Link**.

### Spec (agreed decisions)

- Every User sees the same links in the same admin-defined order. The admin viewing another User's page sees the identical list. There is no per-user visibility.
- Fields: target URL (required, `http`/`https` only), manual title (optional). There's no description and no enable toggle. Deleting a link deletes its visit count.
- Displayed title = manual title, otherwise the extracted title, otherwise the URL hostname.
- On create, and on update when the URL changes, the backend fetches the page synchronously with a 10s overall cap:
  - Title: `og:title`, otherwise `<title>`.
  - Favicon: the first `<link rel>` containing the token `icon`, otherwise `/favicon.ico` at the final page origin. Any `image/*` type is accepted, SVG included. Cap is 100 KiB.
  - Private-network targets are allowed. At most 3 redirects. Fixed User-Agent.
  - A failed fetch never blocks the save.
  - When the URL changes, the old extracted title and favicon are replaced, or cleared if the new fetch fails. There's no refresh button.
- `/l/:id` is public, with no login. Every hit counts: no bot filtering, no rate limit. The redirect is `302` with `Cache-Control: no-store` and `Referrer-Policy: no-referrer`. An unknown id returns 404.
- `/l/:id/favicon` is public. It serves the stored copy and never contacts the external site while serving. Headers: `Cache-Control: public, max-age=86400`, a content-hash `ETag`, `304` on `If-None-Match`, `X-Content-Type-Options: nosniff`. It returns 404 when no icon is stored. **No CSP header. This is an accepted risk**: an SVG from the external site, opened directly on the panel origin, could read the non-HttpOnly token cookie.
- User page: rendered directly above `SubscriptionSection`, with no section title. A HeroUI `ListBox` lays the links out flat. Each item shows favicon + title and opens `/l/:id` in a new tab with `rel="noopener noreferrer"`. No hostname, no copy button. Nothing renders when there are no links.
- Admin: a dedicated page `/settings/resource-links`, linked from the settings index. It lists, adds, edits and deletes (with confirmation) and shows visit counts. Drag-to-reorder supports keyboard reordering. After each drop it sends the full id order in one call, which the backend rewrites in one transaction. The backend returns 409 when the id set doesn't match. New links are appended to the end.
- `GET /api/panel/resource-links` accepts any logged-in role and returns only id, displayed title and favicon version. The admin endpoints additionally return URL, manual and extracted titles, `visit_count` and `last_visited_at`.

## Global Constraints

- Store every datetime in UTC (`time.Now().UTC()`).
- Backend changes pass `go build ./...` and `go vet ./...` with zero warnings. After any contract change run `make openapi`, then `pnpm api:sync` in `frontend/`.
- Frontend gate: `pnpm typecheck` and `pnpm i18n:check`. Every visible string comes from Paraglide, with keys in both `messages/en.json` and `messages/zh-cn.json`. HeroUI Alert titles and messages end without `.` or `。`. No em dashes in UI copy.
- Forms use TanStack Form (`useForm`, `form.Field`, `form.Subscribe`). Network calls live in `src/api/queries.ts`, and components never call `apiClient` directly.
- Limits (exact): title ≤ 200 runes, URL ≤ 2048 bytes, page read ≤ 512 KiB, favicon ≤ 100 KiB, ≤ 3 redirects, 10s total fetch, favicon `Cache-Control: public, max-age=86400`.
- Allowed new dependencies: `golang.org/x/net` (already indirect; becomes direct) and `react-aria-components` pinned to `1.18.0`. That's the version HeroUI 3.2.1 ships, and HeroUI does not re-export `useDragAndDrop`. Nothing else.
- Commit style: `feat(backend): …` / `feat(frontend): …` / `docs: …`.

## Review Focus

1. A page served in a non-UTF-8 charset (e.g. GBK with `<meta charset="gbk">`) should still yield a readable title. Covered by `TestFetchDecodesDeclaredCharset` (Task 1).
2. A `favicon.ico` served as `application/octet-stream` (common on nginx defaults) should still be accepted by sniffing. Covered by `TestFetchSniffsUntypedIcon` (Task 1).
3. A page that redirects and then uses a relative icon href should resolve against the final URL, not the original. Covered by `TestFetchResolvesIconAgainstFinalURL` (Task 1).
4. Many simultaneous clicks on one link must not lose counts. Covered by `TestVisitResourceLinkCountsConcurrentHits` (Task 3).
5. A reorder sent after another admin deleted or added a link must be rejected, not silently applied to a partial set. Covered by `TestReorderResourceLinksRejectsMismatchedSet` (Task 2).

---

### Task 1: `linkmeta` page metadata fetcher

**Files:**
- Create: `backend/internal/linkmeta/linkmeta.go`
- Test: `backend/internal/linkmeta/linkmeta_test.go`
- Modify: `backend/go.mod`, `backend/go.sum` (`go mod tidy` promotes `golang.org/x/net`)

**Interfaces:**
- Produces:
  ```go
  type Metadata struct {
      Title    string // trimmed, whitespace-collapsed, ≤ 200 runes; "" when none
      Icon     []byte // nil when none
      IconType string // media type without parameters, e.g. "image/png"; "" when Icon is nil
  }
  func Fetch(ctx context.Context, pageURL string) Metadata
  const MaxTitleRunes = 200
  ```
  `Fetch` never returns an error. Every failure yields zero fields. The caller owns the deadline through `ctx`.

- [ ] **Step 1: Write the failing tests.** Each test uses `httptest.NewServer` and calls `Fetch(context.Background(), srv.URL+path)`.
  - `TestFetchPrefersOGTitle`: page has `<title>Plain</title>` and `<meta property="og:title" content="Rock &amp; Roll">`. Assert `Title == "Rock & Roll"`.
  - `TestFetchFallsBackToTitleTag`: `<title>\n  Hello\n  World </title>`. Assert `"Hello World"`.
  - `TestFetchTruncatesLongTitle`: a 300-rune `<title>`. Assert `utf8.RuneCountInString(Title) == 200`.
  - `TestFetchDecodesDeclaredCharset`: body is `你好` encoded as GBK (`simplifiedchinese.GBK.NewEncoder()`) inside `<meta charset="gbk"><title>…</title>`. Assert `Title == "你好"`.
  - `TestFetchUsesLinkRelIcon`: `<link rel="shortcut icon" href="/i.png">`, and `/i.png` serves PNG bytes with `Content-Type: image/png`. Assert `IconType == "image/png"` and the bytes are equal.
  - `TestFetchResolvesIconAgainstFinalURL`: `/start` 302 → `/dir/page`, which has `<link rel="icon" href="icon.svg">`. `/dir/icon.svg` serves `image/svg+xml`. Assert `IconType == "image/svg+xml"`.
  - `TestFetchFallsBackToFaviconICO`: the page has no `<link>`; `/favicon.ico` serves ICO bytes (`\x00\x00\x01\x00…`) as `image/x-icon`. Assert `Icon != nil`.
  - `TestFetchSniffsUntypedIcon`: the same ICO bytes, but `Content-Type: application/octet-stream`. Assert `IconType == "image/x-icon"`.
  - `TestFetchRejectsNonImageIcon`: `/favicon.ico` returns `text/html`. Assert `Icon == nil && IconType == ""`.
  - `TestFetchRejectsOversizedIcon`: 100 KiB + 1 bytes of PNG. Assert `Icon == nil`.
  - `TestFetchStopsAfterThreeRedirects`: a chain of 4 redirects. Assert `Title == ""`.
  - `TestFetchNonHTMLStillFindsFavicon`: the page is `application/pdf`. Assert `Title == ""` and that `Icon` comes from `/favicon.ico`.
  - `TestFetchHonoursContextDeadline`: the handler sleeps 2s, and ctx has a 100ms timeout. Assert it returns in < 1s with zero `Metadata`.

- [ ] **Step 2: Run** `cd backend && go test ./internal/linkmeta/`. Expected: FAIL (package doesn't compile).

- [ ] **Step 3: Implement `Fetch` in `linkmeta.go`.**
  - Use one package-level `*http.Client` whose `CheckRedirect` errors once `len(via) >= 3`. Send a fixed `User-Agent: Mozilla/5.0 (compatible; HystericalPanel; +resource-links)`.
  - Read the page through `io.LimitReader(body, 512<<10)`. Parse HTML only when the response `Content-Type` is `text/html` or `application/xhtml+xml`. Decode with `charset.NewReader(body, contentType)`, then tokenize with `html.NewTokenizer`. Stop at `</head>` or `<body>`.
  - Icon candidates: the first `<link>` whose space-separated `rel` contains `icon` (case-insensitive), resolved with `resp.Request.URL.ResolveReference`, then `<origin>/favicon.ico`. Try them in order.
  - Read an icon with a limit of `100<<10 + 1`, and reject it when the length is over `100<<10`.
  - Type: take the header media type (via `mime.ParseMediaType`) if it starts with `image/`. Otherwise use `http.DetectContentType` if that starts with `image/`. Otherwise reject.

- [ ] **Step 4: Run** `go test ./internal/linkmeta/ && go vet ./internal/linkmeta/`. Expected: PASS, no vet output.

- [ ] **Step 5: Run** `go mod tidy`, then commit.
  ```bash
  git add backend/internal/linkmeta backend/go.mod backend/go.sum
  git commit -m "feat(backend): fetch page title and favicon for resource links"
  ```

### Task 2: `resource_links` collection and admin/list API

**Files:**
- Create: `backend/migrations/1730000037_create_resource_links.go`
- Create: `backend/internal/api/resource_links.go`
- Test: `backend/internal/api/resource_links_test.go`
- Modify: `backend/internal/api/dto.go` (append the DTOs), `backend/internal/api/api.go` (routes, placed after the `ignored connection IPs` block)

**Interfaces:**
- Consumes: `linkmeta.Fetch`, `linkmeta.Metadata`, `linkmeta.MaxTitleRunes` (Task 1).
- Produces:
  - Collection `resource_links`. Created with `core.NewBaseCollection`, so all API rules stay nil (superuser only). Fields:
    - `url` (Text, Required, Max 2048)
    - `title` (Text, Max 200)
    - `fetched_title` (Text, Max 200)
    - `favicon_data` (Text, Hidden, Max 140000: base64 of ≤ 100 KiB)
    - `favicon_type` (Text, Max 100)
    - `favicon_hash` (Text, Max 16)
    - `sort_order` (Number, OnlyInt)
    - `visit_count` (Number, OnlyInt)
    - `last_visited_at` (Date)
    - `created`/`updated` autodate
    - Index `idx_resource_links_sort_order` on `sort_order`, non-unique.
  - DTOs in `dto.go`:
    ```go
    type ResourceLink struct {           // any logged-in User
        ID             string `json:"id"`
        Title          string `json:"title"`           // displayed title
        FaviconVersion string `json:"favicon_version"` // "" = no stored icon
    }
    type ManagedResourceLink struct {    // admin
        ResourceLink
        URL           string `json:"url"`
        ManualTitle   string `json:"manual_title"`
        FetchedTitle  string `json:"fetched_title"`
        VisitCount    int64  `json:"visit_count"`
        LastVisitedAt string `json:"last_visited_at"` // "" = never
        Created       string `json:"created"`
    }
    type ResourceLinkCreateRequest struct {
        URL   string `json:"url"`
        Title string `json:"title,omitempty"`
    }
    type ResourceLinkUpdateRequest struct {
        URL   *string `json:"url,omitempty"`
        Title *string `json:"title,omitempty"` // "" clears the manual title
    }
    type ResourceLinkOrderRequest struct {
        IDs []string `json:"ids"`
    }
    ```
  - Helpers in `resource_links.go`:
    - `func resourceLinkDisplayTitle(manual, fetched, rawURL string) string`
    - `func normalizeResourceLinkURL(raw string) (string, error)`: trims, requires scheme `http`/`https` and a non-empty host, ≤ 2048 bytes.
    - `func publicResourceLink(rec *core.Record) ResourceLink`
    - `func managedResourceLink(rec *core.Record) ManagedResourceLink`
    - `const resourceLinkFetchTimeout = 10 * time.Second`
  - Handlers, all `func (h *Handlers) x(e *core.RequestEvent) error`:
    - `listResourceLinks`: sorted by `sort_order`, then `created`
    - `listManagedResourceLinks`
    - `createResourceLink`: `sort_order` = current max + 1
    - `updateResourceLink`: refetches only when the normalized URL differs, and on refetch overwrites `fetched_title` + all `favicon_*` (cleared on failure)
    - `deleteResourceLink`: returns `DeleteResponse`
    - `reorderResourceLinks`: one `app.RunInTransaction`; the id set must equal the stored set exactly, with no duplicates, or it returns `apis.NewApiError(409, "resource link order is out of date", nil)`; it sets `sort_order` = index; it returns `[]ManagedResourceLink` in the new order
  - Routes on group `g`:
    ```go
    g.GET("/resource-links", h.listResourceLinks)
    g.GET("/resource-links/manage", h.listManagedResourceLinks).Bind(adminOnly)
    g.POST("/resource-links", h.createResourceLink).Bind(adminOnly)
    g.PUT("/resource-links/order", h.reorderResourceLinks).Bind(adminOnly)
    g.PATCH("/resource-links/{id}", h.updateResourceLink).Bind(adminOnly)
    g.DELETE("/resource-links/{id}", h.deleteResourceLink).Bind(adminOnly)
    ```
  - Error messages, compared case-insensitively via `assertAPIError`: `invalid url` (400), `title is too long` (400), `resource link not found` (404), `resource link order is out of date` (409).

- [ ] **Step 1: Write the failing tests.** Use the existing helpers `newMigratedTestApp`, `newUsersTestRecord` (then set `role=admin` for the admin), `panelEvent`, `decodeBody`, `assertAPIError`, and `h := &Handlers{app: app}`. A local `httptest` page server provides `<title>Docs</title>` and a PNG icon.
  - `TestResourceLinkDisplayTitle`, a table:
    - `("Mine","Fetched","https://a.example/x")` → `"Mine"`
    - `("","Fetched",…)` → `"Fetched"`
    - `("","","https://a.example/x")` → `"a.example"`
  - `TestNormalizeResourceLinkURL`, a table that accepts `" https://a.example/p?q=1 "` → trimmed, and rejects `javascript:alert(1)`, `ftp://a.example`, `https://`, `""`, and a 2049-byte URL.
  - `TestCreateResourceLinkExtractsMetadataAndAppends`: create twice against the page server. Assert the first `FetchedTitle == "Docs"`, `FaviconVersion != ""`, and the second gets a higher `sort_order` (inspect records). Assert the `favicon_data` base64 string doesn't appear in any response body.
  - `TestCreateResourceLinkSavesWhenFetchFails`: URL points at a closed `httptest` server. Assert 200, `Title` == the host, `FaviconVersion == ""`.
  - `TestUpdateResourceLinkRefetchesOnlyOnURLChange`: change only `title` with the page server stopped. Assert `fetched_title` is still `"Docs"`. Then change `url` to the closed server. Assert `FetchedTitle == ""` and `FaviconVersion == ""`.
  - `TestListResourceLinksHidesAdminFields`: a plain User lists. The body must not contain `"url"`, `visit_count` or the target host, and the order follows `sort_order`.
  - `TestReorderResourceLinksRewritesOrder`: three links, send `[c,a,b]`. Assert the list order and that `sort_order` values are `0,1,2`.
  - `TestReorderResourceLinksRejectsMismatchedSet`: with links `a,b,c`, each of `[a,b]`, `[a,b,c,x]` and `[a,a,b]` returns 409, and the stored order is unchanged.
  - `TestResourceLinkRoutesGuards`: through the real router (the `Register` + `BuildMux` pattern from `TestOIDCClientRoutesRejectNonAdmins`). A plain User gets 200 on `GET /api/panel/resource-links` and 403 on the manage, create, order, patch and delete routes. No token → 401.

- [ ] **Step 2: Run** `go test ./internal/api -run 'ResourceLink'`. Expected: FAIL (undefined symbols / missing collection).

- [ ] **Step 3: Write the migration and implement the handlers and routes** to the interfaces above. Store the favicon as `base64.StdEncoding`. `favicon_hash` = the first 16 hex characters of the sha256 of the icon bytes, which is the value returned as `favicon_version`. Run the fetch with `context.WithTimeout(e.Request.Context(), resourceLinkFetchTimeout)` before opening any transaction.

- [ ] **Step 4: Run** `go test ./internal/api -run 'ResourceLink' && go build ./... && go vet ./...`. Expected: PASS, no output from vet.

- [ ] **Step 5: Commit.**
  ```bash
  git add backend/migrations/1730000037_create_resource_links.go backend/internal/api/resource_links.go backend/internal/api/resource_links_test.go backend/internal/api/dto.go backend/internal/api/api.go
  git commit -m "feat(backend): manage and list resource links"
  ```

### Task 3: Public visit and favicon endpoints

**Files:**
- Modify: `backend/internal/api/resource_links.go`, `backend/internal/api/resource_links_test.go`, `backend/internal/api/dto.go`, `backend/internal/api/api.go` (next to the other `se.Router` public routes)

**Interfaces:**
- Consumes: the collection and helpers from Task 2.
- Produces:
  - DTO `type ResourceLinkVisitResponse struct { URL string \`json:"url"\` }`
  - `func (h *Handlers) visitResourceLink(e *core.RequestEvent) error`: one statement, `UPDATE resource_links SET visit_count = visit_count + 1, last_visited_at = {:now} WHERE id = {:id}` via `h.app.DB().NewQuery`. When zero rows are affected it returns 404 `resource link not found`. Otherwise it reads the URL and returns `ResourceLinkVisitResponse`. `now` = UTC in PocketBase's datetime format (`types.NowDateTime().String()`).
  - `func (h *Handlers) resourceLinkFavicon(e *core.RequestEvent) error`: 404 when the id is unknown or there's no `favicon_data`. Otherwise it sets `Content-Type: <favicon_type>`, `Cache-Control: public, max-age=86400`, `ETag: "<favicon_hash>"` and `X-Content-Type-Options: nosniff`. It returns `304` with an empty body when `If-None-Match` equals the ETag, and `200` with the decoded bytes otherwise.
  - Routes:
    ```go
    se.Router.POST("/api/resource-links/{id}/visit", h.visitResourceLink)
    se.Router.GET("/api/resource-links/{id}/favicon", h.resourceLinkFavicon)
    ```

- [ ] **Step 1: Write the failing tests.**
  - `TestVisitResourceLinkCountsAndReturnsTarget`: there's no auth on the event. Assert the body `url` equals the stored URL, `visit_count == 1`, and that `last_visited_at` is set and parses as UTC.
  - `TestVisitResourceLinkCountsConcurrentHits`: 20 goroutines each call the handler. Assert `visit_count == 20`.
  - `TestVisitResourceLinkUnknownID`: assert 404 `resource link not found`.
  - `TestResourceLinkFaviconServesStoredIconWithCacheHeaders`: assert the status, body bytes and all four headers have the exact values above.
  - `TestResourceLinkFaviconNotModified`: `If-None-Match: "<hash>"` gives 304 and an empty body.
  - `TestResourceLinkFaviconMissing`: a link without an icon, and an unknown id, both give 404.
  - Extend `TestResourceLinkRoutesGuards`: both public routes answer without a token (not 401).

- [ ] **Step 2: Run** `go test ./internal/api -run 'ResourceLink'`. Expected: the new tests FAIL.

- [ ] **Step 3: Implement both handlers and register the routes.**

- [ ] **Step 4: Run** `go test ./internal/api -run 'ResourceLink' && go build ./... && go vet ./...`. Expected: PASS.

- [ ] **Step 5: Commit.**
  ```bash
  git add backend/internal/api
  git commit -m "feat(backend): count resource link visits and serve stored favicons"
  ```

### Task 4: OpenAPI contract, typed client functions, backend docs

**Files:**
- Modify: `backend/internal/api/openapi.go`
- Test: `backend/internal/api/openapi_resource_links_test.go`
- Modify: `frontend/src/api/queries.ts`
- Modify: `backend/README.md` (endpoint table), `AGENTS.md`:
  - directory tree: `resource_links.go`, `internal/linkmeta/`
  - data model: a `resource_links` entry
  - the panel-interface and public-interface lists
  - migration range `1730000001..37`
  - responsibility list: one line for Resource Links
  - the accepted SVG/no-CSP risk next to the favicon endpoint description

**Interfaces:**
- Consumes: the DTOs and routes from Tasks 2–3.
- Produces:
  - OpenAPI schemas registered in `schemaDefs`: `ResourceLink`, `ManagedResourceLink`, `ResourceLinkCreateRequest`, `ResourceLinkUpdateRequest`, `ResourceLinkOrderRequest`, `ResourceLinkVisitResponse`.
  - Operation IDs:
    - `listResourceLinks`
    - `listManagedResourceLinks`
    - `createResourceLink`
    - `reorderResourceLinks` (409 documented)
    - `updateResourceLink`
    - `deleteResourceLink`
    - `visitResourceLink` (public)
    - `getResourceLinkFavicon` (public; `200` with content `image/*` schema `{type: string, format: binary}`, plus `304` and `404`)
  - Public operations set `op.Security = &openapi3.SecurityRequirements{}`, as `register` does.
  - In `queries.ts`:
    - types `ResourceLink`, `ManagedResourceLink`, `ResourceLinkCreateRequest`, `ResourceLinkUpdateRequest`
    - `queryKeys.resourceLinks()` and `queryKeys.managedResourceLinks()`
    - `fetchResourceLinks(): Promise<ResourceLink[]>`
    - `fetchManagedResourceLinks(): Promise<ManagedResourceLink[]>`
    - `createResourceLink(body): Promise<ManagedResourceLink>`
    - `updateResourceLink(id, body): Promise<ManagedResourceLink>`
    - `deleteResourceLink(id): Promise<{ deleted: boolean }>`
    - `reorderResourceLinks(ids: string[]): Promise<ManagedResourceLink[]>`
    - `visitResourceLink(id): Promise<string | null>`: resolves to the target URL, or `null` on 404. It's used server-side by Task 5.
    - Each follows the existing `apiRequest(..., m.error_…())` pattern, with the error keys added in Task 6/7.

- [ ] **Step 1: Write the failing test** `TestOpenAPIDescribesResourceLinks`: build the spec. Assert that all eight operation IDs exist under the expected paths and methods, that `visitResourceLink` and `getResourceLinkFavicon` have an empty, non-nil `Security`, and that `listResourceLinks` does not.

- [ ] **Step 2: Run** `go test ./internal/api -run TestOpenAPIDescribesResourceLinks`. Expected: FAIL.

- [ ] **Step 3: Add the schemas and paths to `openapi.go`**, following the `/ignored-connection-ips` blocks.

- [ ] **Step 4: Run** `go test ./internal/api -run TestOpenAPI && go vet ./...`, then `cd ../frontend && pnpm api:sync`. Expected: PASS, and `src/api/schema.d.ts` regenerates with `/api/panel/resource-links` paths.

- [ ] **Step 5: Add the `queries.ts` functions and update the docs listed above.** Then run `pnpm typecheck`. Expected: PASS. This needs the error message keys, so add them to both locales now: `error_resource_links_load`, `error_resource_link_save`, `error_resource_link_save_network`, `error_resource_link_delete`, `error_resource_link_delete_network`, `error_resource_links_reorder`, `error_resource_links_reorder_conflict`. Then run `pnpm i18n:check`.

- [ ] **Step 6: Commit.**
  ```bash
  git add backend/internal/api/openapi.go backend/internal/api/openapi_resource_links_test.go backend/README.md AGENTS.md frontend/src/api/queries.ts frontend/messages
  git commit -m "feat(api): describe resource link endpoints and add typed client calls"
  ```

### Task 5: Frontend `/l/$id` and `/l/$id/favicon` server routes

**Files:**
- Create: `frontend/src/routes/l/$id/index.ts`
- Create: `frontend/src/routes/l/$id/favicon.ts`
- Modify: `frontend/src/routeTree.gen.ts` (regenerated by `pnpm dev`/`pnpm build`; commit it)

**Interfaces:**
- Consumes: `visitResourceLink(id)` (Task 4), `resolveServerApiBaseUrl()` from `src/api/panel-config.ts`.
- Produces: the public URLs `/l/<id>` and `/l/<id>/favicon?v=<favicon_version>` that Tasks 6–7 link to.

- [ ] **Step 1: Implement `/l/$id`** as `createFileRoute("/l/$id/")({ server: { handlers: { GET } } })`.
  - The `GET` handler calls `visitResourceLink(params.id)`.
  - On a URL it returns `new Response(null, { status: 302, headers: { Location: url, "Cache-Control": "no-store", "Referrer-Policy": "no-referrer" } })`.
  - On `null` it returns `new Response("Not found", { status: 404, headers: { "Cache-Control": "no-store" } })`. Server routes bypass the React renderer, so this is a plain 404 rather than the app's 404 page.
  - On any other failure it returns `502`.

- [ ] **Step 2: Implement `/l/$id/favicon`** with a `GET` handler.
  - It does a plain `fetch(`${resolveServerApiBaseUrl()}/api/resource-links/${encodeURIComponent(id)}/favicon`)`, forwarding `If-None-Match`. This is binary, so it doesn't go through the typed client.
  - It returns the upstream status and body, copying only `Content-Type`, `Cache-Control`, `ETag` and `X-Content-Type-Options`.

- [ ] **Step 3: Verify** with `pnpm dev` from the repo root (backend + frontend). Create a link via `curl` against the admin API with an admin token, then check:
  - `curl -si localhost:3000/l/<id>`: expect `302`, `Location: <target>`, `Cache-Control: no-store` and `Referrer-Policy: no-referrer`. The admin list's `visit_count` went up by 1.
  - `curl -si localhost:3000/l/nope`: expect `404`.
  - `curl -si localhost:3000/l/<id>/favicon`: expect `200`, `Cache-Control: public, max-age=86400` and an `ETag`. Repeat with `-H 'If-None-Match: <etag>'` and expect `304`.

- [ ] **Step 4: Run** `pnpm typecheck`. Expected: PASS.

- [ ] **Step 5: Commit.**
  ```bash
  git add frontend/src/routes/l frontend/src/routeTree.gen.ts
  git commit -m "feat(frontend): redirect /l/:id through the panel and proxy favicons"
  ```

### Task 6: Resource Links on the user page

**Files:**
- Create: `frontend/src/components/resource-links.tsx`
- Modify: `frontend/src/routes/users/$userId.tsx` (render directly above `<SubscriptionSection` at ~line 193)
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes: `fetchResourceLinks` and `queryKeys.resourceLinks()` (Task 4), the `/l/…` URLs (Task 5).
- Produces:
  - `export function ResourceLinks(): JSX.Element | null`
  - `export function ResourceLinkFavicon({ id, version }: { id: string; version: string }): JSX.Element`. It's shared with Task 7. It renders `<img src={`/l/${id}/favicon?v=${version}`} alt="" className="size-4">` while `version` is non-empty and the image hasn't fired `onError`, and the `Globe` icon from `@gravity-ui/icons` otherwise.

- [ ] **Step 1: Implement `ResourceLinks`.**
  - It uses `useQuery({ queryKey: queryKeys.resourceLinks(), queryFn: fetchResourceLinks, enabled: canQueryPanelApi() })` and returns `null` while loading, on error, or when the list is empty.
  - Otherwise it renders HeroUI `ListBox` with `aria-label={m.resource_links_list_label()}`, `layout="grid"`, `selectionMode="none"`, and a responsive grid (`grid-cols-1 sm:grid-cols-2 lg:grid-cols-3`) with the page's existing section spacing (`mb-*` matching `Section`). There's no heading.
  - Each `ListBox.Item` has `id={link.id}`, `textValue={link.title}`, `href={`/l/${link.id}`}`, `target="_blank"` and `rel="noopener noreferrer"`. It renders `<ResourceLinkFavicon/>` + a truncated title.
  - `target="_blank"` makes React Aria open the link natively instead of going through the TanStack router. Confirm in the browser that clicking opens a new tab at `/l/<id>` and the current tab doesn't navigate.

- [ ] **Step 2: Add `resource_links_list_label`** to both locales and wire `<ResourceLinks />` into `$userId.tsx` above `SubscriptionSection`, outside the `user &&` guard.

- [ ] **Step 3: Verify:**
  - `pnpm typecheck && pnpm i18n:check` passes.
  - In the browser, as a plain User and as an admin viewing another User:
    - links appear above the subscription area in admin order
    - the favicon or the globe fallback shows
    - Tab/arrow keys reach the items and Enter opens a new tab
    - with zero links nothing renders and there's no layout gap

- [ ] **Step 4: Commit.**
  ```bash
  git add frontend/src/components/resource-links.tsx frontend/src/routes/users/\$userId.tsx frontend/messages
  git commit -m "feat(frontend): show resource links above the subscription area"
  ```

### Task 7: Admin settings page with drag-to-reorder

**Files:**
- Create: `frontend/src/routes/settings/resource-links.tsx`
- Modify: `frontend/src/routes/settings/index.tsx` (a `Link` card to `/settings/resource-links`, in the same style as the `/settings/subscriptions` card)
- Modify: `frontend/package.json`, `pnpm-lock.yaml` (`react-aria-components` exactly `1.18.0`)
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`
- Modify: `AGENTS.md` (the `routes/` list in the frontend directory tree gets `settings/resource-links`; the components list gets `resource-links.tsx`; and the routes list gets `l/$id`)

**Interfaces:**
- Consumes: the managed query functions (Task 4), `ResourceLinkFavicon` (Task 6), `DestructiveConfirmModal`, `PageShell`, `Section`, `ErrorAlert` and `TableSkeleton` from `~/components/ui`, and `requireAdmin` / `markResponsePrivate` as in `settings/connected-apps.tsx`.
- Produces: the route `/settings/resource-links`.

- [ ] **Step 1: Install** with `pnpm --filter frontend add react-aria-components@1.18.0 --save-exact`. Confirm `pnpm why react-aria-components` shows a single version.

- [ ] **Step 2: Implement the page.**
  - Layout: a loader like `connected-apps.tsx`, plus a `PageShell` with breadcrumb `m.resource_links_title()`, a description, and an "Add link" button.
  - The list is a HeroUI `ListBox` with `aria-label`, `selectionMode="none"`, `dragAndDropHooks` from `useDragAndDrop` and `onAction={(id) => openEdit(id)}`.
  - Rows contain no nested buttons. Each row shows the favicon, the displayed title, the hostname (muted), the visit count, and the last visit as relative time (`relTimeFromISO`, or "never").
  - `useDragAndDrop`:
    ```ts
    getItems: (keys) => [...keys].map((key) => ({ "text/plain": String(key) })),
    onReorder: (e) => /* compute new id array by moving e.keys before/after e.target.key */
    ```
  - Reorder flow:
    - Optimistically write the new order into `queryKeys.managedResourceLinks()`, then call `reorderResourceLinks(ids)`.
    - On success, set the cache from the response and invalidate `queryKeys.resourceLinks()`.
    - On a 409, show `m.error_resource_links_reorder_conflict()` and refetch.
    - On any other error, roll back and show `m.error_resource_links_reorder()`.
  - Add and edit share one `Modal` with a TanStack Form:
    - Fields `url` (required) and `title` (optional). The placeholder is the extracted title when editing, otherwise `m.resource_links_title_placeholder()`.
    - The submit button is disabled while pending (`form.Subscribe`), and the label explains the save may take a few seconds while the page is read.
    - Edit mode has a footer delete button that opens `DestructiveConfirmModal`.
    - Every successful mutation invalidates both query keys.
  - Empty state: the `PanelMessage`/teaching pattern used elsewhere, with `m.resource_links_empty()`.

- [ ] **Step 3: Add the copy keys to both locales:**
  - `resource_links_title`, `resource_links_description`, `resource_links_add`, `resource_links_edit`, `resource_links_empty`
  - `resource_links_url_label`, `resource_links_title_label`, `resource_links_title_placeholder`, `resource_links_save`, `resource_links_saving_hint`
  - `resource_links_visits` (`{count}`), `resource_links_last_visited` (`{time}`), `resource_links_never_visited`
  - `resource_links_reorder_hint`, `resource_links_delete`, `resource_links_delete_confirm_title`, `resource_links_delete_confirm_body`
  - `settings_resource_links_manage`

  Alert copy has no trailing punctuation.

- [ ] **Step 4: Verify:**
  - `pnpm typecheck && pnpm i18n:check` passes.
  - In the browser as an admin:
    - add one link with a title and one without; the second shows the extracted page title and favicon
    - drag to reorder and reload; the order persists and the user page matches
    - reorder with the keyboard (focus a row, use React Aria's drag keys) and confirm it saves
    - in a second tab delete a link, then reorder in the first; the conflict message shows and the list refreshes
    - edit and delete through the modal
  - A plain User visiting `/settings/resource-links` is redirected by `requireAdmin`.

- [ ] **Step 5: Commit.**
  ```bash
  git add frontend/src/routes/settings frontend/package.json pnpm-lock.yaml frontend/messages frontend/src/routeTree.gen.ts AGENTS.md
  git commit -m "feat(frontend): manage and reorder resource links in settings"
  ```

### Task 8: Full verification

- [ ] **Step 1: Run** `cd backend && make test && make vet`. Expected: all PASS, no vet output.
- [ ] **Step 2: Run** `cd frontend && pnpm typecheck && pnpm i18n:check && pnpm build`. Expected: all PASS.
- [ ] **Step 3: Run** `git status`. Expected: clean, with no stray `openapi.json` or `schema.d.ts` diffs (both are regenerated or gitignored as the repo expects).
