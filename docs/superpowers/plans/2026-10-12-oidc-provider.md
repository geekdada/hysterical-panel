# OIDC Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admin-registered Connected Apps can sign panel Users in through a narrow OpenID Connect profile: Authorization Code with S256 PKCE, confidential clients, rotating refresh tokens.

**Architecture:** A new package `backend/internal/oidc` owns provider state: collections, signing keys, panel session auth times, token and grant lifecycles, cleanup. `backend/internal/api/oidc_*.go` holds the HTTP handlers: the protocol endpoints under `/api/oidc/*` (outside OpenAPI) and the admin and consent endpoints under `/api/panel/*` (in OpenAPI). The frontend adds the consent page, the Connected Apps admin page and the `/login` re-authentication mode.

**Tech Stack:** Go 1.26, PocketBase v0.39.0 (framework mode, `core`, `dbx`, `tools/types`, `tools/cron`), `github.com/golang-jwt/jwt/v5` v5.3.1 for ID token signing in layer 3. The research suggested go-jose; golang-jwt is used instead because PocketBase already depends on it, and JWKS needs only the RSA modulus and exponent, which the standard library provides, cobra (bundled with PocketBase). Frontend: TanStack Start, HeroUI v3, TanStack Query and Form, Paraglide.

**Spec:** `docs/adr/0010-oidc-provider.md` (decisions) and `docs/research/2026-10-12-oauth-provider.md` (sections 2, 5 and 6 carry the protocol details, collection layout and threat list).

## Delivery layers

Each layer is one local branch stacked on the previous one. Stop after each layer for review before starting the next.

| Branch | Base | Scope | Done when |
|---|---|---|---|
| `oidc/0-docs` | `master` | Research, ADR-0010, this plan | Committed |
| `oidc/1-foundation` | `oidc/0-docs` | Migration for every `oidc_*` collection, session auth time recording, signing keys, cleanup cron, `oidc rotate-signing-key` CLI | Tasks 1 to 6 below pass. No HTTP surface changes |
| `oidc/2-connected-apps` | `oidc/1-foundation` | `app_settings.oidc_enabled`, admin CRUD and secret rotation for Connected Apps under `/api/panel/oidc-clients`, OpenAPI, frontend admin page | Admin can create, edit, disable, delete and rotate a Connected App; secret shown once |
| `oidc/3-protocol` | `oidc/2-connected-apps` | Discovery, JWKS, `/authorize`, approve and deny, `/token`, userinfo, revoke, grant revocation on disable, rate limits | Backend tests for every row of the research threat list; protocol flow works end to end with curl |
| `oidc/4-consent-ui` | `oidc/3-protocol` | Consent and error pages, `/login?reauth=1&redirect=`, remembered consent, authorized apps list in settings | A real RP (next-auth dev app) signs in, re-authenticates on `prompt=login`, and the user can revoke it |
| `oidc/5-finish` | `oidc/4-consent-ui` | Management API lookup by `id`, AGENTS.md, CONTEXT.md, backend README, OpenID conformance Basic OP run | Conformance plan passes or each failure is documented |

The detailed task list for layers 2 to 5 is written when the layer starts, against the code the previous layer actually merged. Only layer 1 is detailed below.

## Global Constraints

- Every datetime is stored in UTC. Filters format times with `types.DefaultDateLayout` (`2006-01-02 15:04:05.000Z`).
- Every `oidc_*` collection keeps all five API rules `nil` (superuser-only).
- Secrets and bearer values are stored as `token.Sha256Hex(value)` in a `Hidden` field and never logged: client secrets, authorization request handles, codes, access tokens, refresh tokens, panel session tokens.
- The signing private key is stored as PKCS#8 PEM encrypted with `cryptobox.Box` in a `Hidden` field.
- Fixed lifetimes (ADR-0010): authorization request 10 minutes, code 60 seconds, access token 10 minutes, ID token 10 minutes, refresh token 30 days idle with a 90-day absolute cap. Retired signing keys stay in JWKS for 24 hours.
- Signing algorithm is RS256 with RSA-2048 keys only.
- Code identifiers use the `oidc` prefix. The glossary term is "Connected App". Do not reuse `oauth2_*` names, which belong to the panel acting as an OAuth client.
- After every task, from `backend/`: `go build ./...` and `go vet ./...` print nothing and `go test ./...` passes.
- Commit messages follow the repo style: lowercase conventional title under 50 characters, body explaining what and why.

## Review Focus

- A token refresh within the same second as the login returns a byte-identical JWT. Recording it again must not fail the sign-in or create a second row. Covered in Task 2 (`TestRecordSessionAuthIsIdempotentForTheSameToken`).
- Impersonation tokens are not refreshable, so PocketBase returns the request token unchanged on refresh. The carried auth time must stay the original one. Covered by the same idempotency test.
- After `PANEL_MASTER_KEY` changes, the stored private key no longer decrypts. The provider must replace it instead of failing every request. Covered in Task 4 (`TestActiveSigningKeyReplacesKeyThatNoLongerDecrypts`).
- Two first requests racing to create the signing key must end with exactly one active key. The partial unique index in Task 1 enforces it (`TestOIDCSigningKeysAllowOneActiveKey`), and `createFirstKey` in Task 4 falls back to the winner's key.
- Running `oidc rotate-signing-key` on a data directory where migrations have not run yet must print an actionable error, not a stack of PocketBase internals. Covered in Task 6.

---

## Layer 1: `oidc/1-foundation`

Start: `git switch -c oidc/1-foundation oidc/0-docs`.

### File structure

| File | Responsibility |
|---|---|
| `backend/migrations/1730000032_create_oidc_provider.go` | Create the seven `oidc_*` collections |
| `backend/migrations/oidc_provider_test.go` | Rules, hidden fields, partial unique indexes, rollback |
| `backend/internal/oidc/oidc.go` | Package doc, collection names, fixed lifetimes |
| `backend/internal/oidc/sessions.go` | `RecordSessionAuth`, `SessionAuthTime` |
| `backend/internal/oidc/keys.go` | `ActiveSigningKey`, `RotateSigningKey`, `PublicJWKS` |
| `backend/internal/oidc/cleanup.go` | `Cleanup`, `ScheduleCleanup` |
| `backend/internal/oidc/command.go` | `oidc rotate-signing-key` cobra command |
| `backend/internal/oidc/*_test.go` | Unit tests |
| `backend/internal/api/oidc_session.go` | Hook that records session auth times on every panel sign-in and refresh |
| `backend/internal/api/oidc_session_test.go` | Login and refresh through the real PocketBase routes |
| `backend/internal/api/api.go` | Call `h.bindOIDCSessionAuthTimes()` in `Register` |
| `backend/main.go` | Register the CLI command and schedule cleanup |

### Task 1: Migration for the `oidc_*` collections

**Files:**
- Create: `backend/migrations/1730000032_create_oidc_provider.go`
- Test: `backend/migrations/oidc_provider_test.go`

**Interfaces:**
- Consumes: `migratedApp(t)` and `migrationRunnerThrough(t, app, file)` from `backend/migrations/user_auth_strings_test.go`.
- Produces: collections `oidc_clients`, `oidc_auth_requests`, `oidc_codes`, `oidc_grants`, `oidc_tokens`, `oidc_signing_keys`, `oidc_session_auth_times` with the fields listed in Step 3. Later layers rely on these exact field names.

- [ ] **Step 1: Write the failing test**

```go
package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

var oidcHiddenFields = map[string][]string{
	"oidc_clients":            {"secret_hash"},
	"oidc_auth_requests":      {"request_hash"},
	"oidc_codes":              {"code_hash"},
	"oidc_grants":             nil,
	"oidc_tokens":             {"token_hash"},
	"oidc_signing_keys":       {"private_key"},
	"oidc_session_auth_times": {"token_hash"},
}

func TestOIDCProviderMigrationCreatesLockedCollections(t *testing.T) {
	app := migratedApp(t)
	for name, hidden := range oidcHiddenFields {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		for rule, value := range map[string]*string{
			"list": c.ListRule, "view": c.ViewRule, "create": c.CreateRule,
			"update": c.UpdateRule, "delete": c.DeleteRule,
		} {
			if value != nil {
				t.Fatalf("%s %s rule = %q, want superuser-only", name, rule, *value)
			}
		}
		for _, field := range hidden {
			f := c.Fields.GetByName(field)
			if f == nil || !f.GetHidden() {
				t.Fatalf("%s.%s must exist and be hidden", name, field)
			}
		}
	}

	runner := migrationRunnerThrough(t, app, "1730000032_create_oidc_provider.go")
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert oidc migration: reverted=%v err=%v", reverted, err)
	}
	for name := range oidcHiddenFields {
		if _, err := app.FindCollectionByNameOrId(name); err == nil {
			t.Fatalf("%s still exists after rollback", name)
		}
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply oidc migration: %v", err)
	}
}

func TestOIDCGrantsAllowOneActiveGrantPerUserAndClient(t *testing.T) {
	app := migratedApp(t)
	user := newOIDCMigrationUser(t, app)
	client := saveOIDCRecord(t, app, "oidc_clients", map[string]any{
		"client_id": "client-a", "name": "App", "secret_hash": "x",
		"redirect_uris": []string{"https://app.example/cb"}, "allowed_scopes": []string{"openid"},
	})
	grant := map[string]any{"user": user.Id, "client": client.Id, "scopes": []string{"openid"}}
	first := saveOIDCRecord(t, app, "oidc_grants", grant)
	if err := saveOIDCRecordErr(app, "oidc_grants", grant); err == nil {
		t.Fatal("second active grant for the same user and client was accepted")
	}
	first.Set("revoked_at", "2026-10-12 00:00:00.000Z")
	if err := app.Save(first); err != nil {
		t.Fatalf("revoke first grant: %v", err)
	}
	saveOIDCRecord(t, app, "oidc_grants", grant)
}

func TestOIDCSigningKeysAllowOneActiveKey(t *testing.T) {
	app := migratedApp(t)
	key := map[string]any{"alg": "RS256", "private_key": "x", "public_jwk": map[string]any{"kty": "RSA"}, "state": "active"}
	key["kid"] = "kid-1"
	saveOIDCRecord(t, app, "oidc_signing_keys", key)
	key["kid"] = "kid-2"
	if err := saveOIDCRecordErr(app, "oidc_signing_keys", key); err == nil {
		t.Fatal("a second active signing key was accepted")
	}
	key["state"] = "retired"
	saveOIDCRecord(t, app, "oidc_signing_keys", key)
}

func newOIDCMigrationUser(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(col)
	user.Set("email", "oidc-migration@example.com")
	user.SetPassword("password12345")
	user.Set("role", "user")
	user.Set("status", "active")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

func saveOIDCRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	rec, err := buildOIDCRecord(app, collection, fields)
	if err == nil {
		err = app.Save(rec)
	}
	if err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return rec
}

func saveOIDCRecordErr(app core.App, collection string, fields map[string]any) error {
	rec, err := buildOIDCRecord(app, collection, fields)
	if err != nil {
		return err
	}
	return app.Save(rec)
}

func buildOIDCRecord(app core.App, collection string, fields map[string]any) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	for key, value := range fields {
		rec.Set(key, value)
	}
	return rec, nil
}
```

`newOIDCMigrationUser` saves a User without a Current Auth String. That is fine here: the hook that adds one is bound by `api.Register`, which a bare migrated app never runs.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./migrations -run 'TestOIDC' -v`
Expected: FAIL with `find oidc_clients: sql: no rows in result set` (or the equivalent missing-collection error).

- [ ] **Step 3: Write the migration**

```go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Storage for the OIDC provider (ADR-0010). Every API rule stays nil, so the
// generic record API serves these collections to superusers only. The panel
// reads and writes them through /api/panel/* and /api/oidc/*.
var oidcCollectionsChildFirst = []string{
	"oidc_session_auth_times", "oidc_signing_keys", "oidc_tokens",
	"oidc_codes", "oidc_auth_requests", "oidc_grants", "oidc_clients",
}

func init() {
	m.Register(createOIDCCollections, func(app core.App) error {
		for _, name := range oidcCollectionsChildFirst {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}

func createOIDCCollections(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	clients := oidcClientsCollection(users)
	if err := app.Save(clients); err != nil {
		return err
	}
	grants := oidcGrantsCollection(users, clients)
	if err := app.Save(grants); err != nil {
		return err
	}
	for _, c := range []*core.Collection{
		oidcAuthRequestsCollection(clients),
		oidcCodesCollection(grants),
		oidcTokensCollection(grants),
		oidcSigningKeysCollection(),
		oidcSessionAuthTimesCollection(users),
	} {
		if err := app.Save(c); err != nil {
			return err
		}
	}
	return nil
}

func addOIDCAutodates(c *core.Collection) {
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
}

func oidcClientsCollection(users *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_clients")
	c.Fields.Add(&core.TextField{Name: "client_id", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 128})
	c.Fields.Add(&core.TextField{Name: "secret_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.JSONField{Name: "redirect_uris", Required: true})
	c.Fields.Add(&core.JSONField{Name: "allowed_scopes", Required: true})
	c.Fields.Add(&core.BoolField{Name: "refresh_enabled"})
	c.Fields.Add(&core.BoolField{Name: "skip_consent"})
	c.Fields.Add(&core.BoolField{Name: "enabled"})
	c.Fields.Add(&core.RelationField{Name: "created_by", MaxSelect: 1, CollectionId: users.Id})
	c.Fields.Add(&core.DateField{Name: "last_used_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_clients_client_id", true, "client_id", "")
	return c
}

// A grant is one User's remembered consent for one Connected App. Only one
// unrevoked grant may exist per pair; scope growth updates it in place.
func oidcGrantsCollection(users, clients *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_grants")
	c.Fields.Add(&core.RelationField{Name: "user", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: users.Id})
	c.Fields.Add(&core.RelationField{Name: "client", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: clients.Id})
	c.Fields.Add(&core.JSONField{Name: "scopes", Required: true})
	c.Fields.Add(&core.DateField{Name: "revoked_at"})
	c.Fields.Add(&core.TextField{Name: "revoke_reason", Max: 64})
	c.Fields.Add(&core.DateField{Name: "last_used_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_grants_active_pair", true, "user, client", "revoked_at = ''")
	return c
}

// max_age 0 means absent: /authorize turns max_age=0 into prompt=login.
func oidcAuthRequestsCollection(clients *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_auth_requests")
	c.Fields.Add(&core.TextField{Name: "request_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.RelationField{Name: "client", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: clients.Id})
	c.Fields.Add(&core.TextField{Name: "redirect_uri", Required: true, Max: 2048})
	c.Fields.Add(&core.TextField{Name: "scope", Required: true, Max: 1024})
	c.Fields.Add(&core.TextField{Name: "state", Max: 1024})
	c.Fields.Add(&core.TextField{Name: "nonce", Max: 1024})
	c.Fields.Add(&core.TextField{Name: "code_challenge", Required: true, Max: 128})
	c.Fields.Add(&core.TextField{Name: "prompt", Max: 64})
	c.Fields.Add(&core.NumberField{Name: "max_age", OnlyInt: true, Min: types.Pointer(0.0)})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	c.Fields.Add(&core.DateField{Name: "consumed_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_auth_requests_hash", true, "request_hash", "")
	c.AddIndex("idx_oidc_auth_requests_expires_at", false, "expires_at", "")
	return c
}

// family links a code to every token issued from it, so a replayed code or
// refresh token revokes exactly that chain (RFC 6749 4.1.2, RFC 9700 4.14.2).
func oidcCodesCollection(grants *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_codes")
	c.Fields.Add(&core.TextField{Name: "code_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.RelationField{Name: "grant", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: grants.Id})
	c.Fields.Add(&core.TextField{Name: "family", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "redirect_uri", Required: true, Max: 2048})
	c.Fields.Add(&core.TextField{Name: "scope", Required: true, Max: 1024})
	c.Fields.Add(&core.TextField{Name: "nonce", Max: 1024})
	c.Fields.Add(&core.TextField{Name: "code_challenge", Required: true, Max: 128})
	c.Fields.Add(&core.DateField{Name: "auth_time"})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	c.Fields.Add(&core.DateField{Name: "used_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_codes_hash", true, "code_hash", "")
	c.AddIndex("idx_oidc_codes_expires_at", false, "expires_at", "")
	return c
}

func oidcTokensCollection(grants *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_tokens")
	c.Fields.Add(&core.TextField{Name: "token_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"access", "refresh"}})
	c.Fields.Add(&core.RelationField{Name: "grant", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: grants.Id})
	c.Fields.Add(&core.TextField{Name: "family", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "scope", Required: true, Max: 1024})
	c.Fields.Add(&core.DateField{Name: "auth_time"})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	c.Fields.Add(&core.DateField{Name: "absolute_expires_at"})
	c.Fields.Add(&core.DateField{Name: "rotated_at"})
	c.Fields.Add(&core.DateField{Name: "revoked_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_tokens_hash", true, "token_hash", "")
	c.AddIndex("idx_oidc_tokens_family", false, "family", "")
	c.AddIndex("idx_oidc_tokens_grant", false, "grant", "")
	c.AddIndex("idx_oidc_tokens_expires_at", false, "expires_at", "")
	return c
}

func oidcSigningKeysCollection() *core.Collection {
	c := core.NewBaseCollection("oidc_signing_keys")
	c.Fields.Add(&core.TextField{Name: "kid", Required: true, Max: 64})
	c.Fields.Add(&core.TextField{Name: "alg", Required: true, Max: 16})
	c.Fields.Add(&core.TextField{Name: "private_key", Required: true, Hidden: true})
	c.Fields.Add(&core.JSONField{Name: "public_jwk", Required: true})
	c.Fields.Add(&core.SelectField{Name: "state", Required: true, MaxSelect: 1, Values: []string{"active", "retired"}})
	c.Fields.Add(&core.DateField{Name: "retired_at"})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_signing_keys_kid", true, "kid", "")
	c.AddIndex("idx_oidc_signing_keys_active", true, "state", "state = 'active'")
	return c
}

// PocketBase session tokens carry no auth time, so the panel records when
// each one came from a real sign-in. prompt=login and max_age read it.
func oidcSessionAuthTimesCollection(users *core.Collection) *core.Collection {
	c := core.NewBaseCollection("oidc_session_auth_times")
	c.Fields.Add(&core.TextField{Name: "token_hash", Required: true, Max: 64, Hidden: true})
	c.Fields.Add(&core.RelationField{Name: "user", Required: true, MaxSelect: 1, CascadeDelete: true, CollectionId: users.Id})
	c.Fields.Add(&core.DateField{Name: "auth_time", Required: true})
	c.Fields.Add(&core.DateField{Name: "expires_at", Required: true})
	addOIDCAutodates(c)
	c.AddIndex("idx_oidc_session_auth_times_hash", true, "token_hash", "")
	c.AddIndex("idx_oidc_session_auth_times_expires_at", false, "expires_at", "")
	return c
}
```

`TextField.Max: 0` on `private_key` falls back to PocketBase's 5000-character default. An encrypted RSA-2048 PKCS#8 PEM is about 2300 base64 characters.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./migrations -run 'TestOIDC' -v`
Expected: PASS for all three tests.

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: no build or vet output, all packages `ok`. Existing migration tests use `migrationRunnerThrough`, so the new migration does not shift their targets.

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/1730000032_create_oidc_provider.go backend/migrations/oidc_provider_test.go
git commit -m "feat(backend): add oidc provider collections"
```

### Task 2: Session auth time storage

**Files:**
- Create: `backend/internal/oidc/oidc.go`
- Create: `backend/internal/oidc/sessions.go`
- Test: `backend/internal/oidc/testutil_test.go`, `backend/internal/oidc/sessions_test.go`

**Interfaces:**
- Consumes: Task 1 collection `oidc_session_auth_times`; `token.Sha256Hex(string) string` from `backend/internal/token`.
- Produces:
  - `oidc.RecordSessionAuth(app core.App, user *core.Record, sessionToken, authMethod, previousToken string, now time.Time) error`
  - `oidc.SessionAuthTime(app core.App, sessionToken string, now time.Time) (authTime time.Time, ok bool, err error)`
  - Constants in `oidc.go` used by every later layer: collection names and lifetimes.

- [ ] **Step 1: Write the test helpers**

`backend/internal/oidc/testutil_test.go`:

```go
package oidc

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	_ "hysterical-panel/migrations"
)

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

func newTestUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("role", "user")
	rec.Set("status", "active")
	rec.Set("verified", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return rec
}

func newTestBox(t *testing.T, masterKey string) *cryptobox.Box {
	t.Helper()
	box, err := cryptobox.New(masterKey)
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	return box
}

func saveTestRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("find %s: %v", collection, err)
	}
	rec := core.NewRecord(col)
	for key, value := range fields {
		rec.Set(key, value)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return rec
}

func countRecords(t *testing.T, app core.App, collection string) int {
	t.Helper()
	recs, err := app.FindAllRecords(collection)
	if err != nil {
		t.Fatalf("list %s: %v", collection, err)
	}
	return len(recs)
}
```

- [ ] **Step 2: Write the failing tests**

`backend/internal/oidc/sessions_test.go`:

```go
package oidc

import (
	"testing"
	"time"
)

var signInTime = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

func TestRecordSessionAuthStoresSignInTime(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "signin@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, ok, err := SessionAuthTime(app, "token-a", signInTime.Add(time.Minute))
	if err != nil || !ok || !got.Equal(signInTime) {
		t.Fatalf("auth time = %v ok=%v err=%v, want %v", got, ok, err, signInTime)
	}
}

func TestRecordSessionAuthCarriesTimeAcrossRefresh(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "refresh@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "passkey", "", signInTime); err != nil {
		t.Fatalf("record sign-in: %v", err)
	}
	later := signInTime.Add(2 * time.Hour)
	if err := RecordSessionAuth(app, user, "token-b", "", "token-a", later); err != nil {
		t.Fatalf("record refresh: %v", err)
	}
	got, ok, err := SessionAuthTime(app, "token-b", later)
	if err != nil || !ok || !got.Equal(signInTime) {
		t.Fatalf("refreshed auth time = %v ok=%v err=%v, want %v", got, ok, err, signInTime)
	}
}

func TestRecordSessionAuthSkipsRefreshOfUnknownToken(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "unknown@example.com")
	if err := RecordSessionAuth(app, user, "token-b", "", "token-never-seen", signInTime); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, ok, err := SessionAuthTime(app, "token-b", signInTime); err != nil || ok {
		t.Fatalf("unknown refresh recorded an auth time: ok=%v err=%v", ok, err)
	}
}

func TestRecordSessionAuthIsIdempotentForTheSameToken(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "same@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime.Add(time.Second)); err != nil {
		t.Fatalf("same token signed in again: %v", err)
	}
	if err := RecordSessionAuth(app, user, "token-a", "", "token-a", signInTime.Add(time.Hour)); err != nil {
		t.Fatalf("non-refreshable token refreshed: %v", err)
	}
	if n := countRecords(t, app, SessionAuthTimesCollection); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	got, _, _ := SessionAuthTime(app, "token-a", signInTime.Add(time.Hour))
	if !got.Equal(signInTime) {
		t.Fatalf("auth time = %v, want the first sign-in %v", got, signInTime)
	}
}

func TestSessionAuthTimeIgnoresExpiredRows(t *testing.T) {
	app := newTestApp(t)
	user := newTestUser(t, app, "expired@example.com")
	if err := RecordSessionAuth(app, user, "token-a", "password", "", signInTime); err != nil {
		t.Fatalf("record: %v", err)
	}
	afterExpiry := signInTime.Add(user.Collection().AuthToken.DurationTime() + time.Second)
	if _, ok, err := SessionAuthTime(app, "token-a", afterExpiry); err != nil || ok {
		t.Fatalf("expired row still answered: ok=%v err=%v", ok, err)
	}
}

func TestSessionAuthTimeOfEmptyTokenIsUnknown(t *testing.T) {
	app := newTestApp(t)
	if _, ok, err := SessionAuthTime(app, "", signInTime); err != nil || ok {
		t.Fatalf("empty token: ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/oidc -v`
Expected: FAIL to compile with `undefined: RecordSessionAuth`.

- [ ] **Step 4: Write `oidc.go`**

```go
// Package oidc holds the storage, signing keys and session bookkeeping of the
// panel's OpenID Connect provider (ADR-0010). HTTP handlers live in
// internal/api.
package oidc

import "time"

const (
	ClientsCollection          = "oidc_clients"
	AuthRequestsCollection     = "oidc_auth_requests"
	CodesCollection            = "oidc_codes"
	GrantsCollection           = "oidc_grants"
	TokensCollection           = "oidc_tokens"
	SigningKeysCollection      = "oidc_signing_keys"
	SessionAuthTimesCollection = "oidc_session_auth_times"
)

// Lifetimes are fixed by ADR-0010.
const (
	AuthRequestTTL      = 10 * time.Minute
	CodeTTL             = 60 * time.Second
	AccessTokenTTL      = 10 * time.Minute
	IDTokenTTL          = 10 * time.Minute
	RefreshTokenIdleTTL = 30 * 24 * time.Hour
	RefreshTokenMaxTTL  = 90 * 24 * time.Hour

	// RetiredKeyRetention keeps a rotated-out key in JWKS long enough for
	// relying parties to verify ID tokens it signed before the rotation.
	RetiredKeyRetention = 24 * time.Hour

	// expiredTokenRetention keeps expired tokens a while longer so a late
	// refresh token replay is still recognised and revokes its family.
	expiredTokenRetention = 7 * 24 * time.Hour
)
```

- [ ] **Step 5: Write `sessions.go`**

```go
package oidc

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/token"
)

// RecordSessionAuth remembers when the panel session behind sessionToken came
// from a real sign-in. PocketBase reports a token refresh with an empty
// authMethod, so a refresh copies the auth time of previousToken instead.
func RecordSessionAuth(app core.App, user *core.Record, sessionToken, authMethod, previousToken string, now time.Time) error {
	// Identical claims within one second produce an identical JWT, and
	// non-refreshable tokens come back unchanged from a refresh.
	if sessionToken == previousToken || sessionRecorded(app, sessionToken) {
		return nil
	}
	authTime := now.UTC()
	if authMethod == "" {
		known, ok, err := SessionAuthTime(app, previousToken, now)
		if err != nil || !ok {
			return err
		}
		authTime = known
	}
	col, err := app.FindCollectionByNameOrId(SessionAuthTimesCollection)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("token_hash", token.Sha256Hex(sessionToken))
	rec.Set("user", user.Id)
	rec.Set("auth_time", authTime)
	rec.Set("expires_at", now.UTC().Add(user.Collection().AuthToken.DurationTime()))
	return app.Save(rec)
}

// SessionAuthTime returns the recorded sign-in time of an unexpired panel
// session token. ok is false for tokens issued before recording started.
func SessionAuthTime(app core.App, sessionToken string, now time.Time) (time.Time, bool, error) {
	if sessionToken == "" {
		return time.Time{}, false, nil
	}
	rec, err := app.FindFirstRecordByFilter(SessionAuthTimesCollection,
		"token_hash = {:hash} && expires_at > {:now}",
		dbx.Params{"hash": token.Sha256Hex(sessionToken), "now": now.UTC().Format(types.DefaultDateLayout)})
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return rec.GetDateTime("auth_time").Time(), true, nil
}

func sessionRecorded(app core.App, sessionToken string) bool {
	_, err := app.FindFirstRecordByFilter(SessionAuthTimesCollection, "token_hash = {:hash}",
		dbx.Params{"hash": token.Sha256Hex(sessionToken)})
	return err == nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/oidc -v`
Expected: PASS for all six tests.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/oidc
git commit -m "feat(backend): record panel session auth times"
```

### Task 3: Record auth times on every panel sign-in

**Files:**
- Create: `backend/internal/api/oidc_session.go`
- Modify: `backend/internal/api/api.go` (in `Register`, after `h.bindUserCreateAuthString()`)
- Test: `backend/internal/api/oidc_session_test.go`

**Interfaces:**
- Consumes: `oidc.RecordSessionAuth`, `oidc.SessionAuthTime` (Task 2); `newMigratedTestApp`, `newUsersTestRecord` (`auth_string_anytls_hash_test.go`), `serveCollectionRequest` (`users_api_rules_test.go`).
- Produces: `(h *Handlers) bindOIDCSessionAuthTimes()` and `bearerToken(r *http.Request) string`. Layer 3 reuses `bearerToken` to read the panel session token on approve.

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hysterical-panel/internal/oidc"
)

func TestSessionAuthTimeFollowsSignInAndRefresh(t *testing.T) {
	app := newMigratedTestApp(t)
	newUsersTestRecord(t, app, "oidc-session@example.com", "SessionSecret123")
	(&Handlers{app: app}).bindOIDCSessionAuthTimes()

	login := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-with-password",
		`{"identity":"oidc-session@example.com","password":"password12345"}`, "")
	loginToken := responseToken(t, login)
	signedIn, ok, err := oidc.SessionAuthTime(app, loginToken, time.Now())
	if err != nil || !ok {
		t.Fatalf("password login was not recorded: ok=%v err=%v", ok, err)
	}

	// A refresh within the same second returns a byte-identical JWT.
	time.Sleep(1100 * time.Millisecond)
	refresh := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-refresh", "", "Bearer "+loginToken)
	refreshed := responseToken(t, refresh)
	if refreshed == loginToken {
		t.Fatal("refresh returned the login token")
	}
	carried, ok, err := oidc.SessionAuthTime(app, refreshed, time.Now())
	if err != nil || !ok || !carried.Equal(signedIn) {
		t.Fatalf("refreshed auth time = %v ok=%v err=%v, want %v", carried, ok, err, signedIn)
	}
}

func TestSessionAuthTimeUnknownForTokensIssuedOutsideSignIn(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "oidc-legacy@example.com", "LegacySecret123")
	(&Handlers{app: app}).bindOIDCSessionAuthTimes()
	legacy, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}
	refresh := serveCollectionRequest(t, app, http.MethodPost, "/api/collections/users/auth-refresh", "", legacy)
	if _, ok, err := oidc.SessionAuthTime(app, responseToken(t, refresh), time.Now()); err != nil || ok {
		t.Fatalf("refresh of an unrecorded token produced an auth time: ok=%v err=%v", ok, err)
	}
}

func TestBearerTokenAcceptsOptionalPrefix(t *testing.T) {
	for header, want := range map[string]string{"Bearer abc": "abc", "bearer abc": "abc", "abc": "abc", "": ""} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", header)
		if got := bearerToken(r); got != want {
			t.Fatalf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}

func responseToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Token == "" {
		t.Fatalf("decode token: %v body=%s", err, rec.Body.String())
	}
	return body.Token
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'TestSessionAuthTime|TestBearerToken' -v`
Expected: FAIL to compile with `h.bindOIDCSessionAuthTimes undefined`.

- [ ] **Step 3: Write `oidc_session.go`**

```go
package api

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

// bindOIDCSessionAuthTimes records when each panel session token came from a
// real sign-in, so the OIDC provider can honour prompt=login and max_age. A
// failed write only means the user re-authenticates later, so sign-in proceeds.
func (h *Handlers) bindOIDCSessionAuthTimes() {
	h.app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		previous := bearerToken(e.Request)
		if err := oidc.RecordSessionAuth(e.App, e.Record, e.Token, e.AuthMethod, previous, time.Now()); err != nil {
			log.Printf("[oidc] record session auth time for user %s: %v", e.Record.Id, err)
		}
		return nil
	})
}

// bearerToken reads the Authorization header the way PocketBase does: the
// Bearer prefix is optional and case-insensitive.
func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if len(value) > 7 && strings.EqualFold(value[:7], "Bearer ") {
		return value[7:]
	}
	return value
}
```

- [ ] **Step 4: Bind it in `Register`**

In `backend/internal/api/api.go`, change:

```go
	h.bindAuthGate()
	h.bindOAuth2LoginOnly()
	h.bindUserCreateAuthString()
```

to:

```go
	h.bindAuthGate()
	h.bindOAuth2LoginOnly()
	h.bindUserCreateAuthString()
	h.bindOIDCSessionAuthTimes()
```

`bindAuthGate` is bound first, so a disabled or unverified user is rejected before this hook runs and no auth time is recorded.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api -run 'TestSessionAuthTime|TestBearerToken|TestGenericUsersRecordAPIIsSuperuserOnly' -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite and commit**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: no output from build and vet, every package `ok`.

```bash
git add backend/internal/api/oidc_session.go backend/internal/api/oidc_session_test.go backend/internal/api/api.go
git commit -m "feat(backend): track sign-in time per panel session"
```

### Task 4: Signing keys and JWKS

**Files:**
- Create: `backend/internal/oidc/keys.go`
- Test: `backend/internal/oidc/keys_test.go`

**Interfaces:**
- Consumes: Task 1 collection `oidc_signing_keys`; `cryptobox.Box.Encrypt/Decrypt`; `token.New(int)`.
- Produces:
  - `type SigningKey struct { ID string; Private *rsa.PrivateKey }`
  - `type JWK struct { KeyType, Use, Algorithm, KeyID, Modulus, Exponent string }` with JSON tags `kty use alg kid n e`
  - `type JWKS struct { Keys []JWK }` with JSON tag `keys`
  - `oidc.ActiveSigningKey(app core.App, box *cryptobox.Box, now time.Time) (*SigningKey, error)`
  - `oidc.RotateSigningKey(app core.App, box *cryptobox.Box, dropPrevious bool, now time.Time) (kid string, err error)`
  - `oidc.PublicJWKS(app core.App, now time.Time) (JWKS, error)`
  - `oidc.SigningAlgorithm = "RS256"` (exported constant for the ID token header and discovery in layer 3)

- [ ] **Step 1: Write the failing tests**

```go
package oidc

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

var keyTime = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

func TestActiveSigningKeyIsCreatedOnceAndReused(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	first, err := ActiveSigningKey(app, box, keyTime)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ActiveSigningKey(app, box, keyTime)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID || first.Private.N.Cmp(second.Private.N) != 0 {
		t.Fatal("second call did not return the stored key")
	}
	if bits := first.Private.N.BitLen(); bits != 2048 {
		t.Fatalf("key size = %d bits, want 2048", bits)
	}
}

func TestRotateSigningKeyKeepsRetiredKeyInJWKSForADay(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	old, _ := ActiveSigningKey(app, box, keyTime)
	newID, err := RotateSigningKey(app, box, false, keyTime)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	active, _ := ActiveSigningKey(app, box, keyTime)
	if active.ID != newID || newID == old.ID {
		t.Fatalf("active = %s, rotated = %s, old = %s", active.ID, newID, old.ID)
	}
	jwks, _ := PublicJWKS(app, keyTime.Add(time.Hour))
	if ids := jwkIDs(jwks); ids != newID+","+old.ID {
		t.Fatalf("jwks kids = %s, want active first then retired", ids)
	}
	jwks, _ = PublicJWKS(app, keyTime.Add(RetiredKeyRetention+time.Minute))
	if ids := jwkIDs(jwks); ids != newID {
		t.Fatalf("jwks kids after retention = %s, want %s", ids, newID)
	}
}

func TestRotateSigningKeyDropPreviousRemovesOlderKeys(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	_, _ = ActiveSigningKey(app, box, keyTime)
	newID, err := RotateSigningKey(app, box, true, keyTime)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	jwks, _ := PublicJWKS(app, keyTime)
	if ids := jwkIDs(jwks); ids != newID {
		t.Fatalf("jwks kids = %s, want only %s", ids, newID)
	}
	if n := countRecords(t, app, SigningKeysCollection); n != 1 {
		t.Fatalf("stored keys = %d, want 1", n)
	}
}

func TestActiveSigningKeyReplacesKeyThatNoLongerDecrypts(t *testing.T) {
	app := newTestApp(t)
	old, err := ActiveSigningKey(app, newTestBox(t, "master-a"), keyTime)
	if err != nil {
		t.Fatalf("create with old master key: %v", err)
	}
	replaced, err := ActiveSigningKey(app, newTestBox(t, "master-b"), keyTime)
	if err != nil {
		t.Fatalf("load with new master key: %v", err)
	}
	if replaced.ID == old.ID {
		t.Fatal("undecryptable key was reused")
	}
	jwks, _ := PublicJWKS(app, keyTime)
	if ids := jwkIDs(jwks); ids != replaced.ID+","+old.ID {
		t.Fatalf("jwks kids = %s, want replacement then retired key", ids)
	}
}

func TestPublicJWKSExposesOnlyPublicParts(t *testing.T) {
	app := newTestApp(t)
	key, _ := ActiveSigningKey(app, newTestBox(t, "master-a"), keyTime)
	jwks, _ := PublicJWKS(app, keyTime)
	raw, _ := json.Marshal(jwks)
	for _, private := range []string{`"d"`, `"p"`, `"q"`, `"dp"`, `"dq"`, `"qi"`} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("jwks contains private member %s: %s", private, raw)
		}
	}
	jwk := jwks.Keys[0]
	if jwk.KeyType != "RSA" || jwk.Use != "sig" || jwk.Algorithm != SigningAlgorithm || jwk.KeyID != key.ID {
		t.Fatalf("unexpected jwk header: %+v", jwk)
	}
	n, err := base64.RawURLEncoding.DecodeString(jwk.Modulus)
	if err != nil || new(big.Int).SetBytes(n).Cmp(key.Private.N) != 0 {
		t.Fatalf("modulus does not match the signing key: %v", err)
	}
	if jwk.Exponent != "AQAB" {
		t.Fatalf("exponent = %s, want AQAB", jwk.Exponent)
	}
}

func jwkIDs(jwks JWKS) string {
	ids := make([]string, 0, len(jwks.Keys))
	for _, k := range jwks.Keys {
		ids = append(ids, k.KeyID)
	}
	return strings.Join(ids, ",")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/oidc -run 'SigningKey|JWKS' -v`
Expected: FAIL to compile with `undefined: ActiveSigningKey`.

- [ ] **Step 3: Write `keys.go`**

```go
package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/token"
)

// SigningAlgorithm is the only ID token algorithm. OIDC Core 15.1 makes
// RS256 mandatory for every OP.
const SigningAlgorithm = "RS256"

const (
	signingKeyBits  = 2048
	keyStateActive  = "active"
	keyStateRetired = "retired"
)

// SigningKey is the active ID token signing key.
type SigningKey struct {
	ID      string
	Private *rsa.PrivateKey
}

// JWK is the public half of a signing key as published in JWKS (RFC 7517).
type JWK struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

// JWKS is the document served at jwks_uri.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// ActiveSigningKey returns the active key and creates one on first use. A key
// that no longer decrypts, because PANEL_MASTER_KEY changed, can never sign
// again, so it is retired and replaced.
func ActiveSigningKey(app core.App, box *cryptobox.Box, now time.Time) (*SigningKey, error) {
	rec, err := app.FindFirstRecordByFilter(SigningKeysCollection, "state = {:state}", dbx.Params{"state": keyStateActive})
	if errors.Is(err, sql.ErrNoRows) {
		return createFirstKey(app, box)
	}
	if err != nil {
		return nil, err
	}
	key, err := decodeSigningKey(rec, box)
	if err == nil {
		return key, nil
	}
	log.Printf("[oidc] signing key %s no longer decrypts, rotating: %v", rec.GetString("kid"), err)
	if _, err := RotateSigningKey(app, box, false, now); err != nil {
		return nil, err
	}
	return ActiveSigningKey(app, box, now)
}

// RotateSigningKey makes a new key active and retires the current one. With
// dropPrevious every older key leaves JWKS at once, which also invalidates the
// ID tokens they signed. Use that after a key compromise.
func RotateSigningKey(app core.App, box *cryptobox.Box, dropPrevious bool, now time.Time) (string, error) {
	key, rec, err := newSigningKeyRecord(app, box)
	if err != nil {
		return "", err
	}
	err = app.RunInTransaction(func(tx core.App) error {
		existing, err := tx.FindAllRecords(SigningKeysCollection)
		if err != nil {
			return err
		}
		for _, old := range existing {
			if err := retireOrDrop(tx, old, dropPrevious, now); err != nil {
				return err
			}
		}
		return tx.Save(rec)
	})
	if err != nil {
		return "", err
	}
	return key.ID, nil
}

// PublicJWKS lists the active key first, then keys retired within
// RetiredKeyRetention.
func PublicJWKS(app core.App, now time.Time) (JWKS, error) {
	cutoff := now.UTC().Add(-RetiredKeyRetention).Format(types.DefaultDateLayout)
	recs, err := app.FindRecordsByFilter(SigningKeysCollection,
		"state = 'active' || retired_at > {:cutoff}", "state,-created", 0, 0, dbx.Params{"cutoff": cutoff})
	if err != nil {
		return JWKS{}, err
	}
	jwks := JWKS{Keys: make([]JWK, 0, len(recs))}
	for _, rec := range recs {
		var jwk JWK
		if err := rec.UnmarshalJSONField("public_jwk", &jwk); err != nil {
			return JWKS{}, fmt.Errorf("decode jwk %s: %w", rec.GetString("kid"), err)
		}
		jwks.Keys = append(jwks.Keys, jwk)
	}
	return jwks, nil
}

func createFirstKey(app core.App, box *cryptobox.Box) (*SigningKey, error) {
	key, rec, err := newSigningKeyRecord(app, box)
	if err != nil {
		return nil, err
	}
	if err := app.Save(rec); err != nil {
		// A concurrent first request may have won the active-key unique index.
		active, findErr := app.FindFirstRecordByFilter(SigningKeysCollection, "state = 'active'")
		if findErr != nil {
			return nil, err
		}
		return decodeSigningKey(active, box)
	}
	return key, nil
}

func retireOrDrop(tx core.App, old *core.Record, dropPrevious bool, now time.Time) error {
	if dropPrevious {
		return tx.Delete(old)
	}
	if old.GetString("state") != keyStateActive {
		return nil
	}
	old.Set("state", keyStateRetired)
	old.Set("retired_at", now.UTC())
	return tx.Save(old)
}

func newSigningKeyRecord(app core.App, box *cryptobox.Box) (*SigningKey, *core.Record, error) {
	col, err := app.FindCollectionByNameOrId(SigningKeysCollection)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is missing; apply migrations first by starting the server once or running `migrate up`: %w", SigningKeysCollection, err)
	}
	private, err := rsa.GenerateKey(rand.Reader, signingKeyBits)
	if err != nil {
		return nil, nil, err
	}
	kid, err := token.New(16)
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := encryptPrivateKey(private, box)
	if err != nil {
		return nil, nil, err
	}
	rec := core.NewRecord(col)
	rec.Set("kid", kid)
	rec.Set("alg", SigningAlgorithm)
	rec.Set("private_key", encrypted)
	rec.Set("public_jwk", publicJWK(kid, &private.PublicKey))
	rec.Set("state", keyStateActive)
	return &SigningKey{ID: kid, Private: private}, rec, nil
}

func encryptPrivateKey(private *rsa.PrivateKey, box *cryptobox.Box) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return "", err
	}
	return box.Encrypt(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
}

func decodeSigningKey(rec *core.Record, box *cryptobox.Box) (*SigningKey, error) {
	plain, err := box.Decrypt(rec.GetString("private_key"))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(plain))
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	private, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return &SigningKey{ID: rec.GetString("kid"), Private: private}, nil
}

func publicJWK(kid string, public *rsa.PublicKey) JWK {
	return JWK{
		KeyType:   "RSA",
		Use:       "sig",
		Algorithm: SigningAlgorithm,
		KeyID:     kid,
		Modulus:   base64.RawURLEncoding.EncodeToString(public.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(public.E)).Bytes()),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/oidc -run 'SigningKey|JWKS' -v`
Expected: PASS for all five tests. RSA generation makes each test take up to a second.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/oidc/keys.go backend/internal/oidc/keys_test.go
git commit -m "feat(backend): add oidc signing keys and jwks"
```

### Task 5: Cleanup of expired provider rows

**Files:**
- Create: `backend/internal/oidc/cleanup.go`
- Modify: `backend/main.go` (inside the `OnServe` handler, after `sendoutService.Start(ctx)`)
- Test: `backend/internal/oidc/cleanup_test.go`

**Interfaces:**
- Consumes: Task 1 collections, Task 2 constants, `saveTestRecord`, `countRecords` helpers.
- Produces: `oidc.Cleanup(app core.App, now time.Time) error`, `oidc.ScheduleCleanup(app core.App)`.

- [ ] **Step 1: Write the failing test**

```go
package oidc

import (
	"testing"
	"time"
)

func TestCleanupRemovesOnlyExpiredRows(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	user := newTestUser(t, app, "cleanup@example.com")
	client := saveTestRecord(t, app, ClientsCollection, map[string]any{
		"client_id": "cleanup", "name": "Cleanup", "secret_hash": "x",
		"redirect_uris": []string{"https://app.example/cb"}, "allowed_scopes": []string{"openid"},
	})
	grant := saveTestRecord(t, app, GrantsCollection, map[string]any{"user": user.Id, "client": client.Id, "scopes": []string{"openid"}})

	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	for i, expires := range []time.Time{past, future} {
		suffix := string(rune('a' + i))
		saveTestRecord(t, app, AuthRequestsCollection, map[string]any{
			"request_hash": "req-" + suffix, "client": client.Id, "redirect_uri": "https://app.example/cb",
			"scope": "openid", "code_challenge": "c", "expires_at": expires,
		})
		saveTestRecord(t, app, CodesCollection, map[string]any{
			"code_hash": "code-" + suffix, "grant": grant.Id, "family": "f", "redirect_uri": "https://app.example/cb",
			"scope": "openid", "code_challenge": "c", "expires_at": expires,
		})
		saveTestRecord(t, app, SessionAuthTimesCollection, map[string]any{
			"token_hash": "session-" + suffix, "user": user.Id, "auth_time": past, "expires_at": expires,
		})
	}
	tokenExpiries := []time.Time{now.Add(-expiredTokenRetention - time.Minute), now.Add(-time.Hour)}
	for i, expires := range tokenExpiries {
		saveTestRecord(t, app, TokensCollection, map[string]any{
			"token_hash": "token-" + string(rune('a'+i)), "kind": "refresh", "grant": grant.Id,
			"family": "f", "scope": "openid", "expires_at": expires,
		})
	}
	for i, retired := range []time.Time{now.Add(-RetiredKeyRetention - time.Minute), now.Add(-time.Hour)} {
		saveTestRecord(t, app, SigningKeysCollection, map[string]any{
			"kid": "kid-" + string(rune('a'+i)), "alg": SigningAlgorithm, "private_key": "x",
			"public_jwk": map[string]any{"kty": "RSA"}, "state": "retired", "retired_at": retired,
		})
	}

	if err := Cleanup(app, now); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, collection := range []string{AuthRequestsCollection, CodesCollection, SessionAuthTimesCollection, TokensCollection, SigningKeysCollection} {
		if n := countRecords(t, app, collection); n != 1 {
			t.Fatalf("%s rows = %d after cleanup, want 1", collection, n)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/oidc -run TestCleanup -v`
Expected: FAIL to compile with `undefined: Cleanup`.

- [ ] **Step 3: Write `cleanup.go`**

```go
package oidc

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const cleanupSchedule = "*/30 * * * *"

type cleanupRule struct {
	collection string
	filter     string
	cutoff     time.Duration
}

// Expired tokens stay for expiredTokenRetention so a late refresh token
// replay still finds its row and revokes the family.
var cleanupRules = []cleanupRule{
	{AuthRequestsCollection, "expires_at < {:cutoff}", 0},
	{CodesCollection, "expires_at < {:cutoff}", 0},
	{SessionAuthTimesCollection, "expires_at < {:cutoff}", 0},
	{TokensCollection, "expires_at < {:cutoff}", expiredTokenRetention},
	{SigningKeysCollection, "state = 'retired' && retired_at < {:cutoff}", RetiredKeyRetention},
}

// Cleanup deletes provider rows that can no longer be used.
func Cleanup(app core.App, now time.Time) error {
	for _, rule := range cleanupRules {
		cutoff := now.UTC().Add(-rule.cutoff).Format(types.DefaultDateLayout)
		recs, err := app.FindRecordsByFilter(rule.collection, rule.filter, "", 0, 0, dbx.Params{"cutoff": cutoff})
		if err != nil {
			return fmt.Errorf("cleanup %s: %w", rule.collection, err)
		}
		for _, rec := range recs {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("cleanup %s: %w", rule.collection, err)
			}
		}
	}
	return nil
}

// ScheduleCleanup runs Cleanup every 30 minutes on the PocketBase cron, which
// the serve command starts.
func ScheduleCleanup(app core.App) {
	app.Cron().MustAdd("oidcCleanup", cleanupSchedule, func() {
		if err := Cleanup(app, time.Now()); err != nil {
			log.Printf("[oidc] cleanup: %v", err)
		}
	})
}
```

- [ ] **Step 4: Schedule it from `main.go`**

In `backend/main.go`, add the import `"hysterical-panel/internal/oidc"` and change:

```go
		monitorService.Start(ctx)
		sendoutService.Start(ctx)
```

to:

```go
		monitorService.Start(ctx)
		sendoutService.Start(ctx)
		oidc.ScheduleCleanup(app)
```

- [ ] **Step 5: Run tests and the full suite**

Run: `go test ./internal/oidc -run TestCleanup -v && go build ./... && go vet ./... && go test ./...`
Expected: PASS, no build or vet output.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/oidc/cleanup.go backend/internal/oidc/cleanup_test.go backend/main.go
git commit -m "feat(backend): clean up expired oidc rows"
```

### Task 6: `oidc rotate-signing-key` command

**Files:**
- Create: `backend/internal/oidc/command.go`
- Modify: `backend/main.go` (after `box, err := cryptobox.New(...)`)
- Test: `backend/internal/oidc/command_test.go`

**Interfaces:**
- Consumes: `RotateSigningKey`, `PublicJWKS` (Task 4).
- Produces: `oidc.NewCommand(app core.App, box *cryptobox.Box) *cobra.Command` with subcommand `rotate-signing-key [--drop-previous]`.

- [ ] **Step 1: Write the failing tests**

```go
package oidc

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestRotateSigningKeyCommandPrintsNewKey(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	cmd := NewCommand(app, box)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"rotate-signing-key"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	jwks, _ := PublicJWKS(app, time.Now())
	if len(jwks.Keys) != 1 || !strings.Contains(out.String(), jwks.Keys[0].KeyID) {
		t.Fatalf("output %q does not name the active key %+v", out.String(), jwks.Keys)
	}
}

func TestRotateSigningKeyCommandExplainsMissingMigrations(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	cmd := NewCommand(app, newTestBox(t, "master-a"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"rotate-signing-key"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("error = %v, want a hint to apply migrations", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/oidc -run TestRotateSigningKeyCommand -v`
Expected: FAIL to compile with `undefined: NewCommand`.

- [ ] **Step 3: Write `command.go`**

```go
package oidc

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"hysterical-panel/internal/cryptobox"
)

// NewCommand returns the `oidc` command group. Signing keys rotate only from
// the command line (ADR-0010).
func NewCommand(app core.App, box *cryptobox.Box) *cobra.Command {
	root := &cobra.Command{Use: "oidc", Short: "Manage the OIDC provider"}
	var dropPrevious bool
	rotate := &cobra.Command{
		Use:          "rotate-signing-key",
		Short:        "Create a new ID token signing key and retire the current one",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kid, err := RotateSigningKey(app, box, dropPrevious, time.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "signing key %s is now active\n", kid)
			return nil
		},
	}
	rotate.Flags().BoolVar(&dropPrevious, "drop-previous", false,
		"remove older keys from JWKS at once; ID tokens they signed stop verifying")
	root.AddCommand(rotate)
	return root
}
```

- [ ] **Step 4: Register it in `main.go`**

In `backend/main.go`, change:

```go
	box, err := cryptobox.New(cfg.PanelMasterKey)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
```

to:

```go
	box, err := cryptobox.New(cfg.PanelMasterKey)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	app.RootCmd.AddCommand(oidc.NewCommand(app, box))
```

- [ ] **Step 5: Run tests and the full suite**

Run: `go test ./internal/oidc -v && go build ./... && go vet ./... && go test ./...`
Expected: PASS, no build or vet output.

- [ ] **Step 6: Smoke-test the binary**

Run from `backend/`, with a throwaway data directory:

```bash
PANEL_MASTER_KEY=dev go run . migrate up --dir /tmp/hp-oidc-smoke
PANEL_MASTER_KEY=dev go run . oidc rotate-signing-key --dir /tmp/hp-oidc-smoke
PANEL_MASTER_KEY=dev go run . oidc rotate-signing-key --drop-previous --dir /tmp/hp-oidc-smoke
rm -rf /tmp/hp-oidc-smoke
```

Expected: each rotate prints `signing key <kid> is now active` with a different kid.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/oidc/command.go backend/internal/oidc/command_test.go backend/main.go
git commit -m "feat(backend): add oidc signing key rotation command"
```

### Layer 1 exit check

- [ ] `go build ./... && go vet ./... && go test ./...` from `backend/` passes.
- [ ] `PANEL_MASTER_KEY=dev make serve` starts, the seven `oidc_*` collections appear in `/_/`, and `GET /api/collections/oidc_clients/records` with a panel user's token returns 403.
- [ ] Signing in to the panel creates one `oidc_session_auth_times` row; reloading the panel after the token refresh creates a second row with the same `auth_time`.
- [ ] `git log --oneline oidc/0-docs..oidc/1-foundation` shows the six commits above. Stop for review.
