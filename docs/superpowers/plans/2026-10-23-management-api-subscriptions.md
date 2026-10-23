# Management API Subscriptions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add read, grant, top-up, terminate, and cancel subscription operations to the Management API without changing panel behavior.

**Architecture:** Reuse the panel subscription handlers behind the existing Bearer-token route group. Add thin Management API wrappers that translate only the three agreed state conflicts from 400 to 409, plus a small visible-Type list handler. Extend the existing Management API OpenAPI builder and its drift tests.

**Tech Stack:** Go, PocketBase v0.39.0, kin-openapi, existing Go handler tests.

**Spec:** `docs/superpowers/specs/2026-10-23-management-api-subscriptions-design.md` and `docs/adr/0013-management-api-user-subscriptions.md`

## Global Constraints

- `/api/mgmt/*` stays out of the panel OpenAPI schema.
- Byte amounts are int64 JSON integers. Datetimes remain UTC.
- Do not add idempotency, token scopes, Subscription Reschedule, Type writes, or combined User creation and granting.
- Panel handlers keep their existing external status codes.
- Run `go build ./...`, `go vet ./...`, and `go test ./...` from `backend` with zero warnings or failures.
- Do not commit unless the user asks.

## Review Focus

1. Hidden Types: list them nowhere in the Type endpoint and reject new grants. Pin in Task 1.
2. Cross-User Subscription IDs: return 404 rather than modifying another User's grant. Pin in Task 1.
3. Unknown outcome retries: repeated Top-up adds again and repeated Grant can queue once. Pin behavior and documentation in Tasks 1 and 2.
4. Current versus queued termination: only current triggers Kick and both preserve history. Pin in Task 1 using the existing kick test server.
5. Panel compatibility: state conflicts remain 400 under `/api/panel`, while Management API returns 409. Pin in Task 1.

---

### Task 1: Management subscription behavior

**Files:**
- Create: `backend/internal/api/management_api_subscriptions.go`
- Create: `backend/internal/api/management_api_subscriptions_test.go`
- Modify: `backend/internal/api/management_api.go` (route table)
- Modify: `backend/internal/api/dto.go` (visible-Type response)
- Modify: `backend/internal/api/subscriptions.go` (shared conflict message constants only)

**Interfaces:**
- Produces `type ManagementSubscriptionType struct { ID, Name string; AllowanceBytes int64; ResetDays int }` with JSON names from the spec.
- Produces handlers `mgmtListSubscriptionTypes`, `mgmtGrantSubscription`, `mgmtTopUpSubscription`, and `mgmtTerminateSubscription`, all taking `*core.RequestEvent` and returning `error`.
- `mgmtListSubscriptionTypes` queries `hidden = false`, sorts by name, and returns only the four DTO fields.
- Grant, Top-up, and DELETE wrappers call the panel handlers. A helper translates only `a future subscription is already queued`, `only a current subscription can be topped up`, and `subscription has already ended` from 400 to `apis.NewApiError(409, message, nil)`. Share message constants with the panel handlers instead of duplicating string literals.
- `managementRoutes()` adds the five operations from the spec. `GET /users/{id}/subscriptions` uses `h.listUserSubscriptions` directly.

- [x] **Step 1: Write failing handler tests**

Write focused tests for visible-Type filtering and field shape; current plus queued grant and third-grant 409; Legacy Unmetered conversion; full history listing; positive, zero, negative, and repeated Top-up; non-current Top-up 409; missing and cross-User 404; current DELETE with Kick and repeated DELETE 409; queued DELETE without Kick; hidden-Type grant 400. Include one assertion that the corresponding panel conflict remains 400.

Use `newMigratedTestApp`, `newUsersTestRecord`, `jsonRequestEvent`, `mgmtStatus`, `startKickTargetNode`, and `expectKick`. Decode successful bodies into DTOs or generic JSON where field absence matters.

- [x] **Step 2: Verify RED**

Run: `cd backend && go test ./internal/api -run TestManagementAPISubscription`
Expected: compile failure because the Management API subscription handlers do not exist.

- [x] **Step 3: Implement the DTO, handlers, wrappers, and routes**

Keep wrappers thin. Do not duplicate grant, usage, or Kick logic.

- [x] **Step 4: Verify GREEN and regression coverage**

Run: `cd backend && go test ./internal/api -run 'TestManagementAPISubscription|TestFirstGrant|TestTopUp|TestConcurrentGrants'`
Expected: PASS.

### Task 2: OpenAPI contract and documentation

**Files:**
- Modify: `backend/internal/api/openapi_management.go`
- Modify: `backend/internal/api/openapi_management_test.go`
- Modify: `backend/README.md`
- Modify: `docs/subscriptions.md`
- Modify: `AGENTS.md`
- Existing decisions: `CONTEXT.md`, `docs/adr/0013-management-api-user-subscriptions.md`

**Interfaces:**
- Reuse `SubscriptionGrantRequest`, `SubscriptionTopUpRequest`, `UserSubscription`, `DeleteResponse`, and `ErrorResponse` schemas in `BuildManagementOpenAPISpec`.
- Add `ManagementSubscriptionType` schema with required `id`, `name`, `allowance_bytes`, and `reset_days`; require `subscription_type` and `allowance_bytes` in their request schemas.
- Add a `Subscriptions` tag and the five operations with operation IDs `mgmtListSubscriptionTypes`, `mgmtListUserSubscriptions`, `mgmtGrantSubscription`, `mgmtTopUpSubscription`, and `mgmtTerminateSubscription`.

- [x] **Step 1: Write failing spec assertions**

Extend `TestManagementSpecMatchesRoutes` coverage through the new route table. Add assertions that every operation documents its exact 400, 404, and 409 responses from the spec, Grant and Top-up descriptions contain the non-idempotency warning, and the Type response schema has only the four public fields. Existing typed-example validation must cover all new request and success examples.

- [x] **Step 2: Verify RED**

Run: `cd backend && go test ./internal/api -run 'TestManagementSpec'`
Expected: FAIL because the five routes have no operations in the spec.

- [x] **Step 3: Implement the spec and update prose docs**

Document the exact lifecycle, error classes, visible-Type filtering, history preservation, current-only Kick, and retry warnings. Update the subscription design's admin-only statement to include these Management API operations. Update the project route inventory without expanding scope.

- [x] **Step 4: Run all verification**

Run: `cd backend && go test ./... && go build ./... && go vet ./...`
Expected: all tests pass; build and vet produce no warnings.

Run: `git diff --check`
Expected: no whitespace errors.
