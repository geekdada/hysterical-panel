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

The detailed task list for each layer is written when the layer starts, against the code the previous layer actually merged. Layers 1 to 3 are detailed below.

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

---

## Layer 2: `oidc/2-connected-apps`

Start: `git switch -c oidc/2-connected-apps oidc/1-foundation`.

### Decisions for this layer

These follow from ADR-0010 and the research report. The plan is the record for them.

- Admins can manage Connected Apps while the provider switch is off, so an app can be registered before the provider is enabled.
- Enabling the provider requires a usable issuer and a consent page. `PANEL_BACKEND_URL_BASE` must be set and use https. Plain http is allowed on loopback hosts. `PANEL_FRONTEND_URL_BASE` must also be set. Enabling creates the signing key at once, so JWKS is never empty while the provider is on.
- `allowed_scopes` is a subset of `openid` and `email`, and `openid` is required. `refresh_enabled` decides whether `offline_access` is honoured, so it is not a scope option.
- Redirect URIs are stored trimmed but otherwise as entered, because `/authorize` compares them by exact string match.
- Validation errors are fixed English strings, so `frontend/src/lib/api-error.ts` can localize them.
- Rotating a client secret does not revoke tokens. A refresh needs client authentication, so holders of the old secret cannot refresh. Access tokens expire within 10 minutes anyway.
- Deleting a Connected App deletes the record. The relation cascade removes its grants, codes, tokens and authorization requests.
- Note for layer 3: `oidc_enabled` can stay true after the environment changes. The protocol endpoints must return 404 unless `oidc_enabled` is true **and** `oidcAvailability()` reports no reason.

### File structure

| File | Responsibility |
|---|---|
| `backend/migrations/1730000033_add_oidc_settings.go` | `app_settings.oidc_enabled` |
| `backend/internal/oidc/issuer.go` | `Issuer`, loopback check |
| `backend/internal/oidc/clients.go` | Redirect URI and scope validation, client credentials |
| `backend/internal/api/oidc_settings.go` | `oidcAvailability`, `applyOIDCEnabled` |
| `backend/internal/api/settings.go` | Read and write the switch, report issuer and availability |
| `backend/internal/api/oidc_clients.go` | Admin CRUD and secret rotation handlers |
| `backend/internal/api/dto.go` | Settings fields and `OIDCClient*` DTOs |
| `backend/internal/api/openapi.go` | Schemas and paths |
| `backend/internal/api/api.go` | Route registration |
| `frontend/src/api/queries.ts` | Types, query key, fetchers |
| `frontend/src/lib/api-error.ts` | Localized backend errors |
| `frontend/messages/en.json`, `frontend/messages/zh-cn.json` | Copy |
| `frontend/src/routes/settings/index.tsx` | Provider switch, issuer, link to the apps page |
| `frontend/src/routes/settings/connected-apps.tsx` | Connected Apps page |

### Task 7: Provider switch in settings

**Files:**
- Create: `backend/migrations/1730000033_add_oidc_settings.go`, `backend/internal/oidc/issuer.go`, `backend/internal/api/oidc_settings.go`
- Modify: `backend/internal/api/settings.go`, `backend/internal/api/dto.go`
- Test: `backend/migrations/oidc_provider_test.go`, `backend/internal/oidc/issuer_test.go`, `backend/internal/api/oidc_settings_test.go`

**Interfaces:**
- Consumes: `oidc.ActiveSigningKey` (Task 4); `Handlers.publicConfig.APIURL` / `.FrontendURL`; `newMigratedTestApp` (`auth_string_anytls_hash_test.go`).
- Produces:
  - `oidc.IssuerPath = "/api/oidc"`, `oidc.Issuer(backendURL string) (string, error)`, `oidc.ErrBackendURLMissing`, `oidc.ErrBackendURLInsecure`, unexported `isLoopbackHost(host string) bool`
  - `(h *Handlers) oidcAvailability() (issuer, reason string)` with reasons `backend_url_missing`, `backend_url_insecure`, `frontend_url_missing`
  - `(h *Handlers) settingsResponse(s settings) SettingsResponse` (was a plain function)
  - `SettingsResponse.OIDCEnabled` (`oidc_enabled`), `.OIDCIssuer` (`oidc_issuer`), `.OIDCUnavailableReason` (`oidc_unavailable_reason`); `SettingsUpdateRequest.OIDCEnabled`
  - `settings.OIDCEnabled`

- [ ] **Step 1: Write the failing tests**

Append to `backend/migrations/oidc_provider_test.go`:

```go
func TestOIDCSettingsMigrationAddsDisabledSwitch(t *testing.T) {
	app := migratedApp(t)
	recs, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("app_settings seed: %v (%d rows)", err, len(recs))
	}
	if recs[0].GetBool("oidc_enabled") {
		t.Fatal("oidc_enabled must default to false")
	}

	runner := migrationRunnerThrough(t, app, "1730000033_add_oidc_settings.go")
	if reverted, err := runner.Down(1); err != nil || len(reverted) != 1 {
		t.Fatalf("revert oidc settings migration: reverted=%v err=%v", reverted, err)
	}
	settings, _ := app.FindCollectionByNameOrId("app_settings")
	if settings.Fields.GetByName("oidc_enabled") != nil {
		t.Fatal("rollback kept oidc_enabled")
	}
	if _, err := runner.Up(); err != nil {
		t.Fatalf("reapply oidc settings migration: %v", err)
	}
}
```

Create `backend/internal/oidc/issuer_test.go`:

```go
package oidc

import (
	"errors"
	"testing"
)

func TestIssuer(t *testing.T) {
	cases := []struct {
		backend, want string
		err           error
	}{
		{"https://panel.example.com", "https://panel.example.com/api/oidc", nil},
		{"https://panel.example.com/", "https://panel.example.com/api/oidc", nil},
		{"http://localhost:8090", "http://localhost:8090/api/oidc", nil},
		{"http://127.0.0.1:8090", "http://127.0.0.1:8090/api/oidc", nil},
		{"http://[::1]:8090", "http://[::1]:8090/api/oidc", nil},
		{"", "", ErrBackendURLMissing},
		{"http://panel.example.com", "", ErrBackendURLInsecure},
		{"http://localhost.example.com", "", ErrBackendURLInsecure},
	}
	for _, c := range cases {
		got, err := Issuer(c.backend)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("Issuer(%q) = %q, %v; want %q, %v", c.backend, got, err, c.want, c.err)
		}
	}
}
```

Create `backend/internal/api/oidc_settings_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/oidc"
)

func newOIDCHandlers(t *testing.T, app core.App, apiURL, frontendURL string) *Handlers {
	t.Helper()
	box, err := cryptobox.New("oidc-test-master-key")
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	return &Handlers{app: app, box: box, publicConfig: PanelConfigResponse{APIURL: apiURL, FrontendURL: frontendURL}}
}

// panelEvent builds a handler event the way the router would, with an
// optional path id and authenticated record.
func panelEvent(t *testing.T, app core.App, method, path, id string, body any, auth *core.Record) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	raw := []byte{}
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if id != "" {
		req.SetPathValue("id", id)
	}
	return &core.RequestEvent{App: app, Auth: auth, Event: router.Event{Request: req, Response: response}}, response
}

// assertAPIError compares the message the way the frontend does: PocketBase
// capitalises it and appends a period.
func assertAPIError(t *testing.T, err error, status int, message string) {
	t.Helper()
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an API error", err)
	}
	got := strings.ToLower(strings.TrimSuffix(apiErr.Message, "."))
	if apiErr.Status != status || got != message {
		t.Fatalf("API error = %d %q, want %d %q", apiErr.Status, got, status, message)
	}
}

func TestEnablingOIDCRequiresIssuerAndConsentOrigins(t *testing.T) {
	cases := map[string]struct{ api, frontend, reason string }{
		"no backend url":     {"", "https://panel.example.com", "backend_url_missing"},
		"plain http backend": {"http://api.example.com", "https://panel.example.com", "backend_url_insecure"},
		"no frontend url":    {"https://api.example.com", "", "frontend_url_missing"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			app := newMigratedTestApp(t)
			h := newOIDCHandlers(t, app, c.api, c.frontend)
			e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
			assertAPIError(t, h.updateSettings(e), http.StatusBadRequest,
				"oidc provider requires an https panel_backend_url_base and panel_frontend_url_base")
			if h.loadSettings().OIDCEnabled {
				t.Fatal("the switch was stored")
			}
			if got := h.settingsResponse(h.loadSettings()).OIDCUnavailableReason; got != c.reason {
				t.Fatalf("reason = %q, want %q", got, c.reason)
			}
		})
	}
}

func TestEnablingOIDCCreatesSigningKeyAndReportsIssuer(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")
	e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": true}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("enable: %v", err)
	}
	var resp SettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OIDCEnabled || resp.OIDCIssuer != "https://api.example.com/api/oidc" || resp.OIDCUnavailableReason != "" {
		t.Fatalf("settings response = %+v", resp)
	}
	if jwks, err := oidc.PublicJWKS(app, time.Now()); err != nil || len(jwks.Keys) != 1 {
		t.Fatalf("jwks after enabling = %+v, err=%v", jwks, err)
	}

	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": false}, nil)
	if err := h.updateSettings(e); err != nil || h.loadSettings().OIDCEnabled {
		t.Fatalf("disable: err=%v enabled=%v", err, h.loadSettings().OIDCEnabled)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./migrations ./internal/oidc ./internal/api -run 'OIDCSettings|TestIssuer|EnablingOIDC'`
Expected: build failure in `internal/oidc` (`undefined: Issuer`) and `internal/api` (`h.settingsResponse undefined`), and `TestOIDCSettingsMigrationAddsDisabledSwitch` fails with `migration 1730000033_add_oidc_settings.go not registered`.

- [ ] **Step 3: Write the migration**

```go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add the OIDC provider switch to app_settings (ADR-0010). It defaults to off,
// so /api/oidc/* stays absent until an admin enables it.
func init() {
	m.Register(func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(&core.BoolField{Name: "oidc_enabled"})
		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("oidc_enabled")
		return app.Save(settings)
	})
}
```

- [ ] **Step 4: Write `issuer.go`**

```go
package oidc

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IssuerPath is appended to PANEL_BACKEND_URL_BASE. The deployed nginx /api/
// rule already routes it to the backend.
const IssuerPath = "/api/oidc"

var (
	ErrBackendURLMissing  = errors.New("PANEL_BACKEND_URL_BASE is not set")
	ErrBackendURLInsecure = errors.New("PANEL_BACKEND_URL_BASE must use https unless it points to a loopback host")
)

// Issuer derives the provider issuer from the public backend origin. OIDC
// requires an https issuer; plain http is accepted on loopback hosts so the
// provider works in local development.
func Issuer(backendURL string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(backendURL), "/")
	if base == "" {
		return "", ErrBackendURLMissing
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: %q is not an absolute URL", ErrBackendURLInsecure, base)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return "", ErrBackendURLInsecure
	}
	return base + IssuerPath, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
```

- [ ] **Step 5: Add the DTO fields**

In `backend/internal/api/dto.go`, inside `SettingsResponse` after `EmailSendoutRatePerMinute`:

```go
	// OIDCEnabled switches the /api/oidc/* provider (ADR-0010).
	OIDCEnabled bool `json:"oidc_enabled"`
	// OIDCIssuer is the issuer Connected Apps configure. It is empty when
	// PANEL_BACKEND_URL_BASE cannot serve as an issuer.
	OIDCIssuer string `json:"oidc_issuer"`
	// OIDCUnavailableReason says why the provider cannot be enabled, or is
	// empty when it can: backend_url_missing, backend_url_insecure or
	// frontend_url_missing.
	OIDCUnavailableReason string `json:"oidc_unavailable_reason"`
```

Inside `SettingsUpdateRequest` after `EmailSendoutRatePerMinute`:

```go
	OIDCEnabled               *bool `json:"oidc_enabled,omitempty"`
```

- [ ] **Step 6: Write `oidc_settings.go`**

```go
package api

import (
	"errors"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

const (
	oidcUnavailableBackendURLMissing  = "backend_url_missing"
	oidcUnavailableBackendURLInsecure = "backend_url_insecure"
	oidcUnavailableFrontendURLMissing = "frontend_url_missing"
)

// oidcAvailability reports the issuer and why the provider cannot be enabled.
// The consent page is served by the frontend, so it needs a frontend origin.
func (h *Handlers) oidcAvailability() (issuer, reason string) {
	issuer, err := oidc.Issuer(h.publicConfig.APIURL)
	switch {
	case errors.Is(err, oidc.ErrBackendURLMissing):
		return "", oidcUnavailableBackendURLMissing
	case err != nil:
		return "", oidcUnavailableBackendURLInsecure
	case h.publicConfig.FrontendURL == "":
		return issuer, oidcUnavailableFrontendURLMissing
	}
	return issuer, ""
}

// applyOIDCEnabled validates and stores the provider switch. Enabling creates
// the signing key at once so JWKS is never empty while the provider is on.
func (h *Handlers) applyOIDCEnabled(rec *core.Record, enabled bool) error {
	if enabled {
		if _, reason := h.oidcAvailability(); reason != "" {
			return apis.NewBadRequestError("oidc provider requires an https PANEL_BACKEND_URL_BASE and PANEL_FRONTEND_URL_BASE", nil)
		}
		if _, err := oidc.ActiveSigningKey(h.app, h.box, time.Now()); err != nil {
			return apis.NewBadRequestError("failed to prepare the oidc signing key", err)
		}
	}
	rec.Set("oidc_enabled", enabled)
	return nil
}
```

- [ ] **Step 7: Wire it into `settings.go`**

1. Add `OIDCEnabled bool` as the last field of `type settings struct`, and `OIDCEnabled: rec.GetBool("oidc_enabled"),` as the last line of `settingsFromRecord`.
2. Replace `func settingsResponse(s settings) SettingsResponse {` with `func (h *Handlers) settingsResponse(s settings) SettingsResponse {`, add `issuer, reason := h.oidcAvailability()` as its first line, and add these three fields to the returned literal:

```go
		OIDCEnabled:               s.OIDCEnabled,
		OIDCIssuer:                issuer,
		OIDCUnavailableReason:     reason,
```

3. Change both callers, in `getSettings` and `updateSettings`, from `settingsResponse(` to `h.settingsResponse(`.
4. In `updateSettings`, insert this block right before the `// Management API: enabling auto-generates` comment:

```go
	if in.OIDCEnabled != nil {
		if err := h.applyOIDCEnabled(rec, *in.OIDCEnabled); err != nil {
			return err
		}
	}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./migrations ./internal/oidc ./internal/api -run 'OIDCSettings|TestIssuer|EnablingOIDC' -v`
Expected: PASS for the migration test, `TestIssuer`, and both `EnablingOIDC` tests (three subtests in the first).

- [ ] **Step 9: Run the full suite and commit**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: no build or vet output, every package `ok`.

```bash
git add backend/migrations/1730000033_add_oidc_settings.go backend/migrations/oidc_provider_test.go backend/internal/oidc/issuer.go backend/internal/oidc/issuer_test.go backend/internal/api/oidc_settings.go backend/internal/api/oidc_settings_test.go backend/internal/api/settings.go backend/internal/api/dto.go
git commit -m "feat(backend): add oidc provider switch"
```

### Task 8: Connected App validation and credentials

**Files:**
- Create: `backend/internal/oidc/clients.go`
- Test: `backend/internal/oidc/clients_test.go`

**Interfaces:**
- Consumes: `isLoopbackHost` (Task 7), `token.New`, `token.Sha256Hex`.
- Produces:
  - `oidc.ScopeOpenID = "openid"`, `oidc.ScopeEmail = "email"`, `oidc.MaxRedirectURIs = 10`
  - `oidc.NormalizeRedirectURIs(raw []string) ([]string, error)`, `oidc.NormalizeScopes(raw []string) ([]string, error)`
  - `oidc.ErrRedirectURICount`, `oidc.ErrRedirectURIInvalid`, `oidc.ErrRedirectURIDuplicate`, `oidc.ErrScopesInvalid` (messages are API error strings)
  - `oidc.NewClientID() (string, error)`, `oidc.NewClientSecret() (secret, secretHash string, err error)`

- [ ] **Step 1: Write the failing tests**

```go
package oidc

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"hysterical-panel/internal/token"
)

func TestNormalizeRedirectURIsAcceptsHTTPSAndLoopbackHTTP(t *testing.T) {
	valid := []string{
		"https://app.example/cb",
		"https://app.example/cb?tenant=1",
		"https://app.example:8443/auth/callback",
		"http://localhost:3000/cb",
		"http://127.0.0.1/cb",
		"http://[::1]:8080/cb",
	}
	got, err := NormalizeRedirectURIs(append([]string{"  https://trim.example/cb  "}, valid...))
	if err != nil {
		t.Fatalf("valid redirect URIs rejected: %v", err)
	}
	if want := append([]string{"https://trim.example/cb"}, valid...); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Exact matching keeps a trailing slash meaningful, so both spellings stay.
	if got, err := NormalizeRedirectURIs([]string{"https://a.example/cb", "https://a.example/cb/"}); err != nil || len(got) != 2 {
		t.Fatalf("distinct spellings: %q, %v", got, err)
	}
}

func TestNormalizeRedirectURIsRejectsUnsafeValues(t *testing.T) {
	for _, bad := range []string{
		"",
		"/cb",
		"app.example/cb",
		"http://app.example/cb",
		"http://localhost.example/cb",
		"https://app.example/cb#fragment",
		"https://user:pass@app.example/cb",
		"ftp://app.example/cb",
		"javascript:alert(1)",
		"https:///cb",
		"https://app.example/" + strings.Repeat("a", 2048),
	} {
		if _, err := NormalizeRedirectURIs([]string{bad}); !errors.Is(err, ErrRedirectURIInvalid) {
			t.Errorf("%q: err = %v, want ErrRedirectURIInvalid", bad, err)
		}
	}
	eleven := make([]string, MaxRedirectURIs+1)
	for i := range eleven {
		eleven[i] = fmt.Sprintf("https://app.example/cb%d", i)
	}
	for _, list := range [][]string{nil, {}, eleven} {
		if _, err := NormalizeRedirectURIs(list); !errors.Is(err, ErrRedirectURICount) {
			t.Errorf("%d entries: err = %v, want ErrRedirectURICount", len(list), err)
		}
	}
	if _, err := NormalizeRedirectURIs([]string{"https://a.example/cb", " https://a.example/cb"}); !errors.Is(err, ErrRedirectURIDuplicate) {
		t.Errorf("duplicate: err = %v, want ErrRedirectURIDuplicate", err)
	}
}

func TestNormalizeScopes(t *testing.T) {
	if got, err := NormalizeScopes([]string{"email", "openid", "email"}); err != nil || !slices.Equal(got, []string{"openid", "email"}) {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := NormalizeScopes([]string{"openid"}); err != nil || !slices.Equal(got, []string{"openid"}) {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range [][]string{nil, {"email"}, {"openid", "profile"}, {"openid", "offline_access"}} {
		if _, err := NormalizeScopes(bad); !errors.Is(err, ErrScopesInvalid) {
			t.Errorf("%q: err = %v, want ErrScopesInvalid", bad, err)
		}
	}
}

func TestNewClientCredentials(t *testing.T) {
	id, err := NewClientID()
	if err != nil || len(id) < 20 {
		t.Fatalf("client id %q, %v", id, err)
	}
	secret, hash, err := NewClientSecret()
	if err != nil || len(secret) < 43 || hash != token.Sha256Hex(secret) {
		t.Fatalf("secret %q hash %q err %v", secret, hash, err)
	}
	other, _, _ := NewClientSecret()
	if other == secret {
		t.Fatal("two secrets are equal")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/oidc -run 'NormalizeRedirect|NormalizeScopes|NewClientCredentials'`
Expected: build failure with `undefined: NormalizeRedirectURIs`.

- [ ] **Step 3: Write `clients.go`**

```go
package oidc

import (
	"errors"
	"net/url"
	"strings"

	"hysterical-panel/internal/token"
)

const (
	ScopeOpenID = "openid"
	ScopeEmail  = "email"

	// MaxRedirectURIs bounds the admin form and the exact-match lookup.
	MaxRedirectURIs      = 10
	maxRedirectURILength = 2048
)

// The messages are API error strings that the frontend localizes.
var (
	ErrRedirectURICount     = errors.New("redirect_uris must contain between 1 and 10 entries")
	ErrRedirectURIInvalid   = errors.New("redirect URI must be an absolute https URL, or http on localhost, without a fragment or credentials")
	ErrRedirectURIDuplicate = errors.New("redirect URIs must be unique")
	ErrScopesInvalid        = errors.New("allowed_scopes must include openid and only contain openid or email")
)

// NormalizeRedirectURIs trims and validates each entry. /authorize compares
// redirect_uri by exact string match (RFC 9700 2.1), so values are otherwise
// stored as entered.
func NormalizeRedirectURIs(raw []string) ([]string, error) {
	if len(raw) == 0 || len(raw) > MaxRedirectURIs {
		return nil, ErrRedirectURICount
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, entry := range raw {
		uri := strings.TrimSpace(entry)
		if !validRedirectURI(uri) {
			return nil, ErrRedirectURIInvalid
		}
		if seen[uri] {
			return nil, ErrRedirectURIDuplicate
		}
		seen[uri] = true
		out = append(out, uri)
	}
	return out, nil
}

// RFC 6749 3.1.2 forbids fragments. Plain http is allowed only on loopback
// hosts, where the code never crosses the network.
func validRedirectURI(uri string) bool {
	if uri == "" || len(uri) > maxRedirectURILength || strings.Contains(uri, "#") {
		return false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return isLoopbackHost(u.Hostname())
	}
	return false
}

// NormalizeScopes returns the allowed scopes in canonical order. Refresh
// tokens depend on the client's refresh_enabled flag, not on a scope here.
func NormalizeScopes(raw []string) ([]string, error) {
	has := map[string]bool{}
	for _, scope := range raw {
		scope = strings.TrimSpace(scope)
		if scope != ScopeOpenID && scope != ScopeEmail {
			return nil, ErrScopesInvalid
		}
		has[scope] = true
	}
	if !has[ScopeOpenID] {
		return nil, ErrScopesInvalid
	}
	out := []string{ScopeOpenID}
	if has[ScopeEmail] {
		out = append(out, ScopeEmail)
	}
	return out, nil
}

// NewClientID returns a random public client identifier.
func NewClientID() (string, error) {
	return token.New(16)
}

// NewClientSecret returns a 256-bit secret, shown to the admin once, and its
// SHA-256 hash, which is all the panel stores.
func NewClientSecret() (secret, secretHash string, err error) {
	secret, err = token.New(32)
	if err != nil {
		return "", "", err
	}
	return secret, token.Sha256Hex(secret), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/oidc -run 'NormalizeRedirect|NormalizeScopes|NewClientCredentials' -v`
Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/oidc/clients.go backend/internal/oidc/clients_test.go
git commit -m "feat(backend): validate connected app settings"
```

### Task 9: Connected App admin endpoints

**Files:**
- Create: `backend/internal/api/oidc_clients.go`
- Modify: `backend/internal/api/dto.go`, `backend/internal/api/api.go`
- Test: `backend/internal/api/oidc_clients_test.go`

**Interfaces:**
- Consumes: Task 8 functions and errors; `panelEvent`, `assertAPIError`, `newOIDCHandlers` (Task 7 test file); `newUsersTestRecord`.
- Produces:
  - DTOs `OIDCClient`, `OIDCClientCreateRequest`, `OIDCClientUpdateRequest`, `OIDCClientSecretResponse`
  - Routes, all `requireAdmin`: `GET|POST /api/panel/oidc-clients`, `PATCH|DELETE /api/panel/oidc-clients/{id}`, `POST /api/panel/oidc-clients/{id}/rotate-secret`
  - API error strings: `connected app name must be between 1 and 128 characters`, `connected app name cannot contain control characters`, `connected app not found`, plus the four Task 8 errors

- [ ] **Step 1: Add the DTOs**

Append to `backend/internal/api/dto.go`:

```go
// ── OIDC Connected Apps ──────────────────────────────────────────────────────

// OIDCClient is a Connected App as admins see it. The client secret and its
// hash are never returned; only create and rotate responses carry the
// plaintext secret, once.
type OIDCClient struct {
	ID             string   `json:"id"`
	ClientID       string   `json:"client_id"`
	Name           string   `json:"name"`
	RedirectURIs   []string `json:"redirect_uris"`
	AllowedScopes  []string `json:"allowed_scopes"`
	RefreshEnabled bool     `json:"refresh_enabled"`
	SkipConsent    bool     `json:"skip_consent"`
	Enabled        bool     `json:"enabled"`
	LastUsedAt     *string  `json:"last_used_at,omitempty"`
	Created        string   `json:"created"`
	Updated        string   `json:"updated"`
}

// OIDCClientCreateRequest registers a Connected App. allowed_scopes defaults
// to openid and email, and enabled defaults to true.
type OIDCClientCreateRequest struct {
	Name           string   `json:"name"`
	RedirectURIs   []string `json:"redirect_uris"`
	AllowedScopes  []string `json:"allowed_scopes,omitempty"`
	RefreshEnabled *bool    `json:"refresh_enabled,omitempty"`
	SkipConsent    *bool    `json:"skip_consent,omitempty"`
	Enabled        *bool    `json:"enabled,omitempty"`
}

// OIDCClientUpdateRequest changes a Connected App. Omitted fields stay.
type OIDCClientUpdateRequest struct {
	Name           *string   `json:"name,omitempty"`
	RedirectURIs   *[]string `json:"redirect_uris,omitempty"`
	AllowedScopes  *[]string `json:"allowed_scopes,omitempty"`
	RefreshEnabled *bool     `json:"refresh_enabled,omitempty"`
	SkipConsent    *bool     `json:"skip_consent,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
}

// OIDCClientSecretResponse is returned on create and on secret rotation, the
// only places the plaintext client secret appears.
type OIDCClientSecretResponse struct {
	Client       OIDCClient `json:"client"`
	ClientSecret string     `json:"client_secret"`
}
```

- [ ] **Step 2: Write the failing tests**

`backend/internal/api/oidc_clients_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
	"hysterical-panel/internal/token"
)

const helpCenterCallback = "https://help.example/api/auth/callback/panel"

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func createTestOIDCClient(t *testing.T, h *Handlers, admin *core.Record, body map[string]any) OIDCClientSecretResponse {
	t.Helper()
	e, rec := panelEvent(t, h.app, http.MethodPost, "/api/panel/oidc-clients", "", body, admin)
	if err := h.createOIDCClient(e); err != nil {
		t.Fatalf("create: %v", err)
	}
	return decodeBody[OIDCClientSecretResponse](t, rec)
}

func TestCreateOIDCClientReturnsSecretOnceAndStoresOnlyHash(t *testing.T) {
	app := newMigratedTestApp(t)
	admin := newUsersTestRecord(t, app, "oidc-admin@example.com", "AdminSecret123")
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")

	created := createTestOIDCClient(t, h, admin, map[string]any{
		"name": "  Help center  ", "redirect_uris": []string{helpCenterCallback},
	})
	client := created.Client
	if created.ClientSecret == "" || client.ClientID == "" || client.Name != "Help center" {
		t.Fatalf("create response = %+v", created)
	}
	if !slices.Equal(client.AllowedScopes, []string{"openid", "email"}) || !client.Enabled || client.RefreshEnabled || client.SkipConsent {
		t.Fatalf("defaults = %+v", client)
	}
	stored, err := app.FindRecordById(oidc.ClientsCollection, client.ID)
	if err != nil {
		t.Fatalf("find stored client: %v", err)
	}
	if stored.GetString("secret_hash") != token.Sha256Hex(created.ClientSecret) || stored.GetString("created_by") != admin.Id {
		t.Fatalf("stored hash or creator wrong: %v", stored.PublicExport())
	}

	e, rec := panelEvent(t, app, http.MethodGet, "/api/panel/oidc-clients", "", nil, admin)
	if err := h.listOIDCClients(e); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, secret := range []string{created.ClientSecret, stored.GetString("secret_hash")} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("list leaked %q: %s", secret, rec.Body.String())
		}
	}
	if list := decodeBody[[]OIDCClient](t, rec); len(list) != 1 || list[0].ClientID != client.ClientID {
		t.Fatalf("list = %+v", list)
	}
}

func TestCreateOIDCClientRejectsInvalidInput(t *testing.T) {
	cases := map[string]struct {
		body    map[string]any
		message string
	}{
		"empty name":         {map[string]any{"name": " ", "redirect_uris": []string{helpCenterCallback}}, "connected app name must be between 1 and 128 characters"},
		"control character":  {map[string]any{"name": "Help\ncenter", "redirect_uris": []string{helpCenterCallback}}, "connected app name cannot contain control characters"},
		"no redirect":        {map[string]any{"name": "Help", "redirect_uris": []string{}}, "redirect_uris must contain between 1 and 10 entries"},
		"plain http":         {map[string]any{"name": "Help", "redirect_uris": []string{"http://help.example/cb"}}, "redirect uri must be an absolute https url, or http on localhost, without a fragment or credentials"},
		"duplicate redirect": {map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback, helpCenterCallback}}, "redirect uris must be unique"},
		"missing openid":     {map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}, "allowed_scopes": []string{"email"}}, "allowed_scopes must include openid and only contain openid or email"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			app := newMigratedTestApp(t)
			h := newOIDCHandlers(t, app, "", "")
			e, _ := panelEvent(t, app, http.MethodPost, "/api/panel/oidc-clients", "", c.body, nil)
			assertAPIError(t, h.createOIDCClient(e), http.StatusBadRequest, c.message)
			if recs, _ := app.FindAllRecords(oidc.ClientsCollection); len(recs) != 0 {
				t.Fatalf("stored %d clients", len(recs))
			}
		})
	}
}

func TestUpdateOIDCClientChangesOnlyGivenFields(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "", "")
	created := createTestOIDCClient(t, h, nil, map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}})
	id := created.Client.ID

	uris := []string{helpCenterCallback, "http://localhost:3000/api/auth/callback/panel"}
	e, rec := panelEvent(t, app, http.MethodPatch, "/api/panel/oidc-clients/"+id, id,
		map[string]any{"enabled": false, "redirect_uris": uris, "refresh_enabled": true}, nil)
	if err := h.updateOIDCClient(e); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated := decodeBody[OIDCClient](t, rec)
	if updated.Name != "Help" || updated.Enabled || !updated.RefreshEnabled || !slices.Equal(updated.RedirectURIs, uris) || updated.ClientID != created.Client.ClientID {
		t.Fatalf("updated = %+v", updated)
	}

	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/oidc-clients/"+id, id,
		map[string]any{"name": "Renamed", "redirect_uris": []string{"http://help.example/cb"}}, nil)
	assertAPIError(t, h.updateOIDCClient(e), http.StatusBadRequest,
		"redirect uri must be an absolute https url, or http on localhost, without a fragment or credentials")
	stored, _ := app.FindRecordById(oidc.ClientsCollection, id)
	if stored.GetString("name") != "Help" {
		t.Fatal("a rejected update changed the stored name")
	}

	e, _ = panelEvent(t, app, http.MethodPatch, "/api/panel/oidc-clients/missing", "missing", map[string]any{"enabled": true}, nil)
	assertAPIError(t, h.updateOIDCClient(e), http.StatusNotFound, "connected app not found")
}

func TestRotateOIDCClientSecretReplacesHash(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "", "")
	created := createTestOIDCClient(t, h, nil, map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}})
	id := created.Client.ID

	e, rec := panelEvent(t, app, http.MethodPost, "/api/panel/oidc-clients/"+id+"/rotate-secret", id, nil, nil)
	if err := h.rotateOIDCClientSecret(e); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	rotated := decodeBody[OIDCClientSecretResponse](t, rec)
	stored, _ := app.FindRecordById(oidc.ClientsCollection, id)
	if rotated.ClientSecret == created.ClientSecret || stored.GetString("secret_hash") != token.Sha256Hex(rotated.ClientSecret) {
		t.Fatal("rotation did not replace the stored secret")
	}
	if rotated.Client.ClientID != created.Client.ClientID {
		t.Fatal("rotation changed the client id")
	}
}

func TestDeleteOIDCClientRemovesItsGrants(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "oidc-grant@example.com", "GrantSecret123")
	h := newOIDCHandlers(t, app, "", "")
	created := createTestOIDCClient(t, h, nil, map[string]any{"name": "Help", "redirect_uris": []string{helpCenterCallback}})
	grants, _ := app.FindCollectionByNameOrId(oidc.GrantsCollection)
	grant := core.NewRecord(grants)
	grant.Set("user", user.Id)
	grant.Set("client", created.Client.ID)
	grant.Set("scopes", []string{"openid"})
	if err := app.Save(grant); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	id := created.Client.ID
	e, _ := panelEvent(t, app, http.MethodDelete, "/api/panel/oidc-clients/"+id, id, nil, nil)
	if err := h.deleteOIDCClient(e); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := app.FindRecordById(oidc.GrantsCollection, grant.Id); err == nil {
		t.Fatal("grant survived its client")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/api -run OIDCClient`
Expected: build failure with `h.createOIDCClient undefined`.

- [ ] **Step 4: Write `oidc_clients.go`**

```go
package api

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

func publicOIDCClient(rec *core.Record) OIDCClient {
	var lastUsedAt *string
	if value := rec.GetString("last_used_at"); value != "" {
		lastUsedAt = &value
	}
	return OIDCClient{
		ID:             rec.Id,
		ClientID:       rec.GetString("client_id"),
		Name:           rec.GetString("name"),
		RedirectURIs:   jsonStringList(rec, "redirect_uris"),
		AllowedScopes:  jsonStringList(rec, "allowed_scopes"),
		RefreshEnabled: rec.GetBool("refresh_enabled"),
		SkipConsent:    rec.GetBool("skip_consent"),
		Enabled:        rec.GetBool("enabled"),
		LastUsedAt:     lastUsedAt,
		Created:        rec.GetString("created"),
		Updated:        rec.GetString("updated"),
	}
}

// jsonStringList reads a JSON string array field. A malformed value reads as
// empty instead of failing the whole list.
func jsonStringList(rec *core.Record, field string) []string {
	var out []string
	if err := rec.UnmarshalJSONField(field, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func (h *Handlers) findOIDCClient(id string) (*core.Record, error) {
	rec, err := h.app.FindRecordById(oidc.ClientsCollection, id)
	if err != nil {
		return nil, apis.NewNotFoundError("connected app not found", err)
	}
	return rec, nil
}

func (h *Handlers) listOIDCClients(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter(oidc.ClientsCollection, "", "name", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list connected apps", err)
	}
	out := make([]OIDCClient, 0, len(records))
	for _, rec := range records {
		out = append(out, publicOIDCClient(rec))
	}
	return ok(e, out)
}

func (h *Handlers) createOIDCClient(e *core.RequestEvent) error {
	var in OIDCClientCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	coll, err := h.app.FindCollectionByNameOrId(oidc.ClientsCollection)
	if err != nil {
		return apis.NewBadRequestError("failed to load connected apps", err)
	}
	rec := core.NewRecord(coll)
	if err := applyOIDCClientUpdate(rec, createAsUpdate(in)); err != nil {
		return err
	}
	clientID, err := oidc.NewClientID()
	if err != nil {
		return apis.NewBadRequestError("failed to generate client credentials", err)
	}
	secret, secretHash, err := oidc.NewClientSecret()
	if err != nil {
		return apis.NewBadRequestError("failed to generate client credentials", err)
	}
	rec.Set("client_id", clientID)
	rec.Set("secret_hash", secretHash)
	if e.Auth != nil {
		rec.Set("created_by", e.Auth.Id)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save connected app", err)
	}
	return ok(e, OIDCClientSecretResponse{Client: publicOIDCClient(rec), ClientSecret: secret})
}

// createAsUpdate applies the create defaults: openid and email scopes, enabled.
func createAsUpdate(in OIDCClientCreateRequest) OIDCClientUpdateRequest {
	scopes := in.AllowedScopes
	if scopes == nil {
		scopes = []string{oidc.ScopeOpenID, oidc.ScopeEmail}
	}
	enabled := in.Enabled == nil || *in.Enabled
	return OIDCClientUpdateRequest{
		Name:           &in.Name,
		RedirectURIs:   &in.RedirectURIs,
		AllowedScopes:  &scopes,
		RefreshEnabled: in.RefreshEnabled,
		SkipConsent:    in.SkipConsent,
		Enabled:        &enabled,
	}
}

func (h *Handlers) updateOIDCClient(e *core.RequestEvent) error {
	rec, err := h.findOIDCClient(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in OIDCClientUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if err := applyOIDCClientUpdate(rec, in); err != nil {
		return err
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save connected app", err)
	}
	return ok(e, publicOIDCClient(rec))
}

func (h *Handlers) deleteOIDCClient(e *core.RequestEvent) error {
	rec, err := h.findOIDCClient(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := h.app.Delete(rec); err != nil {
		return apis.NewBadRequestError("failed to delete connected app", err)
	}
	return ok(e, DeleteResponse{Deleted: true})
}

// rotateOIDCClientSecret replaces the secret. Existing tokens stay valid, but
// a refresh needs client authentication, so only the new secret can refresh.
func (h *Handlers) rotateOIDCClientSecret(e *core.RequestEvent) error {
	rec, err := h.findOIDCClient(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	secret, secretHash, err := oidc.NewClientSecret()
	if err != nil {
		return apis.NewBadRequestError("failed to generate client credentials", err)
	}
	rec.Set("secret_hash", secretHash)
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save connected app", err)
	}
	return ok(e, OIDCClientSecretResponse{Client: publicOIDCClient(rec), ClientSecret: secret})
}

// applyOIDCClientUpdate validates every given field before the caller saves,
// so a rejected request changes nothing.
func applyOIDCClientUpdate(rec *core.Record, in OIDCClientUpdateRequest) error {
	if in.Name != nil {
		name, err := normalizeOIDCClientName(*in.Name)
		if err != nil {
			return err
		}
		rec.Set("name", name)
	}
	if in.RedirectURIs != nil {
		uris, err := oidc.NormalizeRedirectURIs(*in.RedirectURIs)
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		rec.Set("redirect_uris", uris)
	}
	if in.AllowedScopes != nil {
		scopes, err := oidc.NormalizeScopes(*in.AllowedScopes)
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		rec.Set("allowed_scopes", scopes)
	}
	setOptionalBool(rec, "refresh_enabled", in.RefreshEnabled)
	setOptionalBool(rec, "skip_consent", in.SkipConsent)
	setOptionalBool(rec, "enabled", in.Enabled)
	return nil
}

func setOptionalBool(rec *core.Record, field string, value *bool) {
	if value != nil {
		rec.Set(field, *value)
	}
}

func normalizeOIDCClientName(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return "", apis.NewBadRequestError("connected app name must be between 1 and 128 characters", nil)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", apis.NewBadRequestError("connected app name cannot contain control characters", nil)
		}
	}
	return name, nil
}
```

- [ ] **Step 5: Register the routes**

In `backend/internal/api/api.go`, after the `g.POST("/notification-channels/{id}/reveal/finish", ...)` line, add:

```go
	g.GET("/oidc-clients", h.listOIDCClients).Bind(adminOnly)
	g.POST("/oidc-clients", h.createOIDCClient).Bind(adminOnly)
	g.PATCH("/oidc-clients/{id}", h.updateOIDCClient).Bind(adminOnly)
	g.DELETE("/oidc-clients/{id}", h.deleteOIDCClient).Bind(adminOnly)
	g.POST("/oidc-clients/{id}/rotate-secret", h.rotateOIDCClientSecret).Bind(adminOnly)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/api -run OIDCClient -v`
Expected: PASS for all five tests, including the six subtests of `TestCreateOIDCClientRejectsInvalidInput`.

- [ ] **Step 7: Run the full suite and commit**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: no build or vet output, every package `ok`.

```bash
git add backend/internal/api/oidc_clients.go backend/internal/api/oidc_clients_test.go backend/internal/api/dto.go backend/internal/api/api.go
git commit -m "feat(backend): add connected app admin endpoints"
```

### Task 10: OpenAPI contract

**Files:**
- Modify: `backend/internal/api/openapi.go`
- Test: `backend/internal/api/openapi_oidc_test.go`
- Regenerate: `backend/openapi.json` and `frontend/src/api/schema.d.ts`. Both are gitignored, so they are not committed.

**Interfaces:**
- Consumes: Task 7 and Task 9 DTOs.
- Produces: schemas `OIDCClient`, `OIDCClientCreateRequest`, `OIDCClientUpdateRequest`, `OIDCClientSecretResponse`; paths `/api/panel/oidc-clients`, `/api/panel/oidc-clients/{id}`, `/api/panel/oidc-clients/{id}/rotate-secret`; operation IDs `listOIDCClients`, `createOIDCClient`, `updateOIDCClient`, `deleteOIDCClient`, `rotateOIDCClientSecret`. The frontend types in Task 11 come from these.

- [ ] **Step 1: Write the failing test**

```go
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
	reason := schemas["SettingsResponse"].Value.Properties["oidc_unavailable_reason"]
	if reason == nil || len(reason.Value.Enum) != 4 {
		t.Fatalf("oidc_unavailable_reason enum = %+v", reason)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestOpenAPIDescribesConnectedApps`
Expected: FAIL with `missing path /api/panel/oidc-clients`.

- [ ] **Step 3: Add schemas, required fields and enums**

In `BuildOpenAPISpec`, add to `schemaDefs` after the `NotificationChannelRevealResponse` entry:

```go
		"OIDCClient":                        OIDCClient{},
		"OIDCClientCreateRequest":           OIDCClientCreateRequest{},
		"OIDCClientUpdateRequest":           OIDCClientUpdateRequest{},
		"OIDCClientSecretResponse":          OIDCClientSecretResponse{},
```

In the required-fields map, replace the `"SettingsResponse"` entry and add three entries:

```go
		"SettingsResponse":               {"email_sendout_rate_per_minute", "oidc_enabled", "oidc_issuer", "oidc_unavailable_reason"},
		"OIDCClient":                     {"id", "client_id", "name", "redirect_uris", "allowed_scopes", "refresh_enabled", "skip_consent", "enabled", "created", "updated"},
		"OIDCClientCreateRequest":        {"name", "redirect_uris"},
		"OIDCClientSecretResponse":       {"client", "client_secret"},
```

After the `NotificationChannelTestResponse` enum block, add:

```go
	if s, ok := schemas["SettingsResponse"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "oidc_unavailable_reason", []any{"", "backend_url_missing", "backend_url_insecure", "frontend_url_missing"})
	}
```

- [ ] **Step 4: Add the paths**

After the `/api/panel/notification-channels/{id}/reveal/finish` path block, add:

```go
	// ── /oidc-clients ─────────────────────────────────────────────────────
	t.Paths.Set("/api/panel/oidc-clients", &openapi3.PathItem{
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listOIDCClients",
				Summary:     "List Connected Apps without their secrets",
				Tags:        []string{"connected apps"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Connected Apps"),
						Content:     content(arrayRef("OIDCClient")),
					},
				})),
			}
			withAuth(op)
			return op
		}(),
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "createOIDCClient",
				Summary:     "Register a Connected App and return its client secret once",
				Tags:        []string{"connected apps"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("OIDCClientCreateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Created Connected App and its plaintext client secret"),
						Content:     content(ref("OIDCClientSecretResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/oidc-clients/{id}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Connected App ID")},
		Patch: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "updateOIDCClient",
				Summary:     "Update a Connected App",
				Tags:        []string{"connected apps"},
				RequestBody: &openapi3.RequestBodyRef{
					Value: openapi3.NewRequestBody().
						WithRequired(true).
						WithJSONSchemaRef(ref("OIDCClientUpdateRequest")),
				},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Updated Connected App"),
						Content:     content(ref("OIDCClient")),
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
				OperationID: "deleteOIDCClient",
				Summary:     "Delete a Connected App with its grants and tokens",
				Tags:        []string{"connected apps"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Deleted Connected App confirmation"),
						Content:     content(ref("DeleteResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})

	t.Paths.Set("/api/panel/oidc-clients/{id}/rotate-secret", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("Connected App ID")},
		Post: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "rotateOIDCClientSecret",
				Summary:     "Replace a Connected App's client secret and return it once",
				Tags:        []string{"connected apps"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Connected App and its new plaintext client secret"),
						Content:     content(ref("OIDCClientSecretResponse")),
					},
				})),
			}
			op.Responses.Set("400", badRequest)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})
```

- [ ] **Step 5: Run the test and regenerate the frontend types**

Run: `go test ./internal/api -run TestOpenAPIDescribesConnectedApps -v` from `backend/`.
Expected: PASS.

Run from `frontend/`: `pnpm api:sync`
Expected: it rewrites `backend/openapi.json` and `frontend/src/api/schema.d.ts`; `grep -c OIDCClient src/api/schema.d.ts` prints a non-zero count.

- [ ] **Step 6: Run the full suite and commit**

Run: `go build ./... && go vet ./... && go test ./...` from `backend/`.
Expected: no build or vet output, every package `ok`.

```bash
git add backend/internal/api/openapi.go backend/internal/api/openapi_oidc_test.go
git commit -m "feat(backend): describe connected apps in openapi"
```

### Task 11: Frontend API layer and copy

**Files:**
- Modify: `frontend/src/api/queries.ts`, `frontend/src/lib/api-error.ts`, `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes: Task 10 schema types (after `pnpm api:sync`).
- Produces: exported types `OIDCClient`, `OIDCClientCreateRequest`, `OIDCClientUpdateRequest`, `OIDCClientSecretResponse`; `queryKeys.oidcClients()`; `fetchOIDCClients`, `createOIDCClient`, `updateOIDCClient`, `deleteOIDCClient`, `rotateOIDCClientSecret`; every message key used in Tasks 11 and 12.

The frontend has no unit tests. `pnpm typecheck` and `pnpm i18n:check` are the gates (AGENTS.md).

- [ ] **Step 1: Add the types and fetchers to `queries.ts`**

Next to the other `components["schemas"]` aliases near the top:

```ts
type OIDCClient = components["schemas"]["OIDCClient"];
type OIDCClientCreateRequest = components["schemas"]["OIDCClientCreateRequest"];
type OIDCClientUpdateRequest = components["schemas"]["OIDCClientUpdateRequest"];
type OIDCClientSecretResponse = components["schemas"]["OIDCClientSecretResponse"];
```

Add the four names to the `export type { ... }` list after `NotificationChannelRevealResponse`. In `queryKeys`, after `notificationChannels`:

```ts
  oidcClients: () => [...queryKeys.all, "oidc-clients"] as const,
```

After `testNotificationChannel` and the other notification-channel fetchers:

```ts
export function fetchOIDCClients(): Promise<OIDCClient[]> {
  return apiRequest<OIDCClient[]>(
    apiClient.GET("/api/panel/oidc-clients"),
    m.error_oidc_clients_load()
  );
}

export function createOIDCClient(body: OIDCClientCreateRequest): Promise<OIDCClientSecretResponse> {
  return apiRequest<OIDCClientSecretResponse>(
    apiClient.POST("/api/panel/oidc-clients", { body }),
    m.error_oidc_client_save(),
    m.error_oidc_client_save_network()
  );
}

export function updateOIDCClient(id: string, body: OIDCClientUpdateRequest): Promise<OIDCClient> {
  return apiRequest<OIDCClient>(
    apiClient.PATCH("/api/panel/oidc-clients/{id}", { params: { path: { id } }, body }),
    m.error_oidc_client_save(),
    m.error_oidc_client_save_network()
  );
}

export function deleteOIDCClient(id: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/oidc-clients/{id}", { params: { path: { id } } }),
    m.error_oidc_client_delete(),
    m.error_oidc_client_delete_network()
  );
}

export function rotateOIDCClientSecret(id: string): Promise<OIDCClientSecretResponse> {
  return apiRequest<OIDCClientSecretResponse>(
    apiClient.POST("/api/panel/oidc-clients/{id}/rotate-secret", { params: { path: { id } } }),
    m.error_oidc_client_rotate(),
    m.error_oidc_client_rotate_network()
  );
}
```

- [ ] **Step 2: Localize the backend errors**

Add to `API_ERROR_MAP` in `frontend/src/lib/api-error.ts`. Keys are lowercase without the trailing period, as `localizeApiError` normalizes them:

```ts
  "connected app name must be between 1 and 128 characters": () => m.error_oidc_client_name_invalid(),
  "connected app name cannot contain control characters": () => m.error_oidc_client_name_invalid(),
  "connected app not found": () => m.error_oidc_client_not_found(),
  "redirect_uris must contain between 1 and 10 entries": () => m.error_oidc_redirect_uris_count(),
  "redirect uri must be an absolute https url, or http on localhost, without a fragment or credentials": () =>
    m.error_oidc_redirect_uri_invalid(),
  "redirect uris must be unique": () => m.error_oidc_redirect_uris_duplicate(),
  "allowed_scopes must include openid and only contain openid or email": () => m.error_oidc_scopes_invalid(),
  "oidc provider requires an https panel_backend_url_base and panel_frontend_url_base": () =>
    m.error_oidc_unavailable(),
```

- [ ] **Step 3: Add the copy**

Append these keys to `frontend/messages/en.json`:

```json
  "settings_oidc": "OpenID Connect",
  "settings_oidc_desc": "Let sites you register sign users in with their panel account.",
  "settings_enable_oidc": "Enable OpenID Connect provider",
  "settings_oidc_enabled_desc": "Connected apps can sign in active users whose email is verified.",
  "settings_oidc_disabled_desc": "Every OpenID Connect endpoint returns 404.",
  "settings_oidc_issuer": "Issuer",
  "settings_oidc_unavailable_backend_url_missing": "Set PANEL_BACKEND_URL_BASE to enable it",
  "settings_oidc_unavailable_backend_url_insecure": "PANEL_BACKEND_URL_BASE must use https unless it points to localhost",
  "settings_oidc_unavailable_frontend_url_missing": "Set PANEL_FRONTEND_URL_BASE to enable it",
  "settings_oidc_manage": "Connected apps",
  "settings_oidc_manage_desc": "Register sites, their redirect URIs and client secrets.",
  "oidc_clients_title": "Connected apps",
  "oidc_clients_new": "New app",
  "oidc_clients_empty_title": "No connected apps",
  "oidc_clients_empty_hint": "Register a site to let it sign users in with their panel account.",
  "oidc_clients_provider_off": "The OpenID Connect provider is off. Apps cannot sign users in until it is turned on.",
  "oidc_clients_discovery": "Discovery URL",
  "oidc_clients_redirect_count": "Redirect URIs: {count}",
  "oidc_clients_last_used": "Last used {time}",
  "oidc_clients_enabled": "Enabled",
  "oidc_clients_disabled": "Disabled",
  "oidc_clients_more_actions": "More actions",
  "oidc_clients_edit": "Edit",
  "oidc_clients_rotate_secret": "Rotate secret",
  "oidc_clients_rotating": "Rotating…",
  "oidc_clients_rotate_title": "Rotate client secret",
  "oidc_clients_rotate_body": "The current secret stops working right away. Update {name} with the new secret.",
  "oidc_clients_delete_title": "Delete connected app",
  "oidc_clients_delete_confirm": "Delete {name}? Users can no longer sign in to it with their panel account, and their consent is removed.",
  "oidc_clients_create_title": "New connected app",
  "oidc_clients_edit_title": "Edit connected app",
  "oidc_clients_form_description": "Users see this name on the consent screen.",
  "oidc_clients_name_label": "Name",
  "oidc_clients_redirect_label": "Redirect URIs",
  "oidc_clients_redirect_hint": "One per line, matched exactly. Use https, or http only for localhost.",
  "oidc_clients_scope_email_label": "Share email address",
  "oidc_clients_scope_email_desc": "Lets the app request the email scope.",
  "oidc_clients_refresh_label": "Allow refresh tokens",
  "oidc_clients_refresh_desc": "Issued only when the app requests offline_access.",
  "oidc_clients_skip_consent_label": "Skip consent screen",
  "oidc_clients_skip_consent_desc": "Only for apps you run yourself. Users sign in without being asked.",
  "oidc_clients_enabled_label": "Enabled",
  "oidc_clients_create_submit": "Create",
  "oidc_clients_save": "Save",
  "oidc_clients_saving": "Saving…",
  "oidc_clients_credentials_title": "Client credentials",
  "oidc_clients_client_id": "Client ID",
  "oidc_clients_client_secret": "Client secret",
  "oidc_clients_secret_warning": "Copy the secret now. It is not shown again",
  "oidc_clients_credentials_done": "Done",
  "oidc_copy_issuer": "issuer",
  "oidc_copy_discovery": "discovery URL",
  "oidc_copy_client_id": "client ID",
  "oidc_copy_client_secret": "client secret",
  "error_oidc_clients_load": "Couldn't load connected apps.",
  "error_oidc_client_save": "Couldn't save the connected app.",
  "error_oidc_client_save_network": "Network error while saving the connected app.",
  "error_oidc_client_delete": "Couldn't delete the connected app.",
  "error_oidc_client_delete_network": "Network error while deleting the connected app.",
  "error_oidc_client_rotate": "Couldn't rotate the client secret.",
  "error_oidc_client_rotate_network": "Network error while rotating the client secret.",
  "error_oidc_client_name_invalid": "Enter a name of 1 to 128 characters without line breaks.",
  "error_oidc_client_not_found": "This connected app no longer exists.",
  "error_oidc_redirect_uris_count": "Enter between 1 and 10 redirect URIs.",
  "error_oidc_redirect_uri_invalid": "Each redirect URI must be an absolute https URL, or http on localhost, without a fragment or credentials.",
  "error_oidc_redirect_uris_duplicate": "Each redirect URI can appear only once.",
  "error_oidc_scopes_invalid": "Scopes must include openid.",
  "error_oidc_unavailable": "Set an https PANEL_BACKEND_URL_BASE and PANEL_FRONTEND_URL_BASE first."
```

Append the same keys to `frontend/messages/zh-cn.json`:

```json
  "settings_oidc": "OpenID Connect",
  "settings_oidc_desc": "让你登记的网站使用面板账号登录。",
  "settings_enable_oidc": "启用 OpenID Connect 提供方",
  "settings_oidc_enabled_desc": "已连接的应用可以让已启用且邮箱已验证的用户登录。",
  "settings_oidc_disabled_desc": "所有 OpenID Connect 端点都返回 404。",
  "settings_oidc_issuer": "Issuer",
  "settings_oidc_unavailable_backend_url_missing": "设置 PANEL_BACKEND_URL_BASE 后才能启用",
  "settings_oidc_unavailable_backend_url_insecure": "PANEL_BACKEND_URL_BASE 必须使用 https，localhost 除外",
  "settings_oidc_unavailable_frontend_url_missing": "设置 PANEL_FRONTEND_URL_BASE 后才能启用",
  "settings_oidc_manage": "已连接的应用",
  "settings_oidc_manage_desc": "登记网站及其回调地址和 client secret。",
  "oidc_clients_title": "已连接的应用",
  "oidc_clients_new": "新建应用",
  "oidc_clients_empty_title": "还没有已连接的应用",
  "oidc_clients_empty_hint": "登记一个网站后，它就能让用户用面板账号登录。",
  "oidc_clients_provider_off": "OpenID Connect 提供方已关闭。开启之前，应用无法让用户登录。",
  "oidc_clients_discovery": "Discovery 地址",
  "oidc_clients_redirect_count": "回调地址：{count}",
  "oidc_clients_last_used": "最近使用 {time}",
  "oidc_clients_enabled": "已启用",
  "oidc_clients_disabled": "已停用",
  "oidc_clients_more_actions": "更多操作",
  "oidc_clients_edit": "编辑",
  "oidc_clients_rotate_secret": "轮换 secret",
  "oidc_clients_rotating": "轮换中…",
  "oidc_clients_rotate_title": "轮换 client secret",
  "oidc_clients_rotate_body": "当前 secret 会立即失效。请在 {name} 中换成新的 secret。",
  "oidc_clients_delete_title": "删除已连接的应用",
  "oidc_clients_delete_confirm": "删除 {name}？用户将无法再用面板账号登录它，相关授权记录也会删除。",
  "oidc_clients_create_title": "新建已连接的应用",
  "oidc_clients_edit_title": "编辑已连接的应用",
  "oidc_clients_form_description": "用户会在授权页看到这个名称。",
  "oidc_clients_name_label": "名称",
  "oidc_clients_redirect_label": "回调地址",
  "oidc_clients_redirect_hint": "每行一个，需完全一致。使用 https，仅 localhost 可用 http。",
  "oidc_clients_scope_email_label": "提供邮箱地址",
  "oidc_clients_scope_email_desc": "允许应用请求 email scope。",
  "oidc_clients_refresh_label": "允许 refresh token",
  "oidc_clients_refresh_desc": "仅在应用请求 offline_access 时签发。",
  "oidc_clients_skip_consent_label": "跳过授权确认",
  "oidc_clients_skip_consent_desc": "只用于你自己运营的应用。用户登录时不会被询问。",
  "oidc_clients_enabled_label": "启用",
  "oidc_clients_create_submit": "创建",
  "oidc_clients_save": "保存",
  "oidc_clients_saving": "保存中…",
  "oidc_clients_credentials_title": "Client 凭据",
  "oidc_clients_client_id": "Client ID",
  "oidc_clients_client_secret": "Client secret",
  "oidc_clients_secret_warning": "现在复制 secret，之后不会再显示",
  "oidc_clients_credentials_done": "完成",
  "oidc_copy_issuer": "Issuer",
  "oidc_copy_discovery": "Discovery 地址",
  "oidc_copy_client_id": "Client ID",
  "oidc_copy_client_secret": "Client secret",
  "error_oidc_clients_load": "无法加载已连接的应用。",
  "error_oidc_client_save": "无法保存已连接的应用。",
  "error_oidc_client_save_network": "保存已连接的应用时网络错误。",
  "error_oidc_client_delete": "无法删除已连接的应用。",
  "error_oidc_client_delete_network": "删除已连接的应用时网络错误。",
  "error_oidc_client_rotate": "无法轮换 client secret。",
  "error_oidc_client_rotate_network": "轮换 client secret 时网络错误。",
  "error_oidc_client_name_invalid": "请输入 1 到 128 个字符的名称，不含换行。",
  "error_oidc_client_not_found": "该应用已不存在。",
  "error_oidc_redirect_uris_count": "请填写 1 到 10 个回调地址。",
  "error_oidc_redirect_uri_invalid": "每个回调地址都必须是绝对 https URL，localhost 可用 http，且不能包含 fragment 或凭据。",
  "error_oidc_redirect_uris_duplicate": "回调地址不能重复。",
  "error_oidc_scopes_invalid": "scope 必须包含 openid。",
  "error_oidc_unavailable": "请先设置 https 的 PANEL_BACKEND_URL_BASE 和 PANEL_FRONTEND_URL_BASE。"
```

- [ ] **Step 4: Verify**

Run from `frontend/`: `pnpm typecheck && pnpm i18n:check`
Expected: both pass. The new message keys are unused until Task 12, which Paraglide allows.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/queries.ts frontend/src/lib/api-error.ts frontend/messages/en.json frontend/messages/zh-cn.json
git commit -m "feat(frontend): add connected app api and copy"
```

### Task 12: Connected Apps page and settings entry

**Files:**
- Create: `frontend/src/routes/settings/connected-apps.tsx`
- Modify: `frontend/src/routes/settings/index.tsx`, `frontend/src/routeTree.gen.ts` (regenerated)

**Interfaces:**
- Consumes: Task 11 fetchers, types and messages; `fetchSettings` for the issuer and switch state.
- Produces: route `/settings/connected-apps` (admin only).

- [ ] **Step 1: Write the page**

Check the HeroUI v3 docs for `TextArea` inside `TextField` first (the heroui-react MCP `get_component_docs` for `TextArea`). If `TextField` does not accept a `TextArea` child, render `<Label>` and a standalone `<TextArea aria-label=...>` as `src/routes/users/$userId.tsx` does.

`frontend/src/routes/settings/connected-apps.tsx`:

```tsx
import { useState, type Key } from "react";
import { useForm } from "@tanstack/react-form";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  Button,
  Dropdown,
  Input,
  Label,
  Modal,
  Separator,
  Switch,
  TextArea,
  TextField,
} from "@heroui/react";
import { ArrowRotateRight, Ellipsis, Pencil, Plus, TrashBin } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  canQueryPanelApi,
  createOIDCClient,
  deleteOIDCClient,
  fetchOIDCClients,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  rotateOIDCClientSecret,
  updateOIDCClient,
  type OIDCClient,
  type OIDCClientCreateRequest,
  type OIDCClientSecretResponse,
  type OIDCClientUpdateRequest,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  CopyableCode,
  DestructiveConfirmModal,
  Dot,
  ErrorAlert,
  LabeledSwitch,
  PageShell,
  PanelMessage,
  Section,
  TableSkeleton,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/connected-apps")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.oidc_clients_title(),
  }),
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData({
      queryKey: queryKeys.oidcClients(),
      queryFn: fetchOIDCClients,
    });
  },
  component: ConnectedAppsPage,
});

function ConnectedAppsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const now = useHydratedNow();
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<OIDCClient | null>(null);
  const [deleting, setDeleting] = useState<OIDCClient | null>(null);
  const [rotating, setRotating] = useState<OIDCClient | null>(null);
  const [credentials, setCredentials] = useState<OIDCClientSecretResponse | null>(null);

  const clientsQuery = useQuery({ queryKey: queryKeys.oidcClients(), queryFn: fetchOIDCClients });
  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi(),
  });

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: queryKeys.oidcClients() });

  const createMutation = useMutation({
    mutationFn: createOIDCClient,
    onSuccess: (data) => {
      setCreateOpen(false);
      setCredentials(data);
      invalidate();
    },
  });
  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: string; body: OIDCClientUpdateRequest }) =>
      updateOIDCClient(id, body),
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
  });
  const rotateMutation = useMutation({
    mutationFn: rotateOIDCClientSecret,
    onSuccess: (data) => {
      setRotating(null);
      setCredentials(data);
      invalidate();
    },
  });
  const deleteMutation = useMutation({
    mutationFn: deleteOIDCClient,
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
  });

  const clients = clientsQuery.data ?? [];
  const issuer = settingsQuery.data?.oidc_issuer ?? "";
  const providerOff = settingsQuery.data !== undefined && !settingsQuery.data.oidc_enabled;
  const loadError = clientsQuery.error ? queryErrorMessage(clientsQuery.error) : "";
  // Row switches share the update mutation with the edit modal, whose own
  // error renders inside the modal.
  const toggleError =
    updateMutation.error && editing === null
      ? queryErrorMessage(updateMutation.error, m.error_oidc_client_save_network())
      : "";

  function openCreate(open: boolean) {
    setCreateOpen(open);
    if (!open) createMutation.reset();
  }

  function closeEdit(open: boolean) {
    if (!open) {
      setEditing(null);
      updateMutation.reset();
    }
  }

  function closeRotate(open: boolean) {
    if (!open) {
      setRotating(null);
      rotateMutation.reset();
    }
  }

  function closeDelete(open: boolean) {
    if (!open) {
      setDeleting(null);
      deleteMutation.reset();
    }
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-6">
        <h1 className="text-base font-semibold tracking-tight">{m.settings_oidc()}</h1>
        <p className="mt-0.5 max-w-2xl text-[13px] text-muted">{m.settings_oidc_desc()}</p>
      </div>

      {providerOff && (
        <div className="mb-4 rounded-lg border bg-surface-secondary px-4 py-3 text-[13px] text-muted">
          {m.oidc_clients_provider_off()}{" "}
          <Link to="/settings" className="font-medium text-accent hover:opacity-80">
            {m.nav_settings()}
          </Link>
        </div>
      )}

      {issuer && (
        <div className="mb-6">
          <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
            {m.oidc_clients_discovery()}
          </p>
          <CopyableCode
            value={`${issuer}/.well-known/openid-configuration`}
            label={m.oidc_copy_discovery()}
          />
        </div>
      )}

      <ErrorAlert message={loadError || toggleError} className="mb-4" />

      <ClientFormModal
        key={createOpen ? "create-open" : "create-closed"}
        isOpen={createOpen}
        onOpenChange={openCreate}
        pending={createMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(createMutation.error, m.error_oidc_client_save_network())
            : ""
        }
        onSubmit={(body) => createMutation.mutate(body)}
      />
      <ClientFormModal
        key={editing ? `edit-${editing.id}` : "edit-closed"}
        client={editing}
        isOpen={editing !== null}
        onOpenChange={closeEdit}
        pending={updateMutation.isPending}
        error={
          updateMutation.error && editing !== null
            ? queryErrorMessage(updateMutation.error, m.error_oidc_client_save_network())
            : ""
        }
        onSubmit={(body) => editing && updateMutation.mutate({ id: editing.id, body })}
      />
      <DestructiveConfirmModal
        isOpen={rotating !== null}
        title={m.oidc_clients_rotate_title()}
        body={m.oidc_clients_rotate_body({ name: rotating?.name ?? "" })}
        confirmLabel={m.oidc_clients_rotate_secret()}
        pendingLabel={m.oidc_clients_rotating()}
        pending={rotateMutation.isPending}
        error={
          rotateMutation.error
            ? queryErrorMessage(rotateMutation.error, m.error_oidc_client_rotate_network())
            : ""
        }
        onOpenChange={closeRotate}
        onConfirm={() => rotating && rotateMutation.mutate(rotating.id)}
      />
      <DestructiveConfirmModal
        isOpen={deleting !== null}
        title={m.oidc_clients_delete_title()}
        body={m.oidc_clients_delete_confirm({ name: deleting?.name ?? "" })}
        confirmLabel={m.common_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={
          deleteMutation.error
            ? queryErrorMessage(deleteMutation.error, m.error_oidc_client_delete_network())
            : ""
        }
        onOpenChange={closeDelete}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
      <CredentialsModal credentials={credentials} onClose={() => setCredentials(null)} />

      <Section
        className="mt-0"
        title={m.oidc_clients_title()}
        action={
          <Button size="sm" variant="secondary" onPress={() => setCreateOpen(true)}>
            <Plus className="size-3.5" aria-hidden />
            {m.oidc_clients_new()}
          </Button>
        }
      >
        {clientsQuery.isPending ? (
          <TableSkeleton />
        ) : clients.length === 0 ? (
          <PanelMessage>
            <span className="block font-medium text-foreground">{m.oidc_clients_empty_title()}</span>
            <span className="mt-1 block">{m.oidc_clients_empty_hint()}</span>
          </PanelMessage>
        ) : (
          <div className="divide-y divide-separator">
            {clients.map((client) => (
              <ClientRow
                key={client.id}
                client={client}
                now={now}
                toggling={updateMutation.isPending && updateMutation.variables?.id === client.id}
                onToggle={(enabled) => updateMutation.mutate({ id: client.id, body: { enabled } })}
                onEdit={() => setEditing(client)}
                onRotate={() => setRotating(client)}
                onDelete={() => setDeleting(client)}
              />
            ))}
          </div>
        )}
      </Section>
    </PageShell>
  );
}

const menuItemClass =
  "h-7 rounded-md px-2 text-[13px] text-foreground transition-colors duration-150 hover:bg-surface-secondary data-[focus=true]:bg-surface-secondary data-[hovered=true]:bg-surface-secondary";
const menuDangerItemClass =
  "h-7 rounded-md px-2 text-[13px] text-danger transition-colors duration-150 hover:bg-danger-soft data-[focus=true]:bg-danger-soft data-[hovered=true]:bg-danger-soft";
const menuIconClass = "size-3.5 shrink-0 text-muted";

function ClientRow({
  client,
  now,
  toggling,
  onToggle,
  onEdit,
  onRotate,
  onDelete,
}: {
  client: OIDCClient;
  now: number | null;
  toggling: boolean;
  onToggle: (enabled: boolean) => void;
  onEdit: () => void;
  onRotate: () => void;
  onDelete: () => void;
}) {
  const stateLabel = client.enabled ? m.oidc_clients_enabled() : m.oidc_clients_disabled();
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone={client.enabled ? "ok" : "idle"} />
          <span className="truncate text-[13px] font-medium">{client.name}</span>
        </div>
        <p className="mt-1 truncate font-mono text-xs text-muted">{client.client_id}</p>
        <p className="mt-0.5 text-xs text-muted">
          {m.oidc_clients_redirect_count({ count: client.redirect_uris.length })}
          {client.last_used_at && now !== null && (
            <>
              <span> · </span>
              <span className="tabular-nums">
                {m.oidc_clients_last_used({ time: relTimeFromISO(client.last_used_at, now) })}
              </span>
            </>
          )}
        </p>
      </div>
      <div className="flex items-center justify-between gap-1.5 sm:flex-shrink-0">
        <Switch
          aria-label={stateLabel}
          isSelected={client.enabled}
          isDisabled={toggling}
          onChange={onToggle}
        >
          <Switch.Content>
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
            <span className="text-xs text-muted">{stateLabel}</span>
          </Switch.Content>
        </Switch>
        <Dropdown>
          <Button
            size="sm"
            variant="ghost"
            aria-label={m.oidc_clients_more_actions()}
            className="px-1.5"
          >
            <Ellipsis className="size-4 text-muted" aria-hidden />
          </Button>
          <Dropdown.Popover
            placement="bottom end"
            className="min-w-40 rounded-lg border bg-surface p-1 shadow-none"
          >
            <Dropdown.Menu
              onAction={(key: Key) => {
                if (key === "edit") onEdit();
                else if (key === "rotate") onRotate();
                else if (key === "delete") onDelete();
              }}
            >
              <Dropdown.Item id="edit" textValue={m.oidc_clients_edit()} className={menuItemClass}>
                <Pencil className={menuIconClass} aria-hidden />
                <Label className="truncate">{m.oidc_clients_edit()}</Label>
              </Dropdown.Item>
              <Dropdown.Item
                id="rotate"
                textValue={m.oidc_clients_rotate_secret()}
                className={menuItemClass}
              >
                <ArrowRotateRight className={menuIconClass} aria-hidden />
                <Label className="truncate">{m.oidc_clients_rotate_secret()}</Label>
              </Dropdown.Item>
              <Separator className="my-1 bg-separator" />
              <Dropdown.Item
                id="delete"
                variant="danger"
                textValue={m.common_delete()}
                className={menuDangerItemClass}
              >
                <TrashBin className="size-3.5 shrink-0 text-current" aria-hidden />
                <Label className="truncate">{m.common_delete()}</Label>
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
      </div>
    </div>
  );
}

function splitLines(value: string): string[] {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function ClientFormModal({
  client,
  isOpen,
  onOpenChange,
  pending,
  error,
  onSubmit,
}: {
  client?: OIDCClient | null;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  onSubmit: (body: OIDCClientCreateRequest) => void;
}) {
  const isNew = !client;
  const form = useForm({
    defaultValues: {
      name: client?.name ?? "",
      redirectURIs: (client?.redirect_uris ?? []).join("\n"),
      shareEmail: client ? client.allowed_scopes.includes("email") : true,
      refreshEnabled: client?.refresh_enabled ?? false,
      skipConsent: client?.skip_consent ?? false,
      enabled: client?.enabled ?? true,
    },
    onSubmit: ({ value }) => {
      onSubmit({
        name: value.name.trim(),
        redirect_uris: splitLines(value.redirectURIs),
        allowed_scopes: value.shareEmail ? ["openid", "email"] : ["openid"],
        refresh_enabled: value.refreshEnabled,
        skip_consent: value.skipConsent,
        enabled: value.enabled,
      });
    },
  });

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="md" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(event) => {
              event.preventDefault();
              event.stopPropagation();
              void form.handleSubmit();
            }}
          >
            <Modal.Header>
              <Modal.Heading>
                {isNew ? m.oidc_clients_create_title() : m.oidc_clients_edit_title()}
              </Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-muted">
                {m.oidc_clients_form_description()}
              </p>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <form.Field name="name">
                  {(field) => (
                    <TextField
                      value={field.state.value}
                      onChange={field.handleChange}
                      onBlur={field.handleBlur}
                      autoFocus
                    >
                      <Label>{m.oidc_clients_name_label()}</Label>
                      <Input autoComplete="off" maxLength={128} />
                    </TextField>
                  )}
                </form.Field>
                <form.Field name="redirectURIs">
                  {(field) => (
                    <TextField
                      value={field.state.value}
                      onChange={field.handleChange}
                      onBlur={field.handleBlur}
                    >
                      <Label>{m.oidc_clients_redirect_label()}</Label>
                      <TextArea
                        rows={3}
                        spellCheck={false}
                        className="font-mono text-xs"
                        style={{ resize: "vertical" }}
                      />
                    </TextField>
                  )}
                </form.Field>
                <p className="-mt-2 text-xs text-muted">{m.oidc_clients_redirect_hint()}</p>
                <form.Field name="shareEmail">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_scope_email_label()}
                      description={m.oidc_clients_scope_email_desc()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
                <form.Field name="refreshEnabled">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_refresh_label()}
                      description={m.oidc_clients_refresh_desc()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
                <form.Field name="skipConsent">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_skip_consent_label()}
                      description={m.oidc_clients_skip_consent_desc()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
                <form.Field name="enabled">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_enabled_label()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
              </div>
              {error && (
                <p className="mt-4 text-[13px] text-danger" role="alert">
                  {error}
                </p>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {m.common_cancel()}
              </Button>
              <form.Subscribe
                selector={(state) => ({
                  name: state.values.name,
                  redirectURIs: state.values.redirectURIs,
                })}
              >
                {({ name, redirectURIs }) => (
                  <Button
                    type="submit"
                    variant="primary"
                    isPending={pending}
                    isDisabled={pending || !name.trim() || splitLines(redirectURIs).length === 0}
                  >
                    {pending
                      ? m.oidc_clients_saving()
                      : isNew
                        ? m.oidc_clients_create_submit()
                        : m.oidc_clients_save()}
                  </Button>
                )}
              </form.Subscribe>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

// The secret exists only in this response, so the modal stays until the
// admin dismisses it.
function CredentialsModal({
  credentials,
  onClose,
}: {
  credentials: OIDCClientSecretResponse | null;
  onClose: () => void;
}) {
  return (
    <Modal.Backdrop isOpen={credentials !== null} onOpenChange={(open) => !open && onClose()}>
      <Modal.Container size="md" placement="auto">
        <Modal.Dialog>
          <Modal.Header>
            <Modal.Heading>{m.oidc_clients_credentials_title()}</Modal.Heading>
          </Modal.Header>
          <Modal.Body>
            <div className="flex flex-col gap-4">
              <p className="text-[13px] font-medium">{credentials?.client.name}</p>
              <div>
                <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
                  {m.oidc_clients_client_id()}
                </p>
                <CopyableCode
                  value={credentials?.client.client_id ?? ""}
                  label={m.oidc_copy_client_id()}
                />
              </div>
              <div>
                <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
                  {m.oidc_clients_client_secret()}
                </p>
                <CopyableCode
                  value={credentials?.client_secret ?? ""}
                  label={m.oidc_copy_client_secret()}
                />
              </div>
              <div className="rounded-lg border border-warning bg-warning-soft px-4 py-3">
                <p className="text-[13px] font-semibold text-warning-soft-foreground">
                  {m.oidc_clients_secret_warning()}
                </p>
              </div>
            </div>
          </Modal.Body>
          <Modal.Footer>
            <Button variant="primary" onPress={onClose}>
              {m.oidc_clients_credentials_done()}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
```

- [ ] **Step 2: Add the provider section and link to the settings page**

In `frontend/src/routes/settings/index.tsx`, add `PlugConnection` to the `@gravity-ui/icons` import. Render the section right before `<ManagementApiSection`, so a save error from the shared settings mutation shows in the `ErrorAlert` that section already renders below it:

```tsx
          <OIDCSection
            settings={settings}
            pending={mutation.isPending}
            onSave={(patch) => mutation.mutate(patch)}
          />
```

Add the component after `ManagementApiSection`:

```tsx
function OIDCSection({
  settings,
  pending,
  onSave,
}: {
  settings: AppSettings | undefined;
  pending: boolean;
  onSave: (patch: SettingsUpdateRequest) => void;
}) {
  const enabled = settings?.oidc_enabled ?? false;
  const issuer = settings?.oidc_issuer ?? "";
  const unavailable = oidcUnavailableMessage(settings?.oidc_unavailable_reason ?? "");

  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">{m.settings_oidc()}</h1>
        <p className="mt-0.5 text-[13px] text-muted">{m.settings_oidc_desc()}</p>
      </div>

      <div className="flex flex-col gap-4 rounded-lg border bg-surface p-5">
        <LabeledSwitch
          label={m.settings_enable_oidc()}
          description={enabled ? m.settings_oidc_enabled_desc() : m.settings_oidc_disabled_desc()}
          isSelected={enabled}
          isDisabled={!settings || pending || (!enabled && unavailable !== "")}
          onChange={(value) => onSave({ oidc_enabled: value })}
        />
        {unavailable && <p className="text-xs text-warning-soft-foreground">{unavailable}</p>}
        {issuer && (
          <>
            <div className="h-px bg-separator" />
            <div>
              <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
                {m.settings_oidc_issuer()}
              </p>
              <CopyableCode value={issuer} label={m.oidc_copy_issuer()} />
            </div>
          </>
        )}
      </div>

      <Link
        to="/settings/connected-apps"
        className="group mt-3 flex items-center gap-3 rounded-lg border bg-surface px-4 py-3.5 transition-colors duration-150 hover:bg-surface-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border bg-surface-secondary text-muted">
          <PlugConnection className="size-4" aria-hidden />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[13px] font-medium text-foreground">
            {m.settings_oidc_manage()}
          </span>
          <span className="block text-xs text-muted">{m.settings_oidc_manage_desc()}</span>
        </span>
        <ChevronRight
          className="size-4 shrink-0 text-muted transition-colors duration-150 group-hover:text-foreground"
          aria-hidden
        />
      </Link>
    </>
  );
}

function oidcUnavailableMessage(reason: string): string {
  switch (reason) {
    case "backend_url_missing":
      return m.settings_oidc_unavailable_backend_url_missing();
    case "backend_url_insecure":
      return m.settings_oidc_unavailable_backend_url_insecure();
    case "frontend_url_missing":
      return m.settings_oidc_unavailable_frontend_url_missing();
    default:
      return "";
  }
}
```


- [ ] **Step 3: Regenerate the route tree and verify**

Run from `frontend/`: `pnpm build` (the TanStack router plugin rewrites `src/routeTree.gen.ts`), then `pnpm typecheck && pnpm i18n:check`.
Expected: build succeeds, `git diff --stat src/routeTree.gen.ts` shows the new route, typecheck and i18n check pass.

- [ ] **Step 4: Check the page in a browser**

Run `pnpm dev` from the repo root with `PANEL_MASTER_KEY` set and `PANEL_BACKEND_URL_BASE=http://localhost:8090`, `PANEL_FRONTEND_URL_BASE=http://localhost:3000` in the backend environment. As an admin:

1. `/settings` shows the OpenID Connect section. The switch turns on, and the issuer `http://localhost:8090/api/oidc` appears.
2. `/settings/connected-apps`: create an app with `http://localhost:3001/api/auth/callback/panel`. The credentials modal shows client ID and secret. After closing it, the secret is gone from the page.
3. Edit the app, switch it off from the row, rotate its secret (new secret shown), delete it.
4. Submitting `http://example.com/cb` shows the localized redirect URI error in the modal. Switch the locale to 中文 and repeat once.
5. Check light and dark themes.

Expected: every step behaves as described and no copy shows a raw message key.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/settings/connected-apps.tsx frontend/src/routes/settings/index.tsx frontend/src/routeTree.gen.ts
git commit -m "feat(frontend): manage connected apps"
```

### Layer 2 exit check

- [ ] Backend: `go build ./... && go vet ./... && go test ./...` passes.
- [ ] Frontend: `pnpm typecheck && pnpm i18n:check && pnpm build` passes.
- [ ] A non-admin token gets 403 on `GET /api/panel/oidc-clients`.
- [ ] No response body from `/api/panel/oidc-clients*` other than create and rotate contains a client secret, and none contains `secret_hash`.
- [ ] Stop for review.

---

## Layer 3: `oidc/3-protocol`

Branch `oidc/3-protocol` from `oidc/2-connected-apps` (including the follow-up commits `aa8c4f2` and `911b9b5`).

### Decisions for this layer

- **Serving gate.** Every `/api/oidc/*` route and every consent endpoint returns 404 unless `oidc_enabled` is true, `oidcAvailability()` reports no reason, and the pinned issuer equals the current one.
- **Issuer pinning.** Turning the switch on stores the issuer in a new `app_settings.oidc_issuer` field. Turning it off clears it. When `PANEL_BACKEND_URL_BASE` later yields a different issuer, the settings response reports `issuer_changed` and the endpoints return 404 until an admin turns the switch off and on. An empty pin means the switch was set outside the panel, for example in `/_/`, so the first served request pins the current issuer.
- **Lazy signing key.** JWKS and ID token signing call `oidc.ActiveSigningKey`, which creates the key when the switch was set outside the panel.
- **Scopes.** The granted scope is the intersection of the request with the app's `allowed_scopes`, plus `offline_access` when `refresh_enabled` is on. Unknown scopes such as `profile` are dropped, as RFC 6749 3.3 allows, because next-auth and most other OIDC clients request `profile` by default. A request without `openid` fails with `invalid_scope`.
- **`prompt=select_account`.** The research table said to reject it with `account_selection_required`. Instead it always shows the consent page, which names the signed-in account and offers to switch. This counts as prompting for account selection, and RPs that always send `select_account` keep working.
- **`max_age=0`.** It is stored as `prompt=login`, because the `max_age` field uses 0 for "absent". Combined with `prompt=none` it fails at `/authorize` with `login_required`.
- **`prompt=login` timing.** Re-authentication must happen after the authorization request was created. The creation time is derived from `expires_at - AuthRequestTTL`, so tests can inject `now`.
- **Single use.** Authorization requests, codes and refresh tokens are claimed with a conditional `UPDATE ... WHERE <field> = ''` inside the transaction, and the affected row count decides who won.
- **Replay.** OAuth errors return from the transaction function with a nil Go error, so a replay's family revocation commits even though the request fails. Go errors roll back and become `server_error`.
- **Refresh scope narrowing.** A narrower `scope` on refresh applies to the new access token and ID token only. The new refresh token keeps the granted scope, as RFC 6749 6 describes.
- **Rate limits.** These per-IP limits replace the research's 30 per minute on `/token`, because one relying party backend sends every user's token requests from a single address:
  - `/authorize` and abort: 60 per minute.
  - `/token`, `/userinfo` and `/revoke`: 300 per minute.
  - Failed client authentication: 10 per minute. After 10 failures the address is blocked until the window ends, even with the right secret.
- **Error page reasons.** The frontend error page at `/oidc/error?reason=` arrives in layer 4. This layer redirects there with `invalid_request`, `invalid_client`, `invalid_redirect_uri` or `rate_limited`.
- **Consent endpoints.** The handle path parameter is called `{handle}`. Approving is the consent. The frontend calls approve without showing the page only when the request view says `consent_required: false`.
- **Revocation hook.** It uses `OnRecordUpdate("users")`, which runs for every save, including saves made in `/_/` and by `updateUser`. It compares the record before and after the save, revokes only once the save succeeds, and logs failures without blocking the user update.

### File structure

| File | Responsibility |
|---|---|
| `backend/migrations/1730000034_add_oidc_issuer.go` | `app_settings.oidc_issuer` text field |
| `backend/internal/oidc/oidc.go` | adds prompt constants |
| `backend/internal/oidc/clients.go` | adds `ScopeOfflineAccess`, `FindClient`, `ClientRedirectURIs`, `ClientScopes` |
| `backend/internal/oidc/discovery.go` | endpoint paths and the discovery document |
| `backend/internal/oidc/errors.go` | OAuth error codes and `*Error` |
| `backend/internal/oidc/clientauth.go` | `client_secret_basic` / `client_secret_post` authentication, repeated-parameter check |
| `backend/internal/oidc/authorize.go` | `/authorize` validation, request storage, redirect builders |
| `backend/internal/oidc/grants.go` | `UserMayUse`, grant upsert, scope helpers, `claimOnce`, `revokeFamily`, `RevokeUserGrants` |
| `backend/internal/oidc/consent.go` | pending request lookup, re-authentication and consent checks, approve, deny, abort, code issuance |
| `backend/internal/oidc/tokens.go` | PKCE, code exchange, refresh rotation, token issuance |
| `backend/internal/oidc/idtoken.go` | ID token signing and user claims |
| `backend/internal/oidc/access.go` | access token lookup for userinfo, RFC 7009 revocation |
| `backend/internal/api/oidc_protocol.go` | serving gate, `/api/oidc/*` routes and handlers, rate limits, OAuth error responses |
| `backend/internal/api/oidc_consent.go` | `/api/panel/oidc/requests/{handle}` view, approve, deny, abort |
| `backend/internal/api/oidc_user_hook.go` | grant revocation when a User stops being usable |
| `backend/internal/api/oidc_settings.go` | issuer pinning and `issuer_changed` |
| `frontend/messages/*.json`, `frontend/src/routes/settings/index.tsx` | `issuer_changed` copy |

### Review Focus

- Two exchanges of the same code, sequential or racing, must end with exactly one token set, and that set must be revoked once the second exchange arrives. Other families and the remembered consent stay. Covered in Task 17 (`TestExchangeCodeReplayRevokesOnlyItsFamily`).
- A refresh token replayed after rotation must revoke the replacement tokens too, including the access token issued alongside them. Covered in Task 18 (`TestRefreshReplayRevokesTheWholeFamily`).
- An access token sent as a refresh token, or a refresh token sent to userinfo, must fail even though both live in one table under one unique hash index. Covered in Tasks 18 and 19 (`TestRefreshRejectsAccessTokens`, `TestLookupAccessTokenRejectsRefreshTokens`).
- A User disabled or unverified through a plain record save, as `/_/` does it, must lose userinfo and refresh access on the next request. Covered in Task 20 (`TestDisablingUserRevokesGrantsOnAnySave`).
- After `PANEL_BACKEND_URL_BASE` moves to another valid https origin, the provider must stop answering instead of signing tokens under an issuer that RPs never configured. Covered in Task 13 (`TestOIDCEndpointsReturn404AfterIssuerChange`).

### Task 13: Serving gate, issuer pinning, discovery and JWKS

**Files:**
- Create: `backend/migrations/1730000034_add_oidc_issuer.go`
- Create: `backend/internal/oidc/discovery.go`, `backend/internal/oidc/discovery_test.go`
- Create: `backend/internal/api/oidc_protocol.go`, `backend/internal/api/oidc_protocol_test.go`
- Modify: `backend/internal/oidc/oidc.go` (prompt constants), `backend/internal/oidc/clients.go` (`ScopeOfflineAccess`)
- Modify: `backend/internal/api/settings.go` (`OIDCIssuer`), `backend/internal/api/oidc_settings.go`, `backend/internal/api/api.go` (`h.bindOIDCRoutes(se)`)
- Modify: `backend/internal/api/openapi.go` and `openapi_oidc_test.go` (`issuer_changed` enum value), `backend/internal/api/oidc_settings_test.go`
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`, `frontend/src/routes/settings/index.tsx`

**Interfaces:**
- Consumes: `oidc.Issuer`, `oidc.IssuerPath`, `oidc.ActiveSigningKey`, `oidc.PublicJWKS`, `oidc.SigningAlgorithm`, `h.oidcAvailability()`, `h.settingsRecord()`, `settingsFromRecord`.
- Produces:
  - `oidc.DiscoveryPath`, `AuthorizePath`, `TokenPath`, `UserInfoPath`, `JWKSPath`, `RevocationPath` (strings relative to the issuer).
  - `oidc.ProviderMetadata`, `oidc.Discovery(issuer string) ProviderMetadata`.
  - `oidc.PromptNone`, `PromptLogin`, `PromptConsent`, `PromptSelectAccount`, `oidc.PromptValues []string` (canonical order).
  - `oidc.ScopeOfflineAccess`.
  - `(h *Handlers) oidcStatus(s settings) (issuer, reason string)`, `(h *Handlers) oidcServingIssuer() (string, bool)`, `(h *Handlers) requireOIDCServing() *hook.Handler[*core.RequestEvent]`, `oidcIssuer(e *core.RequestEvent) string`, `(h *Handlers) bindOIDCRoutes(se *core.ServeEvent)`.
  - Test helpers in `oidc_protocol_test.go`: `oidcTestAPI`, `oidcTestFrontend`, `oidcTestIssuer`, `type oidcTestEnv`, `newOIDCRouterEnv(t) *oidcTestEnv`, `(env) setEnabled(bool)`, `(env) do(*http.Request) *httptest.ResponseRecorder`, `(env) get(path string) *httptest.ResponseRecorder`.

- [ ] **Step 1: Write the failing tests**

`backend/internal/oidc/discovery_test.go`:

```go
package oidc

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestDiscoveryAdvertisesTheNarrowProfile(t *testing.T) {
	const issuer = "https://api.example.com/api/oidc"
	meta := Discovery(issuer)
	if meta.Issuer != issuer || meta.AuthorizationEndpoint != issuer+"/authorize" || meta.TokenEndpoint != issuer+"/token" ||
		meta.UserInfoEndpoint != issuer+"/userinfo" || meta.JWKSURI != issuer+"/jwks" || meta.RevocationEndpoint != issuer+"/revoke" {
		t.Fatalf("endpoints = %+v", meta)
	}
	if !slices.Equal(meta.ResponseTypesSupported, []string{"code"}) ||
		!slices.Equal(meta.CodeChallengeMethodsSupported, []string{"S256"}) ||
		!slices.Equal(meta.IDTokenSigningAlgValuesSupported, []string{"RS256"}) ||
		!slices.Equal(meta.GrantTypesSupported, []string{"authorization_code", "refresh_token"}) ||
		!meta.AuthorizationResponseISSParameterSupported {
		t.Fatalf("profile = %+v", meta)
	}
	// request_uri_parameter_supported defaults to true when absent, so the
	// false value must be written out.
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if value, ok := fields["request_uri_parameter_supported"]; !ok || value != false {
		t.Fatalf("request_uri_parameter_supported = %v (present %v)", value, ok)
	}
}
```

`backend/internal/api/oidc_protocol_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/oidc"
)

const (
	oidcTestAPI      = "https://api.example.com"
	oidcTestFrontend = "https://panel.example.com"
	oidcTestIssuer   = oidcTestAPI + "/api/oidc"
)

// oidcTestEnv serves the real router, so tests cover the serving gate, the
// panel's auth middleware and the handlers together. The Connected App and
// User fields are filled by newOIDCTestEnv.
type oidcTestEnv struct {
	t       *testing.T
	app     core.App
	h       *Handlers
	mux     http.Handler
	client  OIDCClient
	secret  string
	user    *core.Record
	session string
}

func newOIDCRouterEnv(t *testing.T) *oidcTestEnv {
	t.Helper()
	app := newMigratedTestApp(t)
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	box, err := cryptobox.New("oidc-test-master-key")
	if err != nil {
		t.Fatalf("cryptobox: %v", err)
	}
	h := Register(&core.ServeEvent{App: app, Router: r}, app, box, nil, nil, nil, nil,
		PanelConfigResponse{APIURL: oidcTestAPI, FrontendURL: oidcTestFrontend})
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("mux: %v", err)
	}
	return &oidcTestEnv{t: t, app: app, h: h, mux: mux}
}

func (env *oidcTestEnv) setEnabled(enabled bool) {
	env.t.Helper()
	e, _ := panelEvent(env.t, env.app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": enabled}, nil)
	if err := env.h.updateSettings(e); err != nil {
		env.t.Fatalf("set oidc_enabled=%v: %v", enabled, err)
	}
}

func (env *oidcTestEnv) do(req *http.Request) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	env.mux.ServeHTTP(res, req)
	return res
}

func (env *oidcTestEnv) get(path string) *httptest.ResponseRecorder {
	return env.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func TestOIDCEndpointsReturn404UnlessServing(t *testing.T) {
	env := newOIDCRouterEnv(t)
	paths := []string{"/api/oidc/.well-known/openid-configuration", "/api/oidc/jwks"}
	for _, path := range paths {
		if res := env.get(path); res.Code != http.StatusNotFound {
			t.Fatalf("off: %s = %d", path, res.Code)
		}
	}
	env.setEnabled(true)
	for _, path := range paths {
		if res := env.get(path); res.Code != http.StatusOK {
			t.Fatalf("on: %s = %d %s", path, res.Code, res.Body.String())
		}
	}
	env.h.publicConfig.FrontendURL = ""
	for _, path := range paths {
		if res := env.get(path); res.Code != http.StatusNotFound {
			t.Fatalf("unavailable: %s = %d", path, res.Code)
		}
	}
}

func TestOIDCEndpointsReturn404AfterIssuerChange(t *testing.T) {
	env := newOIDCRouterEnv(t)
	env.setEnabled(true)
	env.h.publicConfig.APIURL = "https://moved.example.com"
	if res := env.get("/api/oidc/.well-known/openid-configuration"); res.Code != http.StatusNotFound {
		t.Fatalf("after the move = %d", res.Code)
	}
	if reason := env.h.settingsResponse(env.h.loadSettings()).OIDCUnavailableReason; reason != "issuer_changed" {
		t.Fatalf("reason = %q, want issuer_changed", reason)
	}

	env.setEnabled(false)
	env.setEnabled(true)
	res := env.get("/api/oidc/.well-known/openid-configuration")
	var meta oidc.ProviderMetadata
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &meta) != nil || meta.Issuer != "https://moved.example.com/api/oidc" {
		t.Fatalf("after re-enabling = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCOutOfBandEnablePinsIssuerAndCreatesKey(t *testing.T) {
	env := newOIDCRouterEnv(t)
	rec, err := env.h.settingsRecord()
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	rec.Set("oidc_enabled", true)
	if err := env.app.Save(rec); err != nil {
		t.Fatalf("enable in place: %v", err)
	}

	res := env.get("/api/oidc/jwks")
	var jwks oidc.JWKS
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &jwks) != nil || len(jwks.Keys) != 1 {
		t.Fatalf("jwks = %d %s", res.Code, res.Body.String())
	}
	if pinned := env.h.loadSettings().OIDCIssuer; pinned != oidcTestIssuer {
		t.Fatalf("pinned issuer = %q", pinned)
	}
}
```

Append to `backend/internal/api/oidc_settings_test.go`:

```go
func TestOIDCSwitchPinsAndClearsIssuer(t *testing.T) {
	app := newMigratedTestApp(t)
	h := newOIDCHandlers(t, app, "https://api.example.com", "https://panel.example.com")
	for _, step := range []struct {
		enabled bool
		pinned  string
	}{{true, "https://api.example.com/api/oidc"}, {false, ""}} {
		e, _ := panelEvent(t, app, http.MethodPatch, "/api/panel/settings", "", map[string]any{"oidc_enabled": step.enabled}, nil)
		if err := h.updateSettings(e); err != nil {
			t.Fatalf("oidc_enabled=%v: %v", step.enabled, err)
		}
		if got := h.loadSettings().OIDCIssuer; got != step.pinned {
			t.Fatalf("oidc_enabled=%v pinned %q, want %q", step.enabled, got, step.pinned)
		}
	}
}
```

In `backend/internal/api/openapi_oidc_test.go`, change the enum length check from 4 to 5:

```go
	if reason == nil || len(reason.Value.Enum) != 5 {
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run Discovery && go test ./internal/api -run 'OIDC'`
Expected: FAIL to compile with `undefined: Discovery` and `undefined: oidc.ProviderMetadata`.

- [ ] **Step 3: Add the migration, constants and discovery document**

`backend/migrations/1730000034_add_oidc_issuer.go`:

```go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Pin the issuer the OIDC provider published when it was turned on, so a
// later PANEL_BACKEND_URL_BASE change is detected instead of silently
// breaking every Connected App (ADR-0010).
func init() {
	m.Register(func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.Add(&core.TextField{Name: "oidc_issuer", Max: 2048})
		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("oidc_issuer")
		return app.Save(settings)
	})
}
```

Append to `backend/internal/oidc/oidc.go`:

```go
// Prompt values (OIDC Core 3.1.2.1) in the order they are stored.
const (
	PromptNone          = "none"
	PromptLogin         = "login"
	PromptConsent       = "consent"
	PromptSelectAccount = "select_account"
)

// PromptValues lists every supported prompt value in canonical order.
var PromptValues = []string{PromptNone, PromptLogin, PromptConsent, PromptSelectAccount}
```

In `backend/internal/oidc/clients.go`, extend the scope constants:

```go
const (
	ScopeOpenID        = "openid"
	ScopeEmail         = "email"
	ScopeOfflineAccess = "offline_access"

	// MaxRedirectURIs bounds the admin form and the exact-match lookup.
	MaxRedirectURIs      = 10
	maxRedirectURILength = 2048
)
```

`backend/internal/oidc/discovery.go`:

```go
package oidc

// Endpoint paths relative to the issuer.
const (
	DiscoveryPath  = "/.well-known/openid-configuration"
	AuthorizePath  = "/authorize"
	TokenPath      = "/token"
	UserInfoPath   = "/userinfo"
	JWKSPath       = "/jwks"
	RevocationPath = "/revoke"
)

var clientAuthMethods = []string{"client_secret_basic", "client_secret_post"}

// ProviderMetadata is the discovery document (OIDC Discovery 3 and the
// RFC 8414 fields the panel supports).
type ProviderMetadata struct {
	Issuer                                     string   `json:"issuer"`
	AuthorizationEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                              string   `json:"token_endpoint"`
	UserInfoEndpoint                           string   `json:"userinfo_endpoint"`
	JWKSURI                                    string   `json:"jwks_uri"`
	RevocationEndpoint                         string   `json:"revocation_endpoint"`
	ResponseTypesSupported                     []string `json:"response_types_supported"`
	ResponseModesSupported                     []string `json:"response_modes_supported"`
	GrantTypesSupported                        []string `json:"grant_types_supported"`
	SubjectTypesSupported                      []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported           []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                            []string `json:"scopes_supported"`
	ClaimsSupported                            []string `json:"claims_supported"`
	TokenEndpointAuthMethodsSupported          []string `json:"token_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported     []string `json:"revocation_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported              []string `json:"code_challenge_methods_supported"`
	PromptValuesSupported                      []string `json:"prompt_values_supported"`
	AuthorizationResponseISSParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
	RequestParameterSupported                  bool     `json:"request_parameter_supported"`
	RequestURIParameterSupported               bool     `json:"request_uri_parameter_supported"`
	ClaimsParameterSupported                   bool     `json:"claims_parameter_supported"`
}

// Discovery describes the provider published at issuer.
func Discovery(issuer string) ProviderMetadata {
	return ProviderMetadata{
		Issuer:                                 issuer,
		AuthorizationEndpoint:                  issuer + AuthorizePath,
		TokenEndpoint:                          issuer + TokenPath,
		UserInfoEndpoint:                       issuer + UserInfoPath,
		JWKSURI:                                issuer + JWKSPath,
		RevocationEndpoint:                     issuer + RevocationPath,
		ResponseTypesSupported:                 []string{"code"},
		ResponseModesSupported:                 []string{"query"},
		GrantTypesSupported:                    []string{"authorization_code", "refresh_token"},
		SubjectTypesSupported:                  []string{"public"},
		IDTokenSigningAlgValuesSupported:       []string{SigningAlgorithm},
		ScopesSupported:                        []string{ScopeOpenID, ScopeEmail, ScopeOfflineAccess},
		ClaimsSupported:                        []string{"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce", "at_hash", "email", "email_verified"},
		TokenEndpointAuthMethodsSupported:      clientAuthMethods,
		RevocationEndpointAuthMethodsSupported: clientAuthMethods,
		CodeChallengeMethodsSupported:          []string{"S256"},
		PromptValuesSupported:                  PromptValues,
		AuthorizationResponseISSParameterSupported: true,
	}
}
```

- [ ] **Step 4: Pin the issuer and gate the endpoints**

In `backend/internal/api/settings.go`, add `OIDCIssuer string` after `OIDCEnabled` in `settings`, read it in `settingsFromRecord` with `OIDCIssuer: rec.GetString("oidc_issuer"),`, and change the first line of `settingsResponse` to:

```go
	issuer, reason := h.oidcStatus(s)
```

Replace `backend/internal/api/oidc_settings.go` with:

```go
package api

import (
	"errors"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

const (
	oidcUnavailableBackendURLMissing  = "backend_url_missing"
	oidcUnavailableBackendURLInsecure = "backend_url_insecure"
	oidcUnavailableFrontendURLMissing = "frontend_url_missing"
	oidcUnavailableIssuerChanged      = "issuer_changed"
)

// oidcAvailability reports the issuer and why the provider cannot be enabled.
// The consent page is served by the frontend, so it needs a frontend origin.
func (h *Handlers) oidcAvailability() (issuer, reason string) {
	issuer, err := oidc.Issuer(h.publicConfig.APIURL)
	switch {
	case errors.Is(err, oidc.ErrBackendURLMissing):
		return "", oidcUnavailableBackendURLMissing
	case err != nil:
		return "", oidcUnavailableBackendURLInsecure
	case h.publicConfig.FrontendURL == "":
		return issuer, oidcUnavailableFrontendURLMissing
	}
	return issuer, ""
}

// oidcStatus adds the published issuer to oidcAvailability. Relying parties
// pin the issuer, so a provider whose issuer moved must stop answering.
func (h *Handlers) oidcStatus(s settings) (issuer, reason string) {
	issuer, reason = h.oidcAvailability()
	if reason == "" && s.OIDCEnabled && s.OIDCIssuer != "" && s.OIDCIssuer != issuer {
		return issuer, oidcUnavailableIssuerChanged
	}
	return issuer, reason
}

// oidcServingIssuer reports whether the provider endpoints exist right now.
// A switch set outside the panel has no pinned issuer yet, so the first
// served request pins the current one.
func (h *Handlers) oidcServingIssuer() (string, bool) {
	rec, err := h.settingsRecord()
	if err != nil || !rec.GetBool("oidc_enabled") {
		return "", false
	}
	issuer, reason := h.oidcStatus(settingsFromRecord(rec))
	if reason != "" {
		return "", false
	}
	if rec.GetString("oidc_issuer") == "" {
		rec.Set("oidc_issuer", issuer)
		if err := h.app.Save(rec); err != nil {
			log.Printf("[oidc] pin issuer: %v", err)
		}
	}
	return issuer, true
}

// applyOIDCEnabled validates and stores the provider switch. Enabling creates
// the signing key at once so JWKS is never empty while the provider is on,
// and pins the issuer it publishes. Repeating true is a no-op, so a client
// that resends the whole form is not rejected after the environment changes.
func (h *Handlers) applyOIDCEnabled(rec *core.Record, enabled bool) error {
	if !enabled {
		rec.Set("oidc_enabled", false)
		rec.Set("oidc_issuer", "")
		return nil
	}
	if rec.GetBool("oidc_enabled") {
		return nil
	}
	issuer, reason := h.oidcAvailability()
	if reason != "" {
		return apis.NewBadRequestError("oidc provider requires an https PANEL_BACKEND_URL_BASE and PANEL_FRONTEND_URL_BASE", nil)
	}
	if _, err := oidc.ActiveSigningKey(h.app, h.box, time.Now()); err != nil {
		return apis.NewInternalServerError("failed to prepare the oidc signing key", err)
	}
	rec.Set("oidc_enabled", true)
	rec.Set("oidc_issuer", issuer)
	return nil
}
```

`backend/internal/api/oidc_protocol.go`:

```go
package api

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"hysterical-panel/internal/oidc"
)

// oidcIssuerKey stores the serving issuer on the request event.
const oidcIssuerKey = "oidcIssuer"

// bindOIDCRoutes mounts the provider endpoints (ADR-0010). Relying parties
// call them, not the panel UI, so they stay out of openapi.go.
func (h *Handlers) bindOIDCRoutes(se *core.ServeEvent) {
	g := se.Router.Group(oidc.IssuerPath)
	g.Bind(h.requireOIDCServing())
	g.GET(oidc.DiscoveryPath, h.oidcDiscovery)
	g.GET(oidc.JWKSPath, h.oidcJWKS)
}

// requireOIDCServing hides the provider behind 404 while it is off or
// misconfigured, like the Management API.
func (h *Handlers) requireOIDCServing() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			issuer, ok := h.oidcServingIssuer()
			if !ok {
				return apis.NewNotFoundError("", nil)
			}
			e.Set(oidcIssuerKey, issuer)
			return e.Next()
		},
	}
}

func oidcIssuer(e *core.RequestEvent) string {
	issuer, _ := e.Get(oidcIssuerKey).(string)
	return issuer
}

func (h *Handlers) oidcDiscovery(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "public, max-age=300")
	return e.JSON(http.StatusOK, oidc.Discovery(oidcIssuer(e)))
}

// oidcJWKS creates the signing key first, so a switch set outside the panel
// never publishes an empty key set.
func (h *Handlers) oidcJWKS(e *core.RequestEvent) error {
	now := time.Now()
	if _, err := oidc.ActiveSigningKey(h.app, h.box, now); err != nil {
		return apis.NewInternalServerError("failed to prepare the oidc signing key", err)
	}
	jwks, err := oidc.PublicJWKS(h.app, now)
	if err != nil {
		return apis.NewInternalServerError("failed to load the oidc signing keys", err)
	}
	e.Response.Header().Set("Cache-Control", "public, max-age=300")
	return e.JSON(http.StatusOK, jwks)
}
```

In `backend/internal/api/api.go`, call the binder right before `return h` at the end of `Register`:

```go
	h.bindOIDCRoutes(se)
	return h
```

In `backend/internal/api/openapi.go`, add the new reason to the enum:

```go
		setEnum(s.Value.Properties, "oidc_unavailable_reason", []any{"", "backend_url_missing", "backend_url_insecure", "frontend_url_missing", "issuer_changed"})
```

- [ ] **Step 5: Run the backend tests**

Run: `cd backend && gofmt -l . ; go build ./... && go vet ./... && go test ./internal/oidc ./internal/api ./migrations`
Expected: no gofmt output, PASS.

- [ ] **Step 6: Describe `issuer_changed` in the settings page**

Add after `settings_oidc_unavailable_frontend_url_missing` in `frontend/messages/en.json`:

```json
  "settings_oidc_unavailable_issuer_changed": "PANEL_BACKEND_URL_BASE changed after the provider was turned on. Turn it off and on again to publish the new issuer, then update every connected app",
```

and in `frontend/messages/zh-cn.json`:

```json
  "settings_oidc_unavailable_issuer_changed": "开启提供方之后 PANEL_BACKEND_URL_BASE 发生了变化。关闭再开启即可发布新的 issuer，之后需要更新每个已连接的应用",
```

In `frontend/src/routes/settings/index.tsx`, add a case to `oidcUnavailableMessage` before `default`:

```tsx
    case "issuer_changed":
      return m.settings_oidc_unavailable_issuer_changed();
```

Run: `cd frontend && pnpm api:sync && pnpm build && pnpm typecheck && pnpm i18n:check`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add backend/migrations/1730000034_add_oidc_issuer.go backend/internal/oidc backend/internal/api frontend/messages frontend/src/routes/settings/index.tsx
git commit -m "feat(backend): serve oidc discovery and jwks" -m "Mount /api/oidc behind a gate that returns 404 unless the switch is on, the environment allows it and the pinned issuer still matches. Enabling pins the issuer; a moved PANEL_BACKEND_URL_BASE reports issuer_changed until an admin turns the switch off and on."
```

### Task 14: OAuth errors and client authentication

**Files:**
- Create: `backend/internal/oidc/errors.go`, `backend/internal/oidc/clientauth.go`, `backend/internal/oidc/clientauth_test.go`
- Modify: `backend/internal/oidc/testutil_test.go` (client fixture)
- Modify: `backend/internal/api/passkeys.go` (`blocked`), `backend/internal/api/oidc_protocol_test.go`

**Interfaces:**
- Consumes: `token.Sha256Hex`, `NewClientID`, `NewClientSecret`, `saveTestRecord`.
- Produces:
  - Error codes `oidc.InvalidRequest`, `InvalidClient`, `InvalidGrant`, `InvalidScope`, `UnsupportedGrantType`, `UnsupportedResponseType`, `AccessDenied`, `LoginRequired`, `ConsentRequired`, `RequestNotSupported`, `RequestURINotSupported`, `ServerError`, `TemporarilyUnavailable`.
  - `type oidc.Error struct { Code, Description string }` with `Error()` and `Unwrap()`, `oidc.NewError(code, description string) *Error`, `oidc.WrapServerError(cause error) *Error`.
  - `oidc.HasRepeatedParams(form url.Values) bool`.
  - `oidc.FindClient(app core.App, clientID string) (*core.Record, error)`.
  - `oidc.AuthenticateClient(app core.App, r *http.Request) (*core.Record, *Error)`, which needs `r.ParseForm()` to have run.
  - `(l *passkeyRateLimiter) blocked(key string, now time.Time) bool`.
  - Test fixtures: `testIssuer`, `testRedirectURI`, `newTestClient(t, app, fields map[string]any) (*core.Record, string)`.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/oidc/testutil_test.go` (add `"maps"` to the imports):

```go
const (
	testIssuer      = "https://api.example.com/api/oidc"
	testRedirectURI = "https://app.example.com/callback"
)

// newTestClient saves an enabled Connected App and returns it with its
// plaintext secret. fields override the defaults.
func newTestClient(t *testing.T, app core.App, fields map[string]any) (*core.Record, string) {
	t.Helper()
	clientID, err := NewClientID()
	if err != nil {
		t.Fatalf("client id: %v", err)
	}
	secret, secretHash, err := NewClientSecret()
	if err != nil {
		t.Fatalf("client secret: %v", err)
	}
	values := map[string]any{
		"client_id":      clientID,
		"name":           "Test app",
		"secret_hash":    secretHash,
		"redirect_uris":  []string{testRedirectURI},
		"allowed_scopes": []string{ScopeOpenID, ScopeEmail},
		"enabled":        true,
	}
	maps.Copy(values, fields)
	return saveTestRecord(t, app, ClientsCollection, values), secret
}
```

`backend/internal/oidc/clientauth_test.go`:

```go
package oidc

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// tokenHTTPRequest builds a parsed token endpoint request. Basic credentials
// are form-encoded first, as RFC 6749 2.3.1 requires.
func tokenHTTPRequest(t *testing.T, form url.Values, basicID, basicSecret string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/oidc/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basicID != "" {
		r.SetBasicAuth(url.QueryEscape(basicID), url.QueryEscape(basicSecret))
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	return r
}

func TestAuthenticateClientAcceptsBasicAndPost(t *testing.T) {
	app := newTestApp(t)
	client, secret := newTestClient(t, app, nil)
	id := client.GetString("client_id")
	for name, r := range map[string]*http.Request{
		"basic":                         tokenHTTPRequest(t, url.Values{}, id, secret),
		"basic with matching client_id": tokenHTTPRequest(t, url.Values{"client_id": {id}}, id, secret),
		"post":                          tokenHTTPRequest(t, url.Values{"client_id": {id}, "client_secret": {secret}}, "", ""),
	} {
		got, err := AuthenticateClient(app, r)
		if err != nil || got.Id != client.Id {
			t.Errorf("%s: client=%v err=%v", name, got, err)
		}
	}
}

func TestAuthenticateClientRejectsBadCredentials(t *testing.T) {
	app := newTestApp(t)
	client, secret := newTestClient(t, app, nil)
	disabled, disabledSecret := newTestClient(t, app, map[string]any{"enabled": false})
	id := client.GetString("client_id")
	cases := map[string]struct {
		r    *http.Request
		code string
	}{
		"no credentials":       {tokenHTTPRequest(t, url.Values{}, "", ""), InvalidClient},
		"client_id only":       {tokenHTTPRequest(t, url.Values{"client_id": {id}}, "", ""), InvalidClient},
		"wrong secret":         {tokenHTTPRequest(t, url.Values{}, id, secret+"x"), InvalidClient},
		"unknown client":       {tokenHTTPRequest(t, url.Values{}, "unknown", secret), InvalidClient},
		"disabled client":      {tokenHTTPRequest(t, url.Values{}, disabled.GetString("client_id"), disabledSecret), InvalidClient},
		"both methods":         {tokenHTTPRequest(t, url.Values{"client_id": {id}, "client_secret": {secret}}, id, secret), InvalidRequest},
		"mismatched client_id": {tokenHTTPRequest(t, url.Values{"client_id": {"other"}}, id, secret), InvalidRequest},
	}
	for name, c := range cases {
		if _, err := AuthenticateClient(app, c.r); err == nil || err.Code != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}
}

func TestHasRepeatedParams(t *testing.T) {
	if HasRepeatedParams(url.Values{"a": {"1"}, "b": {"2"}}) {
		t.Fatal("distinct parameters reported as repeated")
	}
	if !HasRepeatedParams(url.Values{"code": {"1", "2"}}) {
		t.Fatal("repeated code not reported")
	}
}
```

Append to `backend/internal/api/oidc_protocol_test.go` (add `"time"` to the imports):

```go
func TestRateLimiterBlockedPeeksWithoutCounting(t *testing.T) {
	limiter := newPasskeyRateLimiter(2, time.Minute)
	now := time.Now()
	for hits := 0; hits < 2; hits++ {
		if limiter.blocked("192.0.2.1", now) {
			t.Fatalf("blocked after %d hits", hits)
		}
		limiter.allow("192.0.2.1", now)
	}
	if !limiter.blocked("192.0.2.1", now) {
		t.Fatal("not blocked at the limit")
	}
	if limiter.blocked("192.0.2.1", now.Add(2*time.Minute)) {
		t.Fatal("still blocked after the window")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'AuthenticateClient|RepeatedParams'; go test ./internal/api -run RateLimiterBlocked`
Expected: FAIL to compile with `undefined: AuthenticateClient` and `limiter.blocked undefined`.

- [ ] **Step 3: Implement errors, client authentication and `blocked`**

`backend/internal/oidc/errors.go`:

```go
package oidc

import "net/url"

// OAuth 2.0 and OIDC error codes (RFC 6749 4.1.2.1 and 5.2, OIDC Core 3.1.2.6).
const (
	InvalidRequest          = "invalid_request"
	InvalidClient           = "invalid_client"
	InvalidGrant            = "invalid_grant"
	InvalidScope            = "invalid_scope"
	UnsupportedGrantType    = "unsupported_grant_type"
	UnsupportedResponseType = "unsupported_response_type"
	AccessDenied            = "access_denied"
	LoginRequired           = "login_required"
	ConsentRequired         = "consent_required"
	RequestNotSupported     = "request_not_supported"
	RequestURINotSupported  = "request_uri_not_supported"
	ServerError             = "server_error"
	TemporarilyUnavailable  = "temporarily_unavailable"
)

// Error is an OAuth error response. Description goes to the client, so it
// never carries internal details. cause is only for the server log.
type Error struct {
	Code        string
	Description string
	cause       error
}

func NewError(code, description string) *Error {
	return &Error{Code: code, Description: description}
}

// WrapServerError hides an internal failure behind server_error.
func WrapServerError(cause error) *Error {
	return &Error{Code: ServerError, cause: cause}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Description
}

func (e *Error) Unwrap() error { return e.cause }

// HasRepeatedParams reports whether any parameter appears more than once,
// which RFC 6749 3.1 and 3.2 forbid.
func HasRepeatedParams(form url.Values) bool {
	for _, values := range form {
		if len(values) > 1 {
			return true
		}
	}
	return false
}
```

`backend/internal/oidc/clientauth.go`:

```go
package oidc

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// unknownClientHash stands in when the client_id matches nothing, so a wrong
// client_id costs the same comparison as a wrong secret.
var unknownClientHash = strings.Repeat("0", 64)

// FindClient looks a Connected App up by its public client_id.
func FindClient(app core.App, clientID string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(ClientsCollection, "client_id = {:id}", dbx.Params{"id": clientID})
}

// AuthenticateClient checks client_secret_basic or client_secret_post on a
// request whose form is already parsed. A disabled app fails like a wrong
// secret.
func AuthenticateClient(app core.App, r *http.Request) (*core.Record, *Error) {
	clientID, secret, oauthErr := clientCredentials(r)
	if oauthErr != nil {
		return nil, oauthErr
	}
	client, err := FindClient(app, clientID)
	expected := unknownClientHash
	if err == nil {
		expected = client.GetString("secret_hash")
	}
	matches := subtle.ConstantTimeCompare([]byte(token.Sha256Hex(secret)), []byte(expected)) == 1
	if err != nil || !matches || !client.GetBool("enabled") {
		return nil, NewError(InvalidClient, "client authentication failed")
	}
	return client, nil
}

// clientCredentials reads the client_id and secret. Basic credentials are
// form-encoded before base64 (RFC 6749 2.3.1), and a request may use only
// one method (RFC 6749 2.3).
func clientCredentials(r *http.Request) (clientID, secret string, oauthErr *Error) {
	basicID, basicSecret, hasBasic := r.BasicAuth()
	postID, postSecret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	switch {
	case hasBasic && postSecret != "":
		return "", "", NewError(InvalidRequest, "use one client authentication method")
	case hasBasic:
		clientID, idErr := url.QueryUnescape(basicID)
		secret, secretErr := url.QueryUnescape(basicSecret)
		if idErr != nil || secretErr != nil {
			return "", "", NewError(InvalidClient, "client authentication failed")
		}
		if postID != "" && postID != clientID {
			return "", "", NewError(InvalidRequest, "client_id does not match the authenticated client")
		}
		return clientID, secret, nil
	case postID != "" && postSecret != "":
		return postID, postSecret, nil
	}
	return "", "", NewError(InvalidClient, "client authentication required")
}
```

Add to `backend/internal/api/passkeys.go` after `allow`:

```go
// blocked reports whether key has used up its window, without counting a hit.
func (l *passkeyRateLimiter) blocked(key string, now time.Time) bool {
	if l == nil {
		return false
	}
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	client, ok := l.clients[key]
	return ok && !now.After(client.reset) && client.count >= l.max
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test ./internal/oidc ./internal/api -run 'AuthenticateClient|RepeatedParams|RateLimiterBlocked'`
Expected: no gofmt output, PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/oidc backend/internal/api/passkeys.go backend/internal/api/oidc_protocol_test.go
git commit -m "feat(backend): authenticate oidc clients" -m "Accept client_secret_basic or client_secret_post, never both, and compare the secret hash in constant time even for unknown client ids. Add a peek to the rate limiter so failed authentications can lock out an address."
```

### Task 15: `/authorize`

**Files:**
- Create: `backend/internal/oidc/authorize.go`, `backend/internal/oidc/scopes.go`, `backend/internal/oidc/authorize_test.go`
- Modify: `backend/internal/oidc/clients.go` (`ClientRedirectURIs`, `ClientScopes`), `backend/internal/oidc/testutil_test.go` (PKCE pair, `authorizeForm`)
- Modify: `backend/internal/api/oidc_protocol.go`, `backend/internal/api/api.go` (`oidcLimits` field), `backend/internal/api/oidc_protocol_test.go`
- Modify: `docs/research/2026-10-12-oauth-provider.md` (the `select_account` row)

**Interfaces:**
- Consumes: `FindClient`, `NewError`, error codes, `PromptValues`, `token.New`, `token.Sha256Hex`, `AuthRequestTTL`, `oidcIssuer(e)`, the Task 13 test env.
- Produces:
  - `oidc.PageInvalidRequest`, `PageInvalidClient`, `PageInvalidRedirectURI`.
  - `type oidc.AuthorizeRequest struct { Client *core.Record; RedirectURI string; Scopes []string; State, Nonce, CodeChallenge string; Prompt []string; MaxAge int }`.
  - `type oidc.AuthorizeFailure struct { Page string; Err *Error; RedirectURI, State string }`.
  - `oidc.ParseAuthorizeRequest(app core.App, form url.Values) (*AuthorizeRequest, *AuthorizeFailure)`.
  - `oidc.SaveAuthRequest(app core.App, req *AuthorizeRequest, now time.Time) (string, error)`, which returns the handle.
  - `oidc.ErrorRedirect(redirectURI string, oauthErr *Error, state, issuer string) string`, and the unexported `codeRedirect(redirectURI, code, state, issuer string) string` and `appendQuery`.
  - `oidc.ClientRedirectURIs(client) []string`, `oidc.ClientScopes(client) []string`.
  - Unexported scope helpers: `orderScopes`, `unionScopes`, `isSubset`, `grantableScopes`.
  - API side: `oidcLimits` with `newOIDCLimits()`, `oidcAuthorizeRateMax`, `oidcBackchannelRateMax`, `oidcClientFailureMax`, `oidcPageRateLimited`, `(h) oidcErrorPage(e, reason)`.
  - Test fixtures: `testVerifier`, `testChallenge`, `authorizeForm(client, overrides)` in the oidc package, and `oidcTestVerifier`, `oidcTestChallenge`, `newOIDCTestEnv(t, clientBody)`, `(env) authorizeQuery(overrides)`, `(env) startAuthorization(overrides) string` in the api package.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/oidc/testutil_test.go` (add `"net/url"` to the imports):

```go
// testVerifier and testChallenge are the PKCE example of RFC 7636 appendix B.
const (
	testVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	testChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

// authorizeForm is a valid authorization request for client. An override
// replaces a value, and an empty override removes the parameter.
func authorizeForm(client *core.Record, overrides map[string]string) url.Values {
	form := url.Values{
		"client_id":             {client.GetString("client_id")},
		"redirect_uri":          {testRedirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {"state-1"},
		"nonce":                 {"nonce-1"},
		"code_challenge":        {testChallenge},
		"code_challenge_method": {"S256"},
	}
	for key, value := range overrides {
		if value == "" {
			form.Del(key)
		} else {
			form.Set(key, value)
		}
	}
	return form
}
```

`backend/internal/oidc/authorize_test.go`:

```go
package oidc

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"

	"hysterical-panel/internal/token"
)

func TestParseAuthorizeRequestAcceptsAValidRequest(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{
		"scope": "profile offline_access email openid", "prompt": "consent login", "max_age": "300", "ui_locales": "zh-CN",
	}))
	if failure != nil {
		t.Fatalf("failure = %+v", failure)
	}
	if req.Client.Id != client.Id || req.RedirectURI != testRedirectURI || req.State != "state-1" || req.Nonce != "nonce-1" ||
		req.CodeChallenge != testChallenge || req.MaxAge != 300 ||
		!slices.Equal(req.Scopes, []string{"openid", "email", "offline_access"}) ||
		!slices.Equal(req.Prompt, []string{"login", "consent"}) {
		t.Fatalf("request = %+v", req)
	}
}

type authorizePageCase struct {
	form url.Values
	page string
}

func TestParseAuthorizeRequestShowsAnErrorPageForUntrustedRedirects(t *testing.T) {
	app := newTestApp(t)
	const withQuery = "https://app.example.com/cb?tenant=1"
	client, _ := newTestClient(t, app, map[string]any{"redirect_uris": []string{testRedirectURI, withQuery}})
	disabled, _ := newTestClient(t, app, map[string]any{"enabled": false})
	repeated := authorizeForm(client, nil)
	repeated.Add("redirect_uri", testRedirectURI)
	cases := map[string]authorizePageCase{
		"missing client_id":     {authorizeForm(client, map[string]string{"client_id": ""}), PageInvalidClient},
		"unknown client":        {authorizeForm(client, map[string]string{"client_id": "unknown"}), PageInvalidClient},
		"disabled client":       {authorizeForm(disabled, nil), PageInvalidClient},
		"missing redirect_uri":  {authorizeForm(client, map[string]string{"redirect_uri": ""}), PageInvalidRedirectURI},
		"repeated redirect_uri": {repeated, PageInvalidRequest},
	}
	// RFC 9700 2.1: only a byte-for-byte match counts.
	for _, uri := range []string{
		testRedirectURI + "/",
		"https://APP.example.com/callback",
		"https://app.example.com/Callback",
		testRedirectURI + "?x=1",
		testRedirectURI + "#fragment",
		"https://user@app.example.com/callback",
		"https://app.example.com:443/callback",
		"http://app.example.com/callback",
		"https://app.example.com/cb?tenant=2",
	} {
		cases["redirect "+uri] = authorizePageCase{authorizeForm(client, map[string]string{"redirect_uri": uri}), PageInvalidRedirectURI}
	}
	for name, c := range cases {
		_, failure := ParseAuthorizeRequest(app, c.form)
		if failure == nil || failure.Err != nil || failure.Page != c.page {
			t.Errorf("%s: failure = %+v, want page %s", name, failure, c.page)
		}
	}
	if _, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{"redirect_uri": withQuery})); failure != nil {
		t.Fatalf("registered URI with a query rejected: %+v", failure)
	}
}

func TestParseAuthorizeRequestRedirectsOtherErrors(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	cases := map[string]struct {
		overrides map[string]string
		code      string
	}{
		"token response type":    {map[string]string{"response_type": "token"}, UnsupportedResponseType},
		"missing response type":  {map[string]string{"response_type": ""}, InvalidRequest},
		"fragment response mode": {map[string]string{"response_mode": "fragment"}, InvalidRequest},
		"no openid":              {map[string]string{"scope": "email"}, InvalidScope},
		"plain pkce":             {map[string]string{"code_challenge_method": "plain"}, InvalidRequest},
		"missing pkce method":    {map[string]string{"code_challenge_method": ""}, InvalidRequest},
		"missing challenge":      {map[string]string{"code_challenge": ""}, InvalidRequest},
		"short challenge":        {map[string]string{"code_challenge": "abc"}, InvalidRequest},
		"unknown prompt":         {map[string]string{"prompt": "sometimes"}, InvalidRequest},
		"none with login":        {map[string]string{"prompt": "none login"}, InvalidRequest},
		"negative max_age":       {map[string]string{"max_age": "-1"}, InvalidRequest},
		"text max_age":           {map[string]string{"max_age": "soon"}, InvalidRequest},
		"none with max_age=0":    {map[string]string{"prompt": "none", "max_age": "0"}, LoginRequired},
		"request object":         {map[string]string{"request": "eyJhbGciOiJub25lIn0"}, RequestNotSupported},
		"request_uri":            {map[string]string{"request_uri": "https://evil.example/request"}, RequestURINotSupported},
		"long nonce":             {map[string]string{"nonce": strings.Repeat("n", 1025)}, InvalidRequest},
	}
	for name, c := range cases {
		_, failure := ParseAuthorizeRequest(app, authorizeForm(client, c.overrides))
		if failure == nil || failure.Err == nil || failure.Err.Code != c.code || failure.RedirectURI != testRedirectURI || failure.State != "state-1" {
			t.Errorf("%s: failure = %+v, want %s back to the client with its state", name, failure, c.code)
		}
	}

	repeated := authorizeForm(client, nil)
	repeated.Add("code_challenge", testChallenge)
	if _, failure := ParseAuthorizeRequest(app, repeated); failure == nil || failure.Err == nil || failure.Err.Code != InvalidRequest {
		t.Errorf("repeated code_challenge: failure = %+v", failure)
	}
	_, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{"state": strings.Repeat("s", 1025), "response_type": "token"}))
	if failure == nil || failure.Err == nil || failure.State != "" {
		t.Errorf("an oversized state must not be echoed: failure = %+v", failure)
	}
}

func TestParseAuthorizeRequestNarrowsScopesAndTurnsMaxAgeZeroIntoLogin(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, map[string]any{"allowed_scopes": []string{ScopeOpenID}})
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{
		"scope": "openid email profile offline_access", "max_age": "0",
	}))
	if failure != nil {
		t.Fatalf("failure = %+v", failure)
	}
	if !slices.Equal(req.Scopes, []string{ScopeOpenID}) || !slices.Equal(req.Prompt, []string{PromptLogin}) || req.MaxAge != 0 {
		t.Fatalf("request = %+v", req)
	}
}

func TestSaveAuthRequestStoresOnlyTheHandleHash(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, map[string]string{"prompt": "login consent"}))
	if failure != nil {
		t.Fatalf("failure = %+v", failure)
	}
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	handle, err := SaveAuthRequest(app, req, now)
	if err != nil || len(handle) < 43 {
		t.Fatalf("handle %q, err %v", handle, err)
	}
	rec, err := app.FindFirstRecordByFilter(AuthRequestsCollection, "request_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(handle)})
	if err != nil {
		t.Fatalf("stored request: %v", err)
	}
	if rec.GetString("scope") != "openid email" || rec.GetString("prompt") != "login consent" ||
		rec.GetString("code_challenge") != testChallenge || !rec.GetDateTime("expires_at").Time().Equal(now.Add(AuthRequestTTL)) {
		t.Fatalf("stored request = %v", rec.PublicExport())
	}
}

func TestErrorRedirectKeepsTheRegisteredQuery(t *testing.T) {
	got := ErrorRedirect("https://app.example.com/cb?tenant=a%20b", NewError(AccessDenied, "denied"), "s 1", testIssuer)
	if !strings.HasPrefix(got, "https://app.example.com/cb?tenant=a%20b&") {
		t.Fatalf("redirect = %s", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("error") != AccessDenied || q.Get("error_description") != "denied" || q.Get("state") != "s 1" || q.Get("iss") != testIssuer {
		t.Fatalf("query = %v", q)
	}
}
```

Append to `backend/internal/api/oidc_protocol_test.go` (add `"maps"`, `"net/url"`, `"strings"` and `"hysterical-panel/internal/token"` to the imports):

```go
// oidcTestVerifier and oidcTestChallenge are the PKCE example of RFC 7636
// appendix B.
const (
	oidcTestVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	oidcTestChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

// newOIDCTestEnv switches the provider on and adds a Connected App for
// helpCenterCallback and a User signed in to the panel.
func newOIDCTestEnv(t *testing.T, clientBody map[string]any) *oidcTestEnv {
	t.Helper()
	env := newOIDCRouterEnv(t)
	env.setEnabled(true)
	body := map[string]any{"name": "Help center", "redirect_uris": []string{helpCenterCallback}}
	maps.Copy(body, clientBody)
	created := createTestOIDCClient(t, env.h, nil, body)
	env.client, env.secret = created.Client, created.ClientSecret
	env.user = newUsersTestRecord(t, env.app, "oidc-user@example.com", "OIDCUserSecret123")
	session, err := issueSessionToken(env.app, env.user)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	env.session = session
	return env
}

// authorizeQuery is a valid authorization request for the env's app. An
// override replaces a value, and an empty override removes the parameter.
func (env *oidcTestEnv) authorizeQuery(overrides map[string]string) url.Values {
	q := url.Values{
		"client_id":             {env.client.ClientID},
		"redirect_uri":          {helpCenterCallback},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {"rp-state"},
		"nonce":                 {"rp-nonce"},
		"code_challenge":        {oidcTestChallenge},
		"code_challenge_method": {"S256"},
	}
	for key, value := range overrides {
		if value == "" {
			q.Del(key)
		} else {
			q.Set(key, value)
		}
	}
	return q
}

// startAuthorization runs /authorize and returns the handle the consent page
// receives.
func (env *oidcTestEnv) startAuthorization(overrides map[string]string) string {
	env.t.Helper()
	res := env.get("/api/oidc/authorize?" + env.authorizeQuery(overrides).Encode())
	location, err := url.Parse(res.Header().Get("Location"))
	if res.Code != http.StatusSeeOther || err != nil || location.Path != "/oidc/authorize" {
		env.t.Fatalf("authorize = %d %q", res.Code, res.Header().Get("Location"))
	}
	return location.Query().Get("request")
}

func TestOIDCAuthorizeHandsTheBrowserToTheConsentPage(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	const consentPrefix = oidcTestFrontend + "/oidc/authorize?request="
	res := env.get("/api/oidc/authorize?" + env.authorizeQuery(nil).Encode())
	location := res.Header().Get("Location")
	if res.Code != http.StatusSeeOther || !strings.HasPrefix(location, consentPrefix) {
		t.Fatalf("GET = %d %q", res.Code, location)
	}
	stored, err := env.app.FindAllRecords(oidc.AuthRequestsCollection)
	if err != nil || len(stored) != 1 || stored[0].GetString("request_hash") != token.Sha256Hex(strings.TrimPrefix(location, consentPrefix)) {
		t.Fatalf("stored requests = %d, err %v", len(stored), err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/oidc/authorize", strings.NewReader(env.authorizeQuery(nil).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if res := env.do(req); res.Code != http.StatusSeeOther || !strings.HasPrefix(res.Header().Get("Location"), consentPrefix) {
		t.Fatalf("POST = %d %q", res.Code, res.Header().Get("Location"))
	}
}

func TestOIDCAuthorizeErrorsNeverRedirectToUnregisteredURIs(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	res := env.get("/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"redirect_uri": "https://evil.example/cb"}).Encode())
	if res.Code != http.StatusSeeOther || res.Header().Get("Location") != oidcTestFrontend+"/oidc/error?reason=invalid_redirect_uri" {
		t.Fatalf("bad redirect_uri = %d %q", res.Code, res.Header().Get("Location"))
	}

	res = env.get("/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"code_challenge_method": "plain"}).Encode())
	location, err := url.Parse(res.Header().Get("Location"))
	if err != nil || res.Code != http.StatusSeeOther || !strings.HasPrefix(location.String(), helpCenterCallback+"?") {
		t.Fatalf("plain pkce = %d %q", res.Code, res.Header().Get("Location"))
	}
	if q := location.Query(); q.Get("error") != oidc.InvalidRequest || q.Get("state") != "rp-state" || q.Get("iss") != oidcTestIssuer {
		t.Fatalf("error query = %v", q)
	}
}

func TestOIDCAuthorizeIsRateLimited(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	path := "/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"client_id": "unknown"}).Encode()
	for range oidcAuthorizeRateMax {
		env.get(path)
	}
	if res := env.get(path); res.Header().Get("Location") != oidcTestFrontend+"/oidc/error?reason=rate_limited" {
		t.Fatalf("over the limit = %d %q", res.Code, res.Header().Get("Location"))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'Authorize|ErrorRedirect|SaveAuthRequest'; go test ./internal/api -run OIDCAuthorize`
Expected: FAIL to compile with `undefined: ParseAuthorizeRequest`.

- [ ] **Step 3: Implement request validation and storage**

Add to `backend/internal/oidc/clients.go` (add `"github.com/pocketbase/pocketbase/core"` to the imports):

```go
// ClientRedirectURIs returns the registered redirect URIs of a Connected App.
func ClientRedirectURIs(client *core.Record) []string {
	return stringList(client, "redirect_uris")
}

// ClientScopes returns the scopes an admin allowed for a Connected App.
func ClientScopes(client *core.Record) []string {
	return stringList(client, "allowed_scopes")
}

func stringList(rec *core.Record, field string) []string {
	var out []string
	if err := rec.UnmarshalJSONField(field, &out); err != nil {
		return nil
	}
	return out
}
```

`backend/internal/oidc/scopes.go`:

```go
package oidc

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// knownScopes is the canonical order scopes are stored and returned in.
var knownScopes = []string{ScopeOpenID, ScopeEmail, ScopeOfflineAccess}

// orderScopes keeps the known scopes of values once each, in canonical order.
func orderScopes(values []string) []string {
	out := []string{}
	for _, scope := range knownScopes {
		if slices.Contains(values, scope) {
			out = append(out, scope)
		}
	}
	return out
}

func unionScopes(a, b []string) []string {
	return orderScopes(append(slices.Clone(a), b...))
}

func isSubset(subset, set []string) bool {
	for _, value := range subset {
		if !slices.Contains(set, value) {
			return false
		}
	}
	return true
}

// grantableScopes drops what the app may not receive. RFC 6749 3.3 lets the
// server grant less than requested, and most OIDC clients ask for profile
// by default.
func grantableScopes(client *core.Record, requested []string) []string {
	allowed := ClientScopes(client)
	if client.GetBool("refresh_enabled") {
		allowed = append(allowed, ScopeOfflineAccess)
	}
	out := []string{}
	for _, scope := range orderScopes(requested) {
		if slices.Contains(allowed, scope) {
			out = append(out, scope)
		}
	}
	return out
}
```

`backend/internal/oidc/authorize.go`:

```go
package oidc

import (
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// Error page reasons for requests whose redirect_uri cannot be trusted, so
// the error must not be sent there (RFC 6749 4.1.2.1).
const (
	PageInvalidRequest     = "invalid_request"
	PageInvalidClient      = "invalid_client"
	PageInvalidRedirectURI = "invalid_redirect_uri"
)

// maxParamLength matches the state, nonce and scope field limits.
const maxParamLength = 1024

var codeChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// authorizeParams may appear at most once. Others, such as ui_locales, are
// accepted and ignored (OIDC Core 15.1).
var authorizeParams = []string{
	"client_id", "redirect_uri", "response_type", "response_mode", "scope", "state",
	"nonce", "code_challenge", "code_challenge_method", "prompt", "max_age",
}

// AuthorizeRequest is a validated authorization request.
type AuthorizeRequest struct {
	Client        *core.Record
	RedirectURI   string
	Scopes        []string
	State         string
	Nonce         string
	CodeChallenge string
	Prompt        []string
	// MaxAge is 0 when absent. max_age=0 is stored as prompt=login.
	MaxAge int
}

// AuthorizeFailure is an error page reason when the client or redirect_uri
// cannot be trusted, and otherwise an OAuth error for RedirectURI.
type AuthorizeFailure struct {
	Page        string
	Err         *Error
	RedirectURI string
	State       string
}

// ParseAuthorizeRequest validates an authorization request. The client and
// redirect_uri come first because every later error is sent to that URI.
func ParseAuthorizeRequest(app core.App, form url.Values) (*AuthorizeRequest, *AuthorizeFailure) {
	if len(form["client_id"]) > 1 || len(form["redirect_uri"]) > 1 {
		return nil, &AuthorizeFailure{Page: PageInvalidRequest}
	}
	client, err := FindClient(app, form.Get("client_id"))
	if err != nil || !client.GetBool("enabled") {
		return nil, &AuthorizeFailure{Page: PageInvalidClient}
	}
	req := &AuthorizeRequest{Client: client, RedirectURI: form.Get("redirect_uri")}
	if !slices.Contains(ClientRedirectURIs(client), req.RedirectURI) {
		return nil, &AuthorizeFailure{Page: PageInvalidRedirectURI}
	}
	if oauthErr := req.parse(form); oauthErr != nil {
		return nil, &AuthorizeFailure{Err: oauthErr, RedirectURI: req.RedirectURI, State: req.State}
	}
	return req, nil
}

func (r *AuthorizeRequest) parse(form url.Values) *Error {
	steps := []func(url.Values) *Error{r.parseState, checkRequestShape, r.parseScope, r.parsePKCE, r.parseNonce, r.parsePrompt}
	for _, step := range steps {
		if oauthErr := step(form); oauthErr != nil {
			return oauthErr
		}
	}
	return nil
}

// parseState runs first so later errors can echo the state back.
func (r *AuthorizeRequest) parseState(form url.Values) *Error {
	state := form.Get("state")
	if len(form["state"]) > 1 || len(state) > maxParamLength {
		return NewError(InvalidRequest, "state must appear once and stay within 1024 characters")
	}
	r.State = state
	return nil
}

func checkRequestShape(form url.Values) *Error {
	for _, name := range authorizeParams {
		if len(form[name]) > 1 {
			return NewError(InvalidRequest, name+" must not repeat")
		}
	}
	responseType := form.Get("response_type")
	switch {
	case form.Has("request"):
		return NewError(RequestNotSupported, "request objects are not supported")
	case form.Has("request_uri"):
		return NewError(RequestURINotSupported, "request_uri is not supported")
	case responseType == "":
		return NewError(InvalidRequest, "response_type is required")
	case responseType != "code":
		return NewError(UnsupportedResponseType, "only response_type=code is supported")
	case form.Has("response_mode") && form.Get("response_mode") != "query":
		return NewError(InvalidRequest, "only response_mode=query is supported")
	}
	return nil
}

func (r *AuthorizeRequest) parseScope(form url.Values) *Error {
	raw := form.Get("scope")
	if len(raw) > maxParamLength {
		return NewError(InvalidRequest, "scope is too long")
	}
	requested := strings.Fields(raw)
	if !slices.Contains(requested, ScopeOpenID) {
		return NewError(InvalidScope, "scope must include openid")
	}
	r.Scopes = grantableScopes(r.Client, requested)
	return nil
}

func (r *AuthorizeRequest) parsePKCE(form url.Values) *Error {
	if form.Get("code_challenge_method") != "S256" {
		return NewError(InvalidRequest, "code_challenge_method must be S256")
	}
	challenge := form.Get("code_challenge")
	if !codeChallengePattern.MatchString(challenge) {
		return NewError(InvalidRequest, "code_challenge must be a base64url SHA-256 value")
	}
	r.CodeChallenge = challenge
	return nil
}

func (r *AuthorizeRequest) parseNonce(form url.Values) *Error {
	nonce := form.Get("nonce")
	if len(nonce) > maxParamLength {
		return NewError(InvalidRequest, "nonce is too long")
	}
	r.Nonce = nonce
	return nil
}

// parsePrompt reads prompt and max_age together, because max_age=0 demands
// a fresh sign-in and is stored as prompt=login.
func (r *AuthorizeRequest) parsePrompt(form url.Values) *Error {
	values := strings.Fields(form.Get("prompt"))
	for _, value := range values {
		if !slices.Contains(PromptValues, value) {
			return NewError(InvalidRequest, "unsupported prompt value")
		}
	}
	none := slices.Contains(values, PromptNone)
	if none && len(orderPrompt(values)) > 1 {
		return NewError(InvalidRequest, "prompt=none cannot be combined with other values")
	}
	maxAge, present, oauthErr := parseMaxAge(form.Get("max_age"))
	if oauthErr != nil {
		return oauthErr
	}
	if present && maxAge == 0 {
		if none {
			return NewError(LoginRequired, "max_age=0 needs an interactive sign-in")
		}
		values = append(values, PromptLogin)
	}
	r.Prompt, r.MaxAge = orderPrompt(values), maxAge
	return nil
}

func orderPrompt(values []string) []string {
	out := []string{}
	for _, value := range PromptValues {
		if slices.Contains(values, value) {
			out = append(out, value)
		}
	}
	return out
}

func parseMaxAge(raw string) (seconds int, present bool, oauthErr *Error) {
	if raw == "" {
		return 0, false, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 || seconds > math.MaxInt32 {
		return 0, false, NewError(InvalidRequest, "max_age must be a non-negative integer")
	}
	return seconds, true, nil
}

// SaveAuthRequest stores a validated request for the consent page and
// returns its handle. Only the handle's hash is stored.
func SaveAuthRequest(app core.App, req *AuthorizeRequest, now time.Time) (string, error) {
	handle, err := token.New(32)
	if err != nil {
		return "", err
	}
	collection, err := app.FindCollectionByNameOrId(AuthRequestsCollection)
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(collection)
	rec.Set("request_hash", token.Sha256Hex(handle))
	rec.Set("client", req.Client.Id)
	rec.Set("redirect_uri", req.RedirectURI)
	rec.Set("scope", strings.Join(req.Scopes, " "))
	rec.Set("state", req.State)
	rec.Set("nonce", req.Nonce)
	rec.Set("code_challenge", req.CodeChallenge)
	rec.Set("prompt", strings.Join(req.Prompt, " "))
	rec.Set("max_age", req.MaxAge)
	rec.Set("expires_at", now.UTC().Add(AuthRequestTTL))
	return handle, app.Save(rec)
}

// ErrorRedirect sends an OAuth error to a registered redirect_uri, with the
// RFC 9207 iss parameter.
func ErrorRedirect(redirectURI string, oauthErr *Error, state, issuer string) string {
	params := url.Values{"error": {oauthErr.Code}, "iss": {issuer}}
	if oauthErr.Description != "" {
		params.Set("error_description", oauthErr.Description)
	}
	if state != "" {
		params.Set("state", state)
	}
	return appendQuery(redirectURI, params)
}

func codeRedirect(redirectURI, code, state, issuer string) string {
	params := url.Values{"code": {code}, "iss": {issuer}}
	if state != "" {
		params.Set("state", state)
	}
	return appendQuery(redirectURI, params)
}

// appendQuery leaves the registered query untouched, because redirect_uri is
// compared byte for byte.
func appendQuery(uri string, params url.Values) string {
	if strings.Contains(uri, "?") {
		return uri + "&" + params.Encode()
	}
	return uri + "?" + params.Encode()
}
```

- [ ] **Step 4: Add the handler and rate limits**

In `backend/internal/api/api.go`, add `oidcLimits    oidcLimits` to `Handlers` and `oidcLimits:    newOIDCLimits(),` to the struct literal in `Register`.

In `backend/internal/api/oidc_protocol.go`, add `"log"` and `"net/url"` to the imports, register the routes in `bindOIDCRoutes`:

```go
	g.GET(oidc.AuthorizePath, h.oidcAuthorize)
	g.POST(oidc.AuthorizePath, h.oidcAuthorize)
```

and append:

```go
// Per-IP limits. A relying party backend sends every user's token requests
// from one address, so the back-channel limit is generous and failed client
// authentication has a much smaller budget of its own.
const (
	oidcAuthorizeRateMax   = 60
	oidcBackchannelRateMax = 300
	oidcClientFailureMax   = 10
	oidcRateWindow         = time.Minute

	oidcPageRateLimited = "rate_limited"
)

type oidcLimits struct {
	authorize      *passkeyRateLimiter
	backchannel    *passkeyRateLimiter
	clientFailures *passkeyRateLimiter
}

func newOIDCLimits() oidcLimits {
	return oidcLimits{
		authorize:      newPasskeyRateLimiter(oidcAuthorizeRateMax, oidcRateWindow),
		backchannel:    newPasskeyRateLimiter(oidcBackchannelRateMax, oidcRateWindow),
		clientFailures: newPasskeyRateLimiter(oidcClientFailureMax, oidcRateWindow),
	}
}

// oidcAuthorize validates the request, stores it and hands the browser to the
// panel's consent page. Every redirect is a 303 (RFC 9700 4.12).
func (h *Handlers) oidcAuthorize(e *core.RequestEvent) error {
	now := time.Now()
	if !h.oidcLimits.authorize.allow(e.RealIP(), now) {
		return h.oidcErrorPage(e, oidcPageRateLimited)
	}
	if err := e.Request.ParseForm(); err != nil {
		return h.oidcErrorPage(e, oidc.PageInvalidRequest)
	}
	req, failure := oidc.ParseAuthorizeRequest(h.app, e.Request.Form)
	if failure != nil {
		return h.oidcAuthorizeFailure(e, failure)
	}
	handle, err := oidc.SaveAuthRequest(h.app, req, now)
	if err != nil {
		log.Printf("[oidc] save authorization request: %v", err)
		return h.oidcAuthorizeFailure(e, &oidc.AuthorizeFailure{
			Err: oidc.NewError(oidc.ServerError, ""), RedirectURI: req.RedirectURI, State: req.State,
		})
	}
	return e.Redirect(http.StatusSeeOther, h.publicConfig.FrontendURL+"/oidc/authorize?request="+url.QueryEscape(handle))
}

func (h *Handlers) oidcAuthorizeFailure(e *core.RequestEvent, failure *oidc.AuthorizeFailure) error {
	if failure.Err == nil {
		return h.oidcErrorPage(e, failure.Page)
	}
	return e.Redirect(http.StatusSeeOther, oidc.ErrorRedirect(failure.RedirectURI, failure.Err, failure.State, oidcIssuer(e)))
}

func (h *Handlers) oidcErrorPage(e *core.RequestEvent, reason string) error {
	return e.Redirect(http.StatusSeeOther, h.publicConfig.FrontendURL+"/oidc/error?reason="+url.QueryEscape(reason))
}
```

- [ ] **Step 5: Record the `select_account` ruling in the research report**

In `docs/research/2026-10-12-oauth-provider.md`, replace the row

```
| `prompt=select_account` | 不支持，返回 `account_selection_required`。同意页一直显示当前账号 email 与「切换账号」链接，用户可以手动切换 |
```

with

```
| `prompt=select_account` | 与 `prompt=consent` 相同，总是显示同意页。同意页一直显示当前账号 email 与「切换账号」链接，用户可以在这里换账号。总是带这个值的 RP 也能正常登录 |
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS. If `TestParseAuthorizeRequestAcceptsAValidRequest` fails on the challenge, check the RFC 7636 appendix B values before touching the pattern.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/oidc backend/internal/api docs/research/2026-10-12-oauth-provider.md
git commit -m "feat(backend): validate oidc authorization requests" -m "Check the client and exact redirect_uri first and send failures to the panel error page, then require response_type=code and S256 PKCE and redirect every other error back with state and iss. Valid requests are stored by handle hash and the browser goes to the consent page with a 303."
```

### Task 16: Consent endpoints

**Files:**
- Create: `backend/internal/oidc/grants.go`, `backend/internal/oidc/consent.go`, `backend/internal/oidc/consent_test.go`
- Modify: `backend/internal/oidc/testutil_test.go` (`testNow`, `newPendingHandle`, `redirectQuery`, `signedInAt`)
- Create: `backend/internal/api/oidc_consent.go`, `backend/internal/api/oidc_consent_test.go`
- Modify: `backend/internal/api/api.go` (routes), `backend/internal/api/dto.go`, `backend/internal/api/openapi.go`, `backend/internal/api/openapi_oidc_test.go`

**Interfaces:**
- Consumes: `FindClient`, `ClientRedirectURIs`, `ParseAuthorizeRequest`, `SaveAuthRequest`, `ErrorRedirect`, `codeRedirect`, scope helpers, `oidc.SessionAuthTime`, `bearerToken`, `oidcIssuer(e)`, `h.requireOIDCServing()`, `h.oidcLimits.authorize`.
- Produces:
  - `oidc.ErrRequestNotFound`, `ErrReauthenticationRequired`, `ErrUserNotAllowed`.
  - `type oidc.Session struct { AuthTime time.Time; Known bool }`.
  - `type oidc.PendingRequest` with field `Client *core.Record` and methods `Scopes() []string`, `RedirectURI() string`, `ExpiresAt() time.Time`, `PromptNone() bool`, `NeedsReauthentication(s Session, now time.Time) bool`, `NeedsConsent(app core.App, userID string) (bool, error)`.
  - `oidc.FindPendingRequest(app, handle string, now) (*PendingRequest, error)`.
  - `oidc.Approve(app, handle string, user *core.Record, s Session, issuer string, now) (string, error)`, `oidc.Deny(app, handle, issuer string, now) (string, error)`, `oidc.Abort(app, handle, issuer string, now) (string, error)`.
  - `oidc.UserMayUse(client, user *core.Record) bool`.
  - Unexported: `findActiveGrant`, `upsertGrant`, `grantScopes`, `claimOnce(app, collection, id, field string, now) (bool, error)`, `dbTime(time.Time) string`.
  - DTOs `OIDCAuthorizationRequest` and `OIDCAuthorizationDecision`. Routes:
    - `GET /api/panel/oidc/requests/{handle}`
    - `POST /api/panel/oidc/requests/{handle}/approve`
    - `POST /api/panel/oidc/requests/{handle}/deny`
    - `POST /api/panel/oidc/requests/{handle}/abort` (public)
  - Test helpers `(env) panelGet(path)` and `(env) panelPost(path)`, plus `(env) approveCode(overrides) string`, which returns the code from an approved request.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/oidc/testutil_test.go` (add `"time"` to the imports):

```go
var testNow = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

// signedInAt is a panel session whose sign-in time is known.
func signedInAt(at time.Time) Session {
	return Session{AuthTime: at, Known: true}
}

// newPendingHandle stores a valid authorization request and returns its handle.
func newPendingHandle(t *testing.T, app core.App, client *core.Record, overrides map[string]string, now time.Time) string {
	t.Helper()
	req, failure := ParseAuthorizeRequest(app, authorizeForm(client, overrides))
	if failure != nil {
		t.Fatalf("authorize: %+v", failure)
	}
	handle, err := SaveAuthRequest(app, req, now)
	if err != nil {
		t.Fatalf("save request: %v", err)
	}
	return handle
}

// redirectQuery parses the query of a redirect back to the app.
func redirectQuery(t *testing.T, redirect string) url.Values {
	t.Helper()
	u, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", redirect, err)
	}
	return u.Query()
}
```

`backend/internal/oidc/consent_test.go`:

```go
package oidc

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/dbx"

	"hysterical-panel/internal/token"
)

func TestUserMayUse(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	disabledClient, _ := newTestClient(t, app, map[string]any{"enabled": false})
	user := newTestUser(t, app, "may-use@example.com")
	if !UserMayUse(client, user) {
		t.Fatal("active verified user rejected")
	}
	if UserMayUse(disabledClient, user) || UserMayUse(nil, user) || UserMayUse(client, nil) {
		t.Fatal("disabled or missing app accepted")
	}
	for field, value := range map[string]any{"status": "disabled", "verified": false} {
		changed := user.Clone()
		changed.Set(field, value)
		if UserMayUse(client, changed) {
			t.Errorf("user with %s=%v accepted", field, value)
		}
	}
}

func TestApproveIssuesASingleUseCode(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "approve@example.com")
	handle := newPendingHandle(t, app, client, nil, testNow)

	redirect, err := Approve(app, handle, user, signedInAt(testNow.Add(-time.Minute)), testIssuer, testNow)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	q := redirectQuery(t, redirect)
	if q.Get("code") == "" || q.Get("state") != "state-1" || q.Get("iss") != testIssuer {
		t.Fatalf("redirect = %s", redirect)
	}
	code, err := app.FindFirstRecordByFilter(CodesCollection, "code_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(q.Get("code"))})
	if err != nil {
		t.Fatalf("stored code: %v", err)
	}
	if code.GetString("nonce") != "nonce-1" || code.GetString("scope") != "openid email" || code.GetString("family") == "" ||
		!code.GetDateTime("expires_at").Time().Equal(testNow.Add(CodeTTL)) ||
		!code.GetDateTime("auth_time").Time().Equal(testNow.Add(-time.Minute)) {
		t.Fatalf("stored code = %v", code.PublicExport())
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("second approve: %v, want ErrRequestNotFound", err)
	}
}

func TestConsentIsRememberedUntilScopesGrow(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "remember@example.com")
	approve := func(scope string) {
		t.Helper()
		handle := newPendingHandle(t, app, client, map[string]string{"scope": scope}, testNow)
		if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); err != nil {
			t.Fatalf("approve %q: %v", scope, err)
		}
	}
	approve("openid")

	cases := map[string]struct {
		overrides map[string]string
		want      bool
	}{
		"same scope":            {map[string]string{"scope": "openid"}, false},
		"wider scope":           {map[string]string{"scope": "openid email"}, true},
		"prompt=consent":        {map[string]string{"scope": "openid", "prompt": "consent"}, true},
		"prompt=select_account": {map[string]string{"scope": "openid", "prompt": "select_account"}, true},
	}
	for name, c := range cases {
		p, err := FindPendingRequest(app, newPendingHandle(t, app, client, c.overrides, testNow), testNow)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if needs, err := p.NeedsConsent(app, user.Id); err != nil || needs != c.want {
			t.Errorf("%s: NeedsConsent = %v, %v; want %v", name, needs, err, c.want)
		}
	}

	approve("openid email")
	grants, err := app.FindAllRecords(GrantsCollection)
	if err != nil || len(grants) != 1 || !slices.Equal(grantScopes(grants[0]), []string{"openid", "email"}) {
		t.Fatalf("grants = %d, err %v", len(grants), err)
	}

	skipping, _ := newTestClient(t, app, map[string]any{"skip_consent": true})
	p, err := FindPendingRequest(app, newPendingHandle(t, app, skipping, nil, testNow), testNow)
	if err != nil {
		t.Fatalf("skip_consent request: %v", err)
	}
	if needs, _ := p.NeedsConsent(app, user.Id); needs {
		t.Fatal("an app marked skip_consent asked for consent")
	}
}

func TestNeedsReauthentication(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	cases := []struct {
		name      string
		overrides map[string]string
		session   Session
		want      bool
	}{
		{"no prompt, unknown session", nil, Session{}, false},
		{"login, signed in before the request", map[string]string{"prompt": "login"}, signedInAt(testNow.Add(-time.Second)), true},
		{"login, signed in after the request", map[string]string{"prompt": "login"}, signedInAt(testNow.Add(time.Second)), false},
		{"login, unknown session", map[string]string{"prompt": "login"}, Session{}, true},
		{"max_age, recent sign-in", map[string]string{"max_age": "300"}, signedInAt(testNow.Add(-299 * time.Second)), false},
		{"max_age, old sign-in", map[string]string{"max_age": "300"}, signedInAt(testNow.Add(-301 * time.Second)), true},
		{"max_age, unknown session", map[string]string{"max_age": "300"}, Session{}, true},
	}
	for _, c := range cases {
		p, err := FindPendingRequest(app, newPendingHandle(t, app, client, c.overrides, testNow), testNow)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := p.NeedsReauthentication(c.session, testNow); got != c.want {
			t.Errorf("%s: NeedsReauthentication = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestApproveHandlesStaleAndSilentRequests(t *testing.T) {
	app := newTestApp(t)
	remembered, _ := newTestClient(t, app, nil)
	unknown, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "silent@example.com")

	stale := newPendingHandle(t, app, remembered, map[string]string{"prompt": "login"}, testNow)
	if _, err := Approve(app, stale, user, signedInAt(testNow.Add(-time.Hour)), testIssuer, testNow); !errors.Is(err, ErrReauthenticationRequired) {
		t.Fatalf("stale session: %v, want ErrReauthenticationRequired", err)
	}
	if _, err := Approve(app, stale, user, signedInAt(testNow.Add(time.Second)), testIssuer, testNow.Add(2*time.Second)); err != nil {
		t.Fatalf("after signing in again: %v", err)
	}

	silent := newPendingHandle(t, app, remembered, map[string]string{"prompt": "none"}, testNow)
	redirect, err := Approve(app, silent, user, Session{}, testIssuer, testNow)
	if err != nil || redirectQuery(t, redirect).Get("code") == "" {
		t.Fatalf("silent request with remembered consent = %q, %v", redirect, err)
	}

	for overrides, want := range map[string]string{"": ConsentRequired, "60": LoginRequired} {
		handle := newPendingHandle(t, app, unknown, map[string]string{"prompt": "none", "max_age": overrides}, testNow)
		redirect, err := Approve(app, handle, user, Session{}, testIssuer, testNow)
		if err != nil || redirectQuery(t, redirect).Get("error") != want {
			t.Errorf("max_age=%q: redirect %q, err %v; want %s", overrides, redirect, err, want)
		}
		if _, err := FindPendingRequest(app, handle, testNow); !errors.Is(err, ErrRequestNotFound) {
			t.Errorf("max_age=%q: a silent failure left the request pending", overrides)
		}
	}
}

func TestApproveRejectsUnusableUsersAppsAndExpiredRequests(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "blocked@example.com")
	handle := newPendingHandle(t, app, client, nil, testNow)

	disabledUser := user.Clone()
	disabledUser.Set("status", "disabled")
	if _, err := Approve(app, handle, disabledUser, signedInAt(testNow), testIssuer, testNow); !errors.Is(err, ErrUserNotAllowed) {
		t.Fatalf("disabled user: %v", err)
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow.Add(AuthRequestTTL)); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("expired request: %v", err)
	}
	client.Set("enabled", false)
	if err := app.Save(client); err != nil {
		t.Fatalf("disable client: %v", err)
	}
	if _, err := Approve(app, handle, user, signedInAt(testNow), testIssuer, testNow); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("disabled app: %v", err)
	}
}

func TestDenyAndAbortEndTheRequest(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)

	handle := newPendingHandle(t, app, client, nil, testNow)
	redirect, err := Deny(app, handle, testIssuer, testNow)
	if q := redirectQuery(t, redirect); err != nil || q.Get("error") != AccessDenied || q.Get("state") != "state-1" || q.Get("iss") != testIssuer {
		t.Fatalf("deny = %q, %v", redirect, err)
	}
	if _, err := Deny(app, handle, testIssuer, testNow); !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("second deny: %v", err)
	}

	for prompt, want := range map[string]string{"none": LoginRequired, "": AccessDenied} {
		handle := newPendingHandle(t, app, client, map[string]string{"prompt": prompt}, testNow)
		redirect, err := Abort(app, handle, testIssuer, testNow)
		if err != nil || redirectQuery(t, redirect).Get("error") != want {
			t.Errorf("abort with prompt=%q = %q, %v; want %s", prompt, redirect, err, want)
		}
	}
}
```

`backend/internal/api/oidc_consent_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func (env *oidcTestEnv) panelRequest(method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", env.session)
	return env.do(req)
}

func (env *oidcTestEnv) panelGet(path string) *httptest.ResponseRecorder {
	return env.panelRequest(http.MethodGet, path)
}

func (env *oidcTestEnv) panelPost(path string) *httptest.ResponseRecorder {
	return env.panelRequest(http.MethodPost, path)
}

// approveCode runs /authorize and approves it as the env's User, returning
// the code sent back to the app.
func (env *oidcTestEnv) approveCode(overrides map[string]string) string {
	env.t.Helper()
	handle := env.startAuthorization(overrides)
	res := env.panelPost("/api/panel/oidc/requests/" + handle + "/approve")
	decision := decodeBody[OIDCAuthorizationDecision](env.t, res)
	location, err := url.Parse(decision.RedirectTo)
	if res.Code != http.StatusOK || err != nil || location.Query().Get("code") == "" {
		env.t.Fatalf("approve = %d %s", res.Code, res.Body.String())
	}
	return location.Query().Get("code")
}

func TestOIDCConsentShowsTheRequestAndApprovesIt(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	res := env.panelGet("/api/panel/oidc/requests/" + handle)
	view := decodeBody[OIDCAuthorizationRequest](t, res)
	if res.Code != http.StatusOK || view.ClientName != "Help center" || view.RedirectOrigin != "https://help.example" ||
		!slices.Equal(view.Scopes, []string{"openid", "email"}) || !view.ConsentRequired || view.ReauthenticationRequired || view.PromptNone {
		t.Fatalf("view = %d %+v", res.Code, view)
	}

	res = env.panelPost("/api/panel/oidc/requests/" + handle + "/approve")
	decision := decodeBody[OIDCAuthorizationDecision](t, res)
	location, err := url.Parse(decision.RedirectTo)
	if res.Code != http.StatusOK || err != nil || !strings.HasPrefix(decision.RedirectTo, helpCenterCallback+"?") {
		t.Fatalf("approve = %d %s", res.Code, res.Body.String())
	}
	if q := location.Query(); q.Get("code") == "" || q.Get("state") != "rp-state" || q.Get("iss") != oidcTestIssuer {
		t.Fatalf("redirect query = %v", q)
	}
	if res := env.panelPost("/api/panel/oidc/requests/" + handle + "/approve"); res.Code != http.StatusNotFound {
		t.Fatalf("second approve = %d", res.Code)
	}

	next := env.startAuthorization(nil)
	if view := decodeBody[OIDCAuthorizationRequest](t, env.panelGet("/api/panel/oidc/requests/"+next)); view.ConsentRequired {
		t.Fatal("remembered consent was asked again")
	}
}

func TestOIDCConsentNeedsAUsablePanelSession(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	if res := env.get("/api/panel/oidc/requests/" + handle); res.Code != http.StatusUnauthorized {
		t.Fatalf("without a session = %d", res.Code)
	}
	env.user.Set("verified", false)
	if err := env.app.Save(env.user); err != nil {
		t.Fatalf("unverify: %v", err)
	}
	if res := env.panelGet("/api/panel/oidc/requests/" + handle); res.Code != http.StatusForbidden {
		t.Fatalf("unverified user = %d", res.Code)
	}
}

func TestOIDCConsentDenyAndAbort(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	decision := decodeBody[OIDCAuthorizationDecision](t, env.panelPost("/api/panel/oidc/requests/"+handle+"/deny"))
	if location, err := url.Parse(decision.RedirectTo); err != nil || location.Query().Get("error") != "access_denied" {
		t.Fatalf("deny = %+v", decision)
	}

	silent := env.startAuthorization(map[string]string{"prompt": "none"})
	res := env.do(httptest.NewRequest(http.MethodPost, "/api/panel/oidc/requests/"+silent+"/abort", nil))
	decision = decodeBody[OIDCAuthorizationDecision](t, res)
	if location, err := url.Parse(decision.RedirectTo); res.Code != http.StatusOK || err != nil || location.Query().Get("error") != "login_required" {
		t.Fatalf("abort = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCConsentReturns404WhileTheProviderIsOff(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	handle := env.startAuthorization(nil)
	env.setEnabled(false)
	if res := env.panelGet("/api/panel/oidc/requests/" + handle); res.Code != http.StatusNotFound {
		t.Fatalf("view while off = %d", res.Code)
	}
	if res := env.panelPost("/api/panel/oidc/requests/" + handle + "/approve"); res.Code != http.StatusNotFound {
		t.Fatalf("approve while off = %d", res.Code)
	}
}
```

In `backend/internal/api/openapi_oidc_test.go`, add the consent paths to the map in `TestOpenAPIDescribesConnectedApps`:

```go
		"/api/panel/oidc/requests/{handle}":         {"GET"},
		"/api/panel/oidc/requests/{handle}/approve": {"POST"},
		"/api/panel/oidc/requests/{handle}/deny":    {"POST"},
		"/api/panel/oidc/requests/{handle}/abort":   {"POST"},
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'UserMayUse|Approve|Consent|Reauthentication|Deny'; go test ./internal/api -run 'OIDCConsent|OpenAPIDescribesConnectedApps'`
Expected: FAIL to compile with `undefined: Approve` and `undefined: OIDCAuthorizationRequest`.

- [ ] **Step 3: Implement grants and the consent decisions**

`backend/internal/oidc/grants.go`:

```go
package oidc

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// UserMayUse is the one place that decides who may sign in to a Connected
// App (ADR-0010). Every active, verified User may use every enabled app.
// Per-app restrictions belong here.
func UserMayUse(client, user *core.Record) bool {
	return client != nil && user != nil && client.GetBool("enabled") &&
		user.GetString("status") == "active" && user.GetBool("verified")
}

func findActiveGrant(app core.App, userID, clientID string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(GrantsCollection,
		"user = {:user} && client = {:client} && revoked_at = ''",
		dbx.Params{"user": userID, "client": clientID})
}

// upsertGrant records consent. A User has at most one active grant per app,
// and approving more scopes widens it.
func upsertGrant(tx core.App, userID, clientID string, scopes []string) (*core.Record, error) {
	grant, err := findActiveGrant(tx, userID, clientID)
	if errors.Is(err, sql.ErrNoRows) {
		collection, findErr := tx.FindCollectionByNameOrId(GrantsCollection)
		if findErr != nil {
			return nil, findErr
		}
		grant = core.NewRecord(collection)
		grant.Set("user", userID)
		grant.Set("client", clientID)
	} else if err != nil {
		return nil, err
	}
	grant.Set("scopes", unionScopes(grantScopes(grant), scopes))
	return grant, tx.Save(grant)
}

func grantScopes(grant *core.Record) []string {
	return stringList(grant, "scopes")
}

func dbTime(t time.Time) string {
	return t.UTC().Format(types.DefaultDateLayout)
}

// claimOnce stamps an empty date field and reports whether this call did it.
// The conditional UPDATE keeps requests, codes and refresh tokens single-use
// even when two requests race.
func claimOnce(app core.App, collection, id, field string, now time.Time) (bool, error) {
	result, err := app.NonconcurrentDB().Update(collection,
		dbx.Params{field: dbTime(now)},
		dbx.NewExp("[[id]] = {:id} AND [["+field+"]] = ''", dbx.Params{"id": id}),
	).Execute()
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
```

`backend/internal/oidc/consent.go`:

```go
package oidc

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

var (
	ErrRequestNotFound          = errors.New("authorization request not found or expired")
	ErrReauthenticationRequired = errors.New("reauthentication required")
	ErrUserNotAllowed           = errors.New("account cannot sign in to connected apps")
)

// Session is what the panel knows about the session deciding a request.
type Session struct {
	AuthTime time.Time
	Known    bool
}

// PendingRequest is an authorization request that can still be decided.
type PendingRequest struct {
	record *core.Record
	Client *core.Record
}

// FindPendingRequest loads an unconsumed, unexpired request whose app is
// still enabled and still lists its redirect_uri.
func FindPendingRequest(app core.App, handle string, now time.Time) (*PendingRequest, error) {
	if handle == "" {
		return nil, ErrRequestNotFound
	}
	rec, err := app.FindFirstRecordByFilter(AuthRequestsCollection,
		"request_hash = {:hash} && consumed_at = '' && expires_at > {:now}",
		dbx.Params{"hash": token.Sha256Hex(handle), "now": dbTime(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	client, err := app.FindRecordById(ClientsCollection, rec.GetString("client"))
	if err != nil || !client.GetBool("enabled") || !slices.Contains(ClientRedirectURIs(client), rec.GetString("redirect_uri")) {
		return nil, ErrRequestNotFound
	}
	return &PendingRequest{record: rec, Client: client}, nil
}

func (p *PendingRequest) Scopes() []string     { return strings.Fields(p.record.GetString("scope")) }
func (p *PendingRequest) RedirectURI() string  { return p.record.GetString("redirect_uri") }
func (p *PendingRequest) ExpiresAt() time.Time { return p.record.GetDateTime("expires_at").Time() }
func (p *PendingRequest) PromptNone() bool     { return p.hasPrompt(PromptNone) }

func (p *PendingRequest) hasPrompt(value string) bool {
	return slices.Contains(strings.Fields(p.record.GetString("prompt")), value)
}

// NeedsReauthentication reports whether the session is too old for the
// request. prompt=login needs a sign-in after the request was made.
func (p *PendingRequest) NeedsReauthentication(s Session, now time.Time) bool {
	requestedAt := p.ExpiresAt().Add(-AuthRequestTTL)
	if p.hasPrompt(PromptLogin) && (!s.Known || s.AuthTime.Before(requestedAt)) {
		return true
	}
	maxAge := p.record.GetInt("max_age")
	return maxAge > 0 && (!s.Known || now.Sub(s.AuthTime) > time.Duration(maxAge)*time.Second)
}

// NeedsConsent reports whether the User must see the consent page. Consent
// is remembered per User, app and scope set (ADR-0010).
func (p *PendingRequest) NeedsConsent(app core.App, userID string) (bool, error) {
	if p.hasPrompt(PromptConsent) || p.hasPrompt(PromptSelectAccount) {
		return true, nil
	}
	if p.Client.GetBool("skip_consent") {
		return false, nil
	}
	grant, err := findActiveGrant(app, userID, p.Client.Id)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !isSubset(p.Scopes(), grantScopes(grant)), nil
}

// Approve turns a pending request into an authorization code and returns the
// redirect back to the app. Approving is the consent, so callers show the
// consent page first unless NeedsConsent says it is not needed.
func Approve(app core.App, handle string, user *core.Record, s Session, issuer string, now time.Time) (string, error) {
	var redirect string
	err := app.RunInTransaction(func(tx core.App) error {
		p, err := FindPendingRequest(tx, handle, now)
		if err != nil {
			return err
		}
		if !UserMayUse(p.Client, user) {
			return ErrUserNotAllowed
		}
		silent, err := p.silentFailure(tx, user.Id, s, now)
		if err != nil {
			return err
		}
		if silent != nil {
			redirect, err = p.fail(tx, silent, issuer, now)
		} else {
			redirect, err = p.approve(tx, user.Id, s, issuer, now)
		}
		return err
	})
	return redirect, err
}

// silentFailure returns the error a prompt=none request must end with. A
// request that may show pages fails with ErrReauthenticationRequired instead
// and stays pending, so the User can sign in again and come back.
func (p *PendingRequest) silentFailure(tx core.App, userID string, s Session, now time.Time) (*Error, error) {
	if p.NeedsReauthentication(s, now) {
		if p.PromptNone() {
			return NewError(LoginRequired, "the user must sign in again"), nil
		}
		return nil, ErrReauthenticationRequired
	}
	if !p.PromptNone() {
		return nil, nil
	}
	needsConsent, err := p.NeedsConsent(tx, userID)
	if err != nil || !needsConsent {
		return nil, err
	}
	return NewError(ConsentRequired, "the user has not approved this app"), nil
}

// Deny ends a request with access_denied.
func Deny(app core.App, handle, issuer string, now time.Time) (string, error) {
	return finishWithError(app, handle, func(*PendingRequest) *Error {
		return NewError(AccessDenied, "the user denied the request")
	}, issuer, now)
}

// Abort ends a request for a User who is not signed in to the panel.
// prompt=none gets login_required (OIDC Core 3.1.2.6). A User leaving the
// sign-in page gets access_denied.
func Abort(app core.App, handle, issuer string, now time.Time) (string, error) {
	return finishWithError(app, handle, func(p *PendingRequest) *Error {
		if p.PromptNone() {
			return NewError(LoginRequired, "the user is not signed in")
		}
		return NewError(AccessDenied, "the user cancelled the sign-in")
	}, issuer, now)
}

func finishWithError(app core.App, handle string, errorFor func(*PendingRequest) *Error, issuer string, now time.Time) (string, error) {
	var redirect string
	err := app.RunInTransaction(func(tx core.App) error {
		p, err := FindPendingRequest(tx, handle, now)
		if err != nil {
			return err
		}
		redirect, err = p.fail(tx, errorFor(p), issuer, now)
		return err
	})
	return redirect, err
}

func (p *PendingRequest) consume(tx core.App, now time.Time) error {
	claimed, err := claimOnce(tx, AuthRequestsCollection, p.record.Id, "consumed_at", now)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrRequestNotFound
	}
	return nil
}

func (p *PendingRequest) fail(tx core.App, oauthErr *Error, issuer string, now time.Time) (string, error) {
	if err := p.consume(tx, now); err != nil {
		return "", err
	}
	return ErrorRedirect(p.RedirectURI(), oauthErr, p.record.GetString("state"), issuer), nil
}

func (p *PendingRequest) approve(tx core.App, userID string, s Session, issuer string, now time.Time) (string, error) {
	if err := p.consume(tx, now); err != nil {
		return "", err
	}
	grant, err := upsertGrant(tx, userID, p.Client.Id, p.Scopes())
	if err != nil {
		return "", err
	}
	code, err := p.saveCode(tx, grant, s, now)
	if err != nil {
		return "", err
	}
	return codeRedirect(p.RedirectURI(), code, p.record.GetString("state"), issuer), nil
}

// saveCode issues a single-use code that starts a new token family.
func (p *PendingRequest) saveCode(tx core.App, grant *core.Record, s Session, now time.Time) (string, error) {
	code, err := token.New(32)
	if err != nil {
		return "", err
	}
	family, err := token.New(16)
	if err != nil {
		return "", err
	}
	collection, err := tx.FindCollectionByNameOrId(CodesCollection)
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(collection)
	rec.Set("code_hash", token.Sha256Hex(code))
	rec.Set("grant", grant.Id)
	rec.Set("family", family)
	rec.Set("redirect_uri", p.RedirectURI())
	rec.Set("scope", p.record.GetString("scope"))
	rec.Set("nonce", p.record.GetString("nonce"))
	rec.Set("code_challenge", p.record.GetString("code_challenge"))
	rec.Set("expires_at", now.UTC().Add(CodeTTL))
	if s.Known {
		rec.Set("auth_time", s.AuthTime.UTC())
	}
	return code, tx.Save(rec)
}
```

- [ ] **Step 4: Add the panel endpoints, DTOs and OpenAPI**

Append to `backend/internal/api/dto.go`:

```go
// OIDCAuthorizationRequest describes a pending Connected App sign-in to the
// consent page.
type OIDCAuthorizationRequest struct {
	ClientName               string   `json:"client_name"`
	RedirectOrigin           string   `json:"redirect_origin"`
	Scopes                   []string `json:"scopes"`
	ConsentRequired          bool     `json:"consent_required"`
	ReauthenticationRequired bool     `json:"reauthentication_required"`
	PromptNone               bool     `json:"prompt_none"`
	ExpiresAt                string   `json:"expires_at"`
}

// OIDCAuthorizationDecision is where the browser goes after a decision.
type OIDCAuthorizationDecision struct {
	RedirectTo string `json:"redirect_to"`
}
```

`backend/internal/api/oidc_consent.go`:

```go
package api

import (
	"errors"
	"log"
	"net/url"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/oidc"
)

func (h *Handlers) getOIDCAuthorizationRequest(e *core.RequestEvent) error {
	now := time.Now()
	p, err := h.pendingOIDCRequest(e, now)
	if err != nil {
		return err
	}
	needsConsent, err := p.NeedsConsent(h.app, e.Auth.Id)
	if err != nil {
		return apis.NewInternalServerError("failed to check consent", err)
	}
	return ok(e, OIDCAuthorizationRequest{
		ClientName:               p.Client.GetString("name"),
		RedirectOrigin:           redirectOrigin(p.RedirectURI()),
		Scopes:                   p.Scopes(),
		ConsentRequired:          needsConsent,
		ReauthenticationRequired: p.NeedsReauthentication(h.oidcSession(e, now), now),
		PromptNone:               p.PromptNone(),
		ExpiresAt:                p.ExpiresAt().UTC().Format(types.DefaultDateLayout),
	})
}

func (h *Handlers) pendingOIDCRequest(e *core.RequestEvent, now time.Time) (*oidc.PendingRequest, error) {
	p, err := oidc.FindPendingRequest(h.app, e.Request.PathValue("handle"), now)
	if err != nil {
		return nil, oidcDecisionError(err)
	}
	if !oidc.UserMayUse(p.Client, e.Auth) {
		return nil, oidcDecisionError(oidc.ErrUserNotAllowed)
	}
	return p, nil
}

func (h *Handlers) approveOIDCAuthorizationRequest(e *core.RequestEvent) error {
	now := time.Now()
	redirect, err := oidc.Approve(h.app, e.Request.PathValue("handle"), e.Auth, h.oidcSession(e, now), oidcIssuer(e), now)
	return oidcDecision(e, redirect, err)
}

func (h *Handlers) denyOIDCAuthorizationRequest(e *core.RequestEvent) error {
	redirect, err := oidc.Deny(h.app, e.Request.PathValue("handle"), oidcIssuer(e), time.Now())
	return oidcDecision(e, redirect, err)
}

// abortOIDCAuthorizationRequest is public, because the User is not signed in
// to the panel. The request handle is the only credential.
func (h *Handlers) abortOIDCAuthorizationRequest(e *core.RequestEvent) error {
	now := time.Now()
	if !h.oidcLimits.authorize.allow(e.RealIP(), now) {
		return apis.NewTooManyRequestsError("too many requests", nil)
	}
	redirect, err := oidc.Abort(h.app, e.Request.PathValue("handle"), oidcIssuer(e), now)
	return oidcDecision(e, redirect, err)
}

func oidcDecision(e *core.RequestEvent, redirect string, err error) error {
	if err != nil {
		return oidcDecisionError(err)
	}
	return ok(e, OIDCAuthorizationDecision{RedirectTo: redirect})
}

func oidcDecisionError(err error) error {
	switch {
	case errors.Is(err, oidc.ErrRequestNotFound):
		return apis.NewNotFoundError("authorization request not found or expired", nil)
	case errors.Is(err, oidc.ErrReauthenticationRequired):
		return apis.NewBadRequestError("reauthentication required", nil)
	case errors.Is(err, oidc.ErrUserNotAllowed):
		return apis.NewForbiddenError("account cannot sign in to connected apps", nil)
	}
	return apis.NewInternalServerError("failed to complete the authorization request", err)
}

// oidcSession reads when the calling panel session signed in.
func (h *Handlers) oidcSession(e *core.RequestEvent, now time.Time) oidc.Session {
	authTime, known, err := oidc.SessionAuthTime(h.app, bearerToken(e.Request), now)
	if err != nil {
		log.Printf("[oidc] read session auth time: %v", err)
	}
	return oidc.Session{AuthTime: authTime, Known: known}
}

func redirectOrigin(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
```

In `backend/internal/api/api.go`, after the `/oidc-clients` routes add:

```go
	// OIDC consent (ADR-0010). Any usable User; 404 while the provider is off.
	oidcServing := h.requireOIDCServing()
	g.GET("/oidc/requests/{handle}", h.getOIDCAuthorizationRequest).Bind(oidcServing)
	g.POST("/oidc/requests/{handle}/approve", h.approveOIDCAuthorizationRequest).Bind(oidcServing)
	g.POST("/oidc/requests/{handle}/deny", h.denyOIDCAuthorizationRequest).Bind(oidcServing)
```

and next to the public passkey routes:

```go
	// Public OIDC abort for a User who is not signed in to the panel.
	se.Router.POST("/api/panel/oidc/requests/{handle}/abort", h.abortOIDCAuthorizationRequest).Bind(h.requireOIDCServing())
```

In `backend/internal/api/openapi.go`, register the schemas next to the other OIDC entries:

```go
		"OIDCAuthorizationRequest":          OIDCAuthorizationRequest{},
		"OIDCAuthorizationDecision":         OIDCAuthorizationDecision{},
```

add the required lists:

```go
		"OIDCAuthorizationRequest":       {"client_name", "redirect_origin", "scopes", "consent_required", "reauthentication_required", "prompt_none", "expires_at"},
		"OIDCAuthorizationDecision":      {"redirect_to"},
```

and add the paths after `/api/panel/oidc-clients/{id}/rotate-secret`:

```go
	// ── /oidc/requests (consent) ──────────────────────────────────────────
	handleParam := &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        "handle",
			In:          "path",
			Required:    true,
			Description: "Authorization request handle from the consent page URL",
			Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		},
	}
	decisionOp := func(id, summary string, auth bool) *openapi3.Operation {
		op := &openapi3.Operation{
			OperationID: id,
			Summary:     summary,
			Tags:        []string{"connected apps"},
			Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
				Value: &openapi3.Response{
					Description: ptr("Where the browser goes next"),
					Content:     content(ref("OIDCAuthorizationDecision")),
				},
			})),
		}
		op.Responses.Set("404", notFound)
		if auth {
			op.Responses.Set("400", badRequest)
			op.Responses.Set("403", forbidden)
			withAuth(op)
		}
		return op
	}
	t.Paths.Set("/api/panel/oidc/requests/{handle}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{handleParam},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "getOIDCAuthorizationRequest",
				Summary:     "Describe a pending Connected App sign-in for the consent page",
				Tags:        []string{"connected apps"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Pending authorization request"),
						Content:     content(ref("OIDCAuthorizationRequest")),
					},
				})),
			}
			op.Responses.Set("403", forbidden)
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})
	t.Paths.Set("/api/panel/oidc/requests/{handle}/approve", &openapi3.PathItem{
		Parameters: openapi3.Parameters{handleParam},
		Post:       decisionOp("approveOIDCAuthorizationRequest", "Approve a Connected App sign-in and return the redirect with a code", true),
	})
	t.Paths.Set("/api/panel/oidc/requests/{handle}/deny", &openapi3.PathItem{
		Parameters: openapi3.Parameters{handleParam},
		Post:       decisionOp("denyOIDCAuthorizationRequest", "Deny a Connected App sign-in", true),
	})
	t.Paths.Set("/api/panel/oidc/requests/{handle}/abort", &openapi3.PathItem{
		Parameters: openapi3.Parameters{handleParam},
		Post:       decisionOp("abortOIDCAuthorizationRequest", "End a Connected App sign-in for a visitor who is not signed in", false),
	})
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test -timeout 120s ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS. A hang in `claimOnce` means `tx.NonconcurrentDB()` is not the transaction; check `core.BaseApp.RunInTransaction` before changing the approach.

Run: `cd frontend && pnpm api:sync && pnpm typecheck`
Expected: PASS. The generated `schema.d.ts` is gitignored.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/oidc backend/internal/api
git commit -m "feat(backend): add oidc consent endpoints" -m "The consent page reads a pending request, then approves, denies or aborts it. Approving consumes the request once, widens the User's single grant for the app and issues a 60-second code that starts a new token family. prompt=none ends with login_required or consent_required instead of showing a page."
```

### Task 17: Token endpoint, code exchange and ID tokens

**Files:**
- Create: `backend/internal/oidc/tokens.go`, `backend/internal/oidc/idtoken.go`, `backend/internal/oidc/tokens_test.go`
- Modify: `backend/internal/oidc/grants.go` (`errInvalidGrant`, `grantUser`, `revokeFamily`)
- Modify: `backend/internal/api/oidc_protocol.go`, `backend/internal/api/oidc_protocol_test.go`
- Modify: `backend/go.mod` (`github.com/golang-jwt/jwt/v5` becomes a direct dependency)

**Interfaces:**
- Consumes: `Approve`, `claimOnce`, `dbTime`, `UserMayUse`, `ActiveSigningKey`, `SigningAlgorithm`, `AccessTokenTTL`, `IDTokenTTL`, `RefreshTokenIdleTTL`, `RefreshTokenMaxTTL`, `oidcIssuer(e)`, `h.oidcLimits`, `AuthenticateClient`, `HasRepeatedParams`, the Task 16 test helpers.
- Produces:
  - `oidc.GrantTypeAuthorizationCode`, `oidc.GrantTypeRefreshToken`.
  - `type oidc.TokenRequest struct { Client *core.Record; Form url.Values; Issuer string; Box *cryptobox.Box; Now time.Time }`.
  - `type oidc.TokenResponse` with JSON fields `access_token`, `token_type`, `expires_in`, `refresh_token` (omitted when empty), `id_token`, `scope`.
  - `oidc.ExchangeCode(app core.App, req TokenRequest) (*TokenResponse, *Error)`.
  - `oidc.UserClaims(user *core.Record, scope []string) map[string]any`.
  - Unexported: `issuance`, `runGrant`, `findIssued`, `claimGrant`, `replayed`, `verifyPKCE`, `issueTokens`, `saveToken`, `saveRefreshToken`, `touchLastUsed`, `signIDToken`, `atHash`, `errInvalidGrant`, `grantUser`, `revokeFamily`.
  - API side: `(h) oidcToken`, `(h) oidcAuthenticatedClient(e) (*core.Record, *oidc.Error)`, `writeOAuthError(e, *oidc.Error) error`, `setNoStore(e)`.
  - Test helpers: `testMasterKey`, `applyOverrides`, `approvedCode`, `codeRequest`, `exchangedTokens`, `parseIDToken`, `tokenRecord`, `tokenRevoked` in the oidc package. `oauthErrorResponse`, `(env) postClientForm(path, form, secret)`, `(env) postToken(form)`, `codeForm(code)`, `(env) tokens(overrides) oidc.TokenResponse`, `(env) verifyIDToken(raw) jwt.MapClaims` and `rsaPublicKey` in the api package.

- [ ] **Step 1: Write the failing tests**

`backend/internal/oidc/tokens_test.go`:

```go
package oidc

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/token"
)

const testMasterKey = "oidc-test-master-key"

// applyOverrides replaces form values. An empty override removes the
// parameter.
func applyOverrides(form url.Values, overrides map[string]string) {
	for key, value := range overrides {
		if value == "" {
			form.Del(key)
		} else {
			form.Set(key, value)
		}
	}
}

// approvedCode approves a request for user, who signed in a minute earlier,
// and returns the code sent back to the app.
func approvedCode(t *testing.T, app core.App, client, user *core.Record, overrides map[string]string) string {
	t.Helper()
	handle := newPendingHandle(t, app, client, overrides, testNow)
	redirect, err := Approve(app, handle, user, signedInAt(testNow.Add(-time.Minute)), testIssuer, testNow)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return redirectQuery(t, redirect).Get("code")
}

// codeRequest is a valid exchange of code one second after approval.
func codeRequest(client *core.Record, box *cryptobox.Box, code string, overrides map[string]string) TokenRequest {
	form := url.Values{
		"grant_type":    {GrantTypeAuthorizationCode},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"code_verifier": {testVerifier},
	}
	applyOverrides(form, overrides)
	return TokenRequest{Client: client, Form: form, Issuer: testIssuer, Box: box, Now: testNow.Add(time.Second)}
}

// exchangedTokens approves a request and exchanges its code.
func exchangedTokens(t *testing.T, app core.App, box *cryptobox.Box, client, user *core.Record, overrides map[string]string) *TokenResponse {
	t.Helper()
	resp, oauthErr := ExchangeCode(app, codeRequest(client, box, approvedCode(t, app, client, user, overrides), nil))
	if oauthErr != nil {
		t.Fatalf("exchange: %v", oauthErr)
	}
	return resp
}

// parseIDToken verifies an ID token with the active signing key at now.
func parseIDToken(t *testing.T, app core.App, box *cryptobox.Box, client *core.Record, raw string, now time.Time) jwt.MapClaims {
	t.Helper()
	key, err := ActiveSigningKey(app, box, now)
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	parsed, err := jwt.Parse(raw, func(tok *jwt.Token) (any, error) {
		if tok.Header["kid"] != key.ID {
			return nil, errors.New("unexpected kid")
		}
		return &key.Private.PublicKey, nil
	}, jwt.WithValidMethods([]string{SigningAlgorithm}), jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithIssuer(testIssuer), jwt.WithAudience(client.GetString("client_id")))
	if err != nil {
		t.Fatalf("parse id token: %v", err)
	}
	return parsed.Claims.(jwt.MapClaims)
}

// tokenRecord loads a stored access or refresh token by its raw value.
func tokenRecord(t *testing.T, app core.App, raw string) *core.Record {
	t.Helper()
	rec, err := app.FindFirstRecordByFilter(TokensCollection, "token_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(raw)})
	if err != nil {
		t.Fatalf("find token: %v", err)
	}
	return rec
}

func tokenRevoked(t *testing.T, app core.App, raw string) bool {
	t.Helper()
	return !tokenRecord(t, app, raw).GetDateTime("revoked_at").IsZero()
}

func TestExchangeCodeIssuesTokensAndASignedIDToken(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	user := newTestUser(t, app, "exchange@example.com")
	issuedAt := testNow.Add(time.Second)

	resp := exchangedTokens(t, app, box, client, user, map[string]string{"scope": "openid email offline_access"})
	if resp.TokenType != "Bearer" || resp.ExpiresIn != 600 || resp.AccessToken == "" || resp.RefreshToken == "" ||
		resp.Scope != "openid email offline_access" {
		t.Fatalf("response = %+v", resp)
	}
	claims := parseIDToken(t, app, box, client, resp.IDToken, issuedAt)
	want := map[string]any{
		"sub":            user.Id,
		"email":          "exchange@example.com",
		"email_verified": true,
		"nonce":          "nonce-1",
		"auth_time":      float64(testNow.Add(-time.Minute).Unix()),
		"iat":            float64(issuedAt.Unix()),
		"exp":            float64(issuedAt.Add(IDTokenTTL).Unix()),
		"at_hash":        atHash(resp.AccessToken),
	}
	for name, value := range want {
		if claims[name] != value {
			t.Errorf("claim %s = %v, want %v", name, claims[name], value)
		}
	}

	access, refresh := tokenRecord(t, app, resp.AccessToken), tokenRecord(t, app, resp.RefreshToken)
	if access.GetString("kind") != "access" || refresh.GetString("kind") != "refresh" ||
		access.GetString("family") == "" || access.GetString("family") != refresh.GetString("family") ||
		!access.GetDateTime("expires_at").Time().Equal(issuedAt.Add(AccessTokenTTL)) ||
		!refresh.GetDateTime("expires_at").Time().Equal(issuedAt.Add(RefreshTokenIdleTTL)) ||
		!refresh.GetDateTime("absolute_expires_at").Time().Equal(issuedAt.Add(RefreshTokenMaxTTL)) {
		t.Fatalf("stored tokens = %v / %v", access.PublicExport(), refresh.PublicExport())
	}

	grants, err := app.FindAllRecords(GrantsCollection)
	if err != nil || len(grants) != 1 {
		t.Fatalf("grants = %d, %v", len(grants), err)
	}
	fresh, err := app.FindRecordById(ClientsCollection, client.Id)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	for _, rec := range []*core.Record{grants[0], fresh} {
		if !rec.GetDateTime("last_used_at").Time().Equal(issuedAt) {
			t.Errorf("%s last_used_at = %v", rec.Collection().Name, rec.GetDateTime("last_used_at"))
		}
	}
}

func TestExchangeCodeIssuesARefreshTokenOnlyForOfflineAccess(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	user := newTestUser(t, app, "offline@example.com")
	refreshing, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	plain, _ := newTestClient(t, app, nil)
	cases := []struct {
		name   string
		client *core.Record
		scope  string
	}{
		{"without offline_access", refreshing, "openid email"},
		{"refresh turned off", plain, "openid email offline_access"},
	}
	for _, c := range cases {
		resp := exchangedTokens(t, app, box, c.client, user, map[string]string{"scope": c.scope})
		if resp.RefreshToken != "" || resp.Scope != "openid email" {
			t.Errorf("%s: response = %+v", c.name, resp)
		}
	}
}

func TestExchangeCodeRejectsMismatchedRequests(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, nil)
	other, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "mismatch@example.com")
	code := approvedCode(t, app, client, user, nil)

	cases := []struct {
		name      string
		client    *core.Record
		overrides map[string]string
		want      string
	}{
		{"missing code", client, map[string]string{"code": ""}, InvalidRequest},
		{"missing redirect_uri", client, map[string]string{"redirect_uri": ""}, InvalidRequest},
		{"missing code_verifier", client, map[string]string{"code_verifier": ""}, InvalidRequest},
		{"unknown code", client, map[string]string{"code": "unknown"}, InvalidGrant},
		{"wrong code_verifier", client, map[string]string{"code_verifier": strings.Repeat("a", 43)}, InvalidGrant},
		{"short code_verifier", client, map[string]string{"code_verifier": testVerifier[:42]}, InvalidGrant},
		{"other redirect_uri", client, map[string]string{"redirect_uri": "https://app.example.com/other"}, InvalidGrant},
		{"another app", other, nil, InvalidGrant},
	}
	for _, c := range cases {
		resp, oauthErr := ExchangeCode(app, codeRequest(c.client, box, code, c.overrides))
		if resp != nil || oauthErr == nil || oauthErr.Code != c.want {
			t.Errorf("%s: %+v, %v; want %s", c.name, resp, oauthErr, c.want)
		}
	}
	if _, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil)); oauthErr != nil {
		t.Fatalf("failed attempts used up the code: %v", oauthErr)
	}

	expired := codeRequest(client, box, approvedCode(t, app, client, user, nil), nil)
	expired.Now = testNow.Add(CodeTTL)
	if _, oauthErr := ExchangeCode(app, expired); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("expired code: %v", oauthErr)
	}
}

func TestExchangeCodeReplayRevokesOnlyItsFamily(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, map[string]any{"refresh_enabled": true})
	user := newTestUser(t, app, "replay@example.com")
	offline := map[string]string{"scope": "openid email offline_access"}

	code := approvedCode(t, app, client, user, offline)
	first, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil))
	if oauthErr != nil {
		t.Fatalf("first exchange: %v", oauthErr)
	}
	other := exchangedTokens(t, app, box, client, user, offline)

	if _, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil)); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("replay: %v", oauthErr)
	}
	if !tokenRevoked(t, app, first.AccessToken) || !tokenRevoked(t, app, first.RefreshToken) {
		t.Fatal("the replayed code's tokens still work")
	}
	if tokenRevoked(t, app, other.AccessToken) || tokenRevoked(t, app, other.RefreshToken) {
		t.Fatal("the replay revoked another family")
	}
	active, err := app.FindRecordsByFilter(GrantsCollection, "revoked_at = ''", "", 0, 0)
	if err != nil || len(active) != 1 {
		t.Fatalf("active grants = %d, %v; the remembered consent must stay", len(active), err)
	}
}

func TestExchangeCodeRejectsAUserWhoCannotSignIn(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, testMasterKey)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "disabled-exchange@example.com")
	code := approvedCode(t, app, client, user, nil)

	user.Set("status", "disabled")
	if err := app.Save(user); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, oauthErr := ExchangeCode(app, codeRequest(client, box, code, nil)); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("disabled user: %v", oauthErr)
	}
}
```

Append to `backend/internal/api/oidc_protocol_test.go` (add `"crypto/rsa"`, `"encoding/base64"`, `"errors"`, `"math/big"` and `"github.com/golang-jwt/jwt/v5"` to the imports):

```go
type oauthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// postClientForm sends a back-channel request with client_secret_basic.
func (env *oidcTestEnv) postClientForm(path string, form url.Values, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(env.client.ClientID), url.QueryEscape(secret))
	return env.do(req)
}

func (env *oidcTestEnv) postToken(form url.Values) *httptest.ResponseRecorder {
	return env.postClientForm("/api/oidc/token", form, env.secret)
}

func codeForm(code string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {helpCenterCallback},
		"code_verifier": {oidcTestVerifier},
	}
}

// tokens runs the whole code flow as the env's User.
func (env *oidcTestEnv) tokens(overrides map[string]string) oidc.TokenResponse {
	env.t.Helper()
	res := env.postToken(codeForm(env.approveCode(overrides)))
	if res.Code != http.StatusOK {
		env.t.Fatalf("token = %d %s", res.Code, res.Body.String())
	}
	return decodeBody[oidc.TokenResponse](env.t, res)
}

// verifyIDToken checks an ID token the way a relying party does, with the
// served JWKS and the issuer from discovery.
func (env *oidcTestEnv) verifyIDToken(raw string) jwt.MapClaims {
	env.t.Helper()
	jwks := decodeBody[oidc.JWKS](env.t, env.get("/api/oidc/jwks"))
	issuer := decodeBody[oidc.ProviderMetadata](env.t, env.get("/api/oidc/.well-known/openid-configuration")).Issuer
	parsed, err := jwt.Parse(raw, func(tok *jwt.Token) (any, error) {
		for _, key := range jwks.Keys {
			if key.KeyID == tok.Header["kid"] {
				return rsaPublicKey(key)
			}
		}
		return nil, errors.New("kid is not in the jwks")
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(issuer), jwt.WithAudience(env.client.ClientID), jwt.WithExpirationRequired())
	if err != nil {
		env.t.Fatalf("verify id token: %v", err)
	}
	return parsed.Claims.(jwt.MapClaims)
}

func rsaPublicKey(key oidc.JWK) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(key.Modulus)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(key.Exponent)
	if err != nil {
		return nil, err
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
}

func TestOIDCCodeFlowEndToEnd(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	code := env.approveCode(nil)
	res := env.postToken(codeForm(code))
	if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" || res.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("token = %d %v %s", res.Code, res.Header(), res.Body.String())
	}
	tokens := decodeBody[oidc.TokenResponse](t, res)
	claims := env.verifyIDToken(tokens.IDToken)
	if claims["sub"] != env.user.Id || claims["email"] != "oidc-user@example.com" || claims["nonce"] != "rp-nonce" ||
		tokens.Scope != "openid email" || tokens.RefreshToken != "" {
		t.Fatalf("claims = %v, tokens = %+v", claims, tokens)
	}

	res = env.postToken(codeForm(code))
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusBadRequest || body.Error != "invalid_grant" {
		t.Fatalf("replay = %d %s", res.Code, res.Body.String())
	}
}

func TestOIDCTokenEndpointRejectsBadRequests(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	res := env.postClientForm("/api/oidc/token", codeForm("unused"), "wrong-secret")
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusUnauthorized || body.Error != "invalid_client" ||
		res.Header().Get("WWW-Authenticate") != `Basic realm="oidc"` {
		t.Fatalf("wrong secret = %d %v %s", res.Code, res.Header(), res.Body.String())
	}

	cases := map[string]struct {
		form url.Values
		want string
	}{
		"unsupported grant type": {url.Values{"grant_type": {"password"}}, "unsupported_grant_type"},
		"missing grant type":     {url.Values{"code": {"unused"}}, "invalid_request"},
		"repeated parameter":     {url.Values{"grant_type": {"authorization_code", "authorization_code"}}, "invalid_request"},
	}
	for name, c := range cases {
		res := env.postToken(c.form)
		if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusBadRequest || body.Error != c.want {
			t.Errorf("%s = %d %s", name, res.Code, res.Body.String())
		}
	}
}

func TestOIDCTokenEndpointLocksOutRepeatedClientFailures(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	for range oidcClientFailureMax {
		if res := env.postClientForm("/api/oidc/token", codeForm("unused"), "wrong-secret"); res.Code != http.StatusUnauthorized {
			t.Fatalf("failure = %d %s", res.Code, res.Body.String())
		}
	}
	res := env.postToken(codeForm(env.approveCode(nil)))
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusTooManyRequests || body.Error != "temporarily_unavailable" {
		t.Fatalf("after the lockout = %d %s", res.Code, res.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'ExchangeCode'; go test ./internal/api -run 'OIDCCodeFlow|OIDCTokenEndpoint'`
Expected: FAIL to compile with `undefined: ExchangeCode` and `undefined: oidc.TokenResponse`.

- [ ] **Step 3: Add the grant helpers**

Append to `backend/internal/oidc/grants.go`:

```go
// errInvalidGrant answers every unusable code or refresh token alike, so a
// client cannot tell an unknown value from an expired or foreign one.
var errInvalidGrant = NewError(InvalidGrant, "the code or refresh token is invalid or expired")

// grantUser returns the User behind an unrevoked grant who may still use the
// app, or errInvalidGrant.
func grantUser(app core.App, grant, client *core.Record) (*core.Record, error) {
	if !grant.GetDateTime("revoked_at").IsZero() {
		return nil, errInvalidGrant
	}
	user, err := app.FindRecordById("users", grant.GetString("user"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	if !UserMayUse(client, user) {
		return nil, errInvalidGrant
	}
	return user, nil
}

// revokeFamily revokes every token issued from one code.
func revokeFamily(app core.App, family string, now time.Time) error {
	_, err := app.NonconcurrentDB().Update(TokensCollection,
		dbx.Params{"revoked_at": dbTime(now)},
		dbx.NewExp("[[family]] = {:family} AND [[revoked_at]] = ''", dbx.Params{"family": family}),
	).Execute()
	return err
}
```

- [ ] **Step 4: Implement the code exchange and the ID token**

`backend/internal/oidc/tokens.go`:

```go
package oidc

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/token"
)

// Grant types accepted at the token endpoint.
const (
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
)

const (
	tokenKindAccess  = "access"
	tokenKindRefresh = "refresh"
	tokenTypeBearer  = "Bearer"
)

// codeVerifierPattern is the code_verifier syntax of RFC 7636 4.1.
var codeVerifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// TokenRequest is a token endpoint request from an authenticated app.
type TokenRequest struct {
	Client *core.Record
	Form   url.Values
	Issuer string
	Box    *cryptobox.Box
	Now    time.Time
}

// TokenResponse is the response of RFC 6749 5.1 and OIDC Core 3.1.3.3.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

// issuance is what one token response is issued from. scope covers the
// access token and the ID token. refreshScope covers the refresh token,
// which keeps the granted scope when a refresh narrows it.
type issuance struct {
	grant, user  *core.Record
	family       string
	scope        []string
	refreshScope []string
	authTime     time.Time
	nonce        string
	refreshUntil time.Time
}

// ExchangeCode redeems an authorization code (RFC 6749 4.1.3, RFC 7636 4.6).
func ExchangeCode(app core.App, req TokenRequest) (*TokenResponse, *Error) {
	raw := req.Form.Get("code")
	if raw == "" || req.Form.Get("redirect_uri") == "" || req.Form.Get("code_verifier") == "" {
		return nil, NewError(InvalidRequest, "code, redirect_uri and code_verifier are required")
	}
	return runGrant(app, func(tx core.App) (*TokenResponse, error) {
		code, grant, err := findIssued(tx, CodesCollection, "code_hash = {:hash}", raw, req.Client)
		if err != nil {
			return nil, err
		}
		if !code.GetDateTime("used_at").IsZero() {
			return nil, replayed(tx, code, req.Now)
		}
		if !req.Now.Before(code.GetDateTime("expires_at").Time()) ||
			code.GetString("redirect_uri") != req.Form.Get("redirect_uri") ||
			!verifyPKCE(req.Form.Get("code_verifier"), code.GetString("code_challenge")) {
			return nil, errInvalidGrant
		}
		user, err := grantUser(tx, grant, req.Client)
		if err != nil {
			return nil, err
		}
		if err := claimGrant(tx, CodesCollection, code, "used_at", req.Now); err != nil {
			return nil, err
		}
		scope := strings.Fields(code.GetString("scope"))
		return issueTokens(tx, req, issuance{
			grant:        grant,
			user:         user,
			family:       code.GetString("family"),
			scope:        scope,
			refreshScope: scope,
			authTime:     code.GetDateTime("auth_time").Time(),
			nonce:        code.GetString("nonce"),
		})
	})
}

// runGrant runs a grant in one transaction. An OAuth error commits, so the
// family revocation of a replay sticks although the request fails. Any other
// error rolls back and becomes server_error.
func runGrant(app core.App, grant func(tx core.App) (*TokenResponse, error)) (*TokenResponse, *Error) {
	var resp *TokenResponse
	var oauthErr *Error
	err := app.RunInTransaction(func(tx core.App) error {
		var err error
		resp, err = grant(tx)
		if errors.As(err, &oauthErr) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, WrapServerError(err)
	}
	if oauthErr != nil {
		return nil, oauthErr
	}
	return resp, nil
}

// findIssued loads a code or refresh token and its grant. A value issued to
// another app fails like an unknown one.
func findIssued(tx core.App, collection, filter, raw string, client *core.Record) (rec, grant *core.Record, err error) {
	rec, err = tx.FindFirstRecordByFilter(collection, filter, dbx.Params{"hash": token.Sha256Hex(raw)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errInvalidGrant
	}
	if err != nil {
		return nil, nil, err
	}
	grant, err = tx.FindRecordById(GrantsCollection, rec.GetString("grant"))
	if err != nil {
		return nil, nil, err
	}
	if grant.GetString("client") != client.Id {
		return nil, nil, errInvalidGrant
	}
	return rec, grant, nil
}

// claimGrant marks a code or refresh token used. Losing the claim means
// another request used it first, which is a replay.
func claimGrant(tx core.App, collection string, rec *core.Record, field string, now time.Time) error {
	claimed, err := claimOnce(tx, collection, rec.Id, field, now)
	if err != nil {
		return err
	}
	if !claimed {
		return replayed(tx, rec, now)
	}
	return nil
}

// replayed revokes every token of the family and fails the request
// (RFC 6749 4.1.2, RFC 9700 4.14.2).
func replayed(tx core.App, rec *core.Record, now time.Time) error {
	if err := revokeFamily(tx, rec.GetString("family"), now); err != nil {
		return err
	}
	return errInvalidGrant
}

// verifyPKCE checks an S256 code_verifier against the stored challenge.
func verifyPKCE(verifier, challenge string) bool {
	if !codeVerifierPattern.MatchString(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// issueTokens issues an access token, an ID token and, when the app allows
// refresh and the grant includes offline_access, a refresh token. Every grant
// includes openid, so every response carries an ID token.
func issueTokens(tx core.App, req TokenRequest, in issuance) (*TokenResponse, error) {
	accessToken, err := saveToken(tx, tokenKindAccess, in, in.scope, req.Now.Add(AccessTokenTTL), time.Time{})
	if err != nil {
		return nil, err
	}
	resp := &TokenResponse{
		AccessToken: accessToken,
		TokenType:   tokenTypeBearer,
		ExpiresIn:   int(AccessTokenTTL.Seconds()),
		Scope:       strings.Join(in.scope, " "),
	}
	if req.Client.GetBool("refresh_enabled") && slices.Contains(in.refreshScope, ScopeOfflineAccess) {
		if resp.RefreshToken, err = saveRefreshToken(tx, in, req.Now); err != nil {
			return nil, err
		}
	}
	if resp.IDToken, err = signIDToken(tx, req, in, accessToken); err != nil {
		return nil, err
	}
	return resp, touchLastUsed(tx, req.Now, in.grant, req.Client)
}

// saveRefreshToken slides the idle lifetime but never past the absolute cap
// set when the family started.
func saveRefreshToken(tx core.App, in issuance, now time.Time) (string, error) {
	until := in.refreshUntil
	if until.IsZero() {
		until = now.Add(RefreshTokenMaxTTL)
	}
	expires := now.Add(RefreshTokenIdleTTL)
	if until.Before(expires) {
		expires = until
	}
	return saveToken(tx, tokenKindRefresh, in, in.refreshScope, expires, until)
}

func saveToken(tx core.App, kind string, in issuance, scope []string, expiresAt, absoluteExpiresAt time.Time) (string, error) {
	raw, err := token.New(32)
	if err != nil {
		return "", err
	}
	collection, err := tx.FindCollectionByNameOrId(TokensCollection)
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(collection)
	rec.Set("token_hash", token.Sha256Hex(raw))
	rec.Set("kind", kind)
	rec.Set("grant", in.grant.Id)
	rec.Set("family", in.family)
	rec.Set("scope", strings.Join(scope, " "))
	rec.Set("expires_at", expiresAt.UTC())
	if !in.authTime.IsZero() {
		rec.Set("auth_time", in.authTime.UTC())
	}
	if !absoluteExpiresAt.IsZero() {
		rec.Set("absolute_expires_at", absoluteExpiresAt.UTC())
	}
	return raw, tx.Save(rec)
}

// touchLastUsed records when the grant and the app last received tokens.
// A plain UPDATE leaves the records' updated time alone.
func touchLastUsed(tx core.App, now time.Time, records ...*core.Record) error {
	for _, rec := range records {
		_, err := tx.NonconcurrentDB().Update(rec.Collection().Name,
			dbx.Params{"last_used_at": dbTime(now)}, dbx.HashExp{"id": rec.Id}).Execute()
		if err != nil {
			return err
		}
	}
	return nil
}
```

`backend/internal/oidc/idtoken.go`:

```go
package oidc

import (
	"crypto/sha256"
	"encoding/base64"
	"slices"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/pocketbase/core"
)

// UserClaims are the claims about a User that scope releases. The ID token
// and userinfo share them, and they follow what help-center-next reads.
func UserClaims(user *core.Record, scope []string) map[string]any {
	claims := map[string]any{"sub": user.Id}
	if slices.Contains(scope, ScopeEmail) {
		claims["email"] = user.Email()
		claims["email_verified"] = user.Verified()
	}
	return claims
}

// signIDToken signs the ID token of a token response (OIDC Core 3.1.3.6). A
// refresh passes the original auth_time and no nonce (OIDC Core 12.2).
func signIDToken(tx core.App, req TokenRequest, in issuance, accessToken string) (string, error) {
	key, err := ActiveSigningKey(tx, req.Box, req.Now)
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims(UserClaims(in.user, in.scope))
	claims["iss"] = req.Issuer
	claims["aud"] = req.Client.GetString("client_id")
	claims["iat"] = req.Now.Unix()
	claims["exp"] = req.Now.Add(IDTokenTTL).Unix()
	claims["at_hash"] = atHash(accessToken)
	if !in.authTime.IsZero() {
		claims["auth_time"] = in.authTime.Unix()
	}
	if in.nonce != "" {
		claims["nonce"] = in.nonce
	}
	idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	idToken.Header["kid"] = key.ID
	return idToken.SignedString(key.Private)
}

// atHash is the left half of the access token's SHA-256 (OIDC Core 3.1.3.6).
func atHash(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return base64.RawURLEncoding.EncodeToString(sum[:len(sum)/2])
}
```

Run: `cd backend && go mod tidy`
Expected: `github.com/golang-jwt/jwt/v5` moves to the direct `require` block.

- [ ] **Step 5: Add the token endpoint**

In `backend/internal/api/oidc_protocol.go`, add `"strconv"` to the imports, register the route in `bindOIDCRoutes`:

```go
	g.POST(oidc.TokenPath, h.oidcToken)
```

and append:

```go
// oauthErrorBody is the error response of RFC 6749 5.2.
type oauthErrorBody struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func (h *Handlers) oidcToken(e *core.RequestEvent) error {
	setNoStore(e)
	client, oauthErr := h.oidcAuthenticatedClient(e)
	if oauthErr != nil {
		return writeOAuthError(e, oauthErr)
	}
	req := oidc.TokenRequest{Client: client, Form: e.Request.PostForm, Issuer: oidcIssuer(e), Box: h.box, Now: time.Now()}
	var resp *oidc.TokenResponse
	switch e.Request.PostForm.Get("grant_type") {
	case oidc.GrantTypeAuthorizationCode:
		resp, oauthErr = oidc.ExchangeCode(h.app, req)
	case "":
		oauthErr = oidc.NewError(oidc.InvalidRequest, "grant_type is required")
	default:
		oauthErr = oidc.NewError(oidc.UnsupportedGrantType, "use authorization_code or refresh_token")
	}
	if oauthErr != nil {
		return writeOAuthError(e, oauthErr)
	}
	return e.JSON(http.StatusOK, resp)
}

// oidcAuthenticatedClient parses a back-channel request and authenticates
// the app. An address with too many failed authentications is refused even
// with the right secret until its window ends. Query parameters are ignored,
// so credentials in a URL never count.
func (h *Handlers) oidcAuthenticatedClient(e *core.RequestEvent) (*core.Record, *oidc.Error) {
	ip, now := e.RealIP(), time.Now()
	if !h.oidcLimits.backchannel.allow(ip, now) || h.oidcLimits.clientFailures.blocked(ip, now) {
		return nil, oidc.NewError(oidc.TemporarilyUnavailable, "too many requests")
	}
	if err := e.Request.ParseForm(); err != nil || oidc.HasRepeatedParams(e.Request.PostForm) {
		return nil, oidc.NewError(oidc.InvalidRequest, "send each parameter once in a form-encoded body")
	}
	client, oauthErr := oidc.AuthenticateClient(h.app, e.Request)
	if oauthErr != nil && oauthErr.Code == oidc.InvalidClient {
		h.oidcLimits.clientFailures.allow(ip, now)
	}
	return client, oauthErr
}

// writeOAuthError answers with RFC 6749 5.2 statuses. invalid_client always
// carries a Basic challenge, which keeps the 401 valid HTTP whichever method
// the client used.
func writeOAuthError(e *core.RequestEvent, oauthErr *oidc.Error) error {
	status := http.StatusBadRequest
	switch oauthErr.Code {
	case oidc.InvalidClient:
		status = http.StatusUnauthorized
		e.Response.Header().Set("WWW-Authenticate", `Basic realm="oidc"`)
	case oidc.TemporarilyUnavailable:
		status = http.StatusTooManyRequests
		e.Response.Header().Set("Retry-After", strconv.Itoa(int(oidcRateWindow.Seconds())))
	case oidc.ServerError:
		status = http.StatusInternalServerError
		log.Printf("[oidc] %s %s: %v", e.Request.Method, e.Request.URL.Path, oauthErr)
	}
	return e.JSON(status, oauthErrorBody{Error: oauthErr.Code, Description: oauthErr.Description})
}

// setNoStore keeps tokens and claims out of caches (RFC 6749 5.1).
func setNoStore(e *core.RequestEvent) {
	e.Response.Header().Set("Cache-Control", "no-store")
	e.Response.Header().Set("Pragma", "no-cache")
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test -timeout 120s ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/oidc backend/internal/api
git commit -m "feat(backend): exchange oidc authorization codes" -m "POST /api/oidc/token authenticates the app, checks the code's redirect_uri and S256 verifier, and returns an opaque access token, an RS256 ID token and, with offline_access, a refresh token. A replayed code fails with invalid_grant and revokes every token issued from it. Repeated client authentication failures lock the address out for a minute."
```

### Task 18: Refresh token rotation

**Files:**
- Modify: `backend/internal/oidc/tokens.go` (`RefreshTokens`, `narrowScope`)
- Create: `backend/internal/oidc/refresh_test.go`
- Modify: `backend/internal/api/oidc_protocol.go` (the `refresh_token` case), `backend/internal/api/oidc_protocol_test.go`

**Interfaces:**
- Consumes: `runGrant`, `findIssued`, `claimGrant`, `replayed`, `grantUser`, `grantableScopes`, `orderScopes`, `isSubset`, `issueTokens`, the Task 17 test helpers.
- Produces: `oidc.RefreshTokens(app core.App, req TokenRequest) (*TokenResponse, *Error)`, unexported `narrowScope(granted []string, requested string) ([]string, error)`. Test helpers `exchangedAt`, `offlineScope`, `refreshRequest`, `type refreshFixture`, `newRefreshFixture(t)`, `(f) refresh(raw, now, overrides)`.

Rulings:
- A refresh re-applies the app's current `allowed_scopes`, so an admin who removes `email` from an app stops it from receiving email on the next refresh.
- An `invalid_scope` or expired refresh does not consume the token. Only a successful rotation sets `rotated_at`.

- [ ] **Step 1: Write the failing tests**

`backend/internal/oidc/refresh_test.go`:

```go
package oidc

import (
	"net/url"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

// exchangedAt is when exchangedTokens issues tokens.
var exchangedAt = testNow.Add(time.Second)

var offlineScope = map[string]string{"scope": "openid email offline_access"}

func refreshRequest(client *core.Record, box *cryptobox.Box, raw string, now time.Time, overrides map[string]string) TokenRequest {
	form := url.Values{"grant_type": {GrantTypeRefreshToken}, "refresh_token": {raw}}
	applyOverrides(form, overrides)
	return TokenRequest{Client: client, Form: form, Issuer: testIssuer, Box: box, Now: now}
}

// refreshFixture is a refresh-enabled app and a User holding tokens with
// offline_access, issued at exchangedAt.
type refreshFixture struct {
	app    core.App
	box    *cryptobox.Box
	client *core.Record
	user   *core.Record
	tokens *TokenResponse
}

func newRefreshFixture(t *testing.T) refreshFixture {
	t.Helper()
	f := refreshFixture{app: newTestApp(t), box: newTestBox(t, testMasterKey)}
	f.client, _ = newTestClient(t, f.app, map[string]any{"refresh_enabled": true})
	f.user = newTestUser(t, f.app, "refresh@example.com")
	f.tokens = exchangedTokens(t, f.app, f.box, f.client, f.user, offlineScope)
	return f
}

func (f refreshFixture) refresh(raw string, now time.Time, overrides map[string]string) (*TokenResponse, *Error) {
	return RefreshTokens(f.app, refreshRequest(f.client, f.box, raw, now, overrides))
}

func TestRefreshRotatesTheRefreshToken(t *testing.T) {
	f := newRefreshFixture(t)
	now := exchangedAt.Add(time.Hour)
	next, oauthErr := f.refresh(f.tokens.RefreshToken, now, nil)
	if oauthErr != nil {
		t.Fatalf("refresh: %v", oauthErr)
	}
	if next.RefreshToken == "" || next.RefreshToken == f.tokens.RefreshToken || next.AccessToken == f.tokens.AccessToken ||
		next.Scope != "openid email offline_access" {
		t.Fatalf("response = %+v", next)
	}
	old := tokenRecord(t, f.app, f.tokens.RefreshToken)
	if old.GetDateTime("rotated_at").IsZero() || tokenRecord(t, f.app, next.RefreshToken).GetString("family") != old.GetString("family") {
		t.Fatalf("old refresh token = %v", old.PublicExport())
	}
	claims := parseIDToken(t, f.app, f.box, f.client, next.IDToken, now)
	if claims["auth_time"] != float64(testNow.Add(-time.Minute).Unix()) || claims["iat"] != float64(now.Unix()) {
		t.Fatalf("claims = %v", claims)
	}
	if _, hasNonce := claims["nonce"]; hasNonce {
		t.Fatal("a refreshed ID token carries the original nonce")
	}
}

func TestRefreshReplayRevokesTheWholeFamily(t *testing.T) {
	f := newRefreshFixture(t)
	other := exchangedTokens(t, f.app, f.box, f.client, f.user, offlineScope)
	now := exchangedAt.Add(time.Hour)
	next, oauthErr := f.refresh(f.tokens.RefreshToken, now, nil)
	if oauthErr != nil {
		t.Fatalf("refresh: %v", oauthErr)
	}

	if _, oauthErr := f.refresh(f.tokens.RefreshToken, now, nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("replay: %v", oauthErr)
	}
	for name, raw := range map[string]string{
		"first access token":  f.tokens.AccessToken,
		"second access token": next.AccessToken,
		"rotated-in refresh":  next.RefreshToken,
	} {
		if !tokenRevoked(t, f.app, raw) {
			t.Errorf("%s still works after the replay", name)
		}
	}
	if _, oauthErr := f.refresh(next.RefreshToken, now, nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("refresh after the replay: %v", oauthErr)
	}
	if tokenRevoked(t, f.app, other.AccessToken) || tokenRevoked(t, f.app, other.RefreshToken) {
		t.Fatal("the replay revoked another family")
	}
}

func TestRefreshTokensExpire(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		f := newRefreshFixture(t)
		if _, oauthErr := f.refresh(f.tokens.RefreshToken, exchangedAt.Add(RefreshTokenIdleTTL), nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
			t.Fatalf("idle refresh token: %v", oauthErr)
		}
	})
	t.Run("absolute", func(t *testing.T) {
		f := newRefreshFixture(t)
		raw := f.tokens.RefreshToken
		step := RefreshTokenIdleTTL - 24*time.Hour
		for now := exchangedAt.Add(step); now.Before(exchangedAt.Add(RefreshTokenMaxTTL)); now = now.Add(step) {
			next, oauthErr := f.refresh(raw, now, nil)
			if oauthErr != nil {
				t.Fatalf("refresh after %v: %v", now.Sub(exchangedAt), oauthErr)
			}
			raw = next.RefreshToken
		}
		if expires := tokenRecord(t, f.app, raw).GetDateTime("expires_at").Time(); !expires.Equal(exchangedAt.Add(RefreshTokenMaxTTL)) {
			t.Fatalf("last refresh token expires at %v, want the absolute cap", expires)
		}
		if _, oauthErr := f.refresh(raw, exchangedAt.Add(RefreshTokenMaxTTL), nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
			t.Fatalf("refresh past the cap: %v", oauthErr)
		}
	})
}

func TestRefreshScopeMayOnlyNarrow(t *testing.T) {
	f := newRefreshFixture(t)
	now := exchangedAt.Add(time.Hour)
	for _, scope := range []string{"openid email profile", "email offline_access"} {
		if _, oauthErr := f.refresh(f.tokens.RefreshToken, now, map[string]string{"scope": scope}); oauthErr == nil || oauthErr.Code != InvalidScope {
			t.Errorf("scope %q: %v, want invalid_scope", scope, oauthErr)
		}
	}

	narrowed, oauthErr := f.refresh(f.tokens.RefreshToken, now, map[string]string{"scope": "openid"})
	if oauthErr != nil || narrowed.Scope != "openid" || narrowed.RefreshToken == "" {
		t.Fatalf("narrowed = %+v, %v", narrowed, oauthErr)
	}
	if claims := parseIDToken(t, f.app, f.box, f.client, narrowed.IDToken, now); claims["email"] != nil {
		t.Fatalf("narrowed ID token = %v", claims)
	}
	full, oauthErr := f.refresh(narrowed.RefreshToken, now, nil)
	if oauthErr != nil || full.Scope != "openid email offline_access" {
		t.Fatalf("the refresh token lost the granted scope: %+v, %v", full, oauthErr)
	}
}

func TestRefreshRejectsAccessTokens(t *testing.T) {
	f := newRefreshFixture(t)
	if _, oauthErr := f.refresh(f.tokens.AccessToken, exchangedAt, nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
		t.Fatalf("access token as refresh token: %v", oauthErr)
	}
	if tokenRevoked(t, f.app, f.tokens.AccessToken) || tokenRevoked(t, f.app, f.tokens.RefreshToken) {
		t.Fatal("presenting an access token revoked the family")
	}
}

func TestRefreshStopsWhenTheAppOrUserCannotContinue(t *testing.T) {
	cases := []struct {
		name   string
		record func(refreshFixture) *core.Record
		field  string
		value  any
	}{
		{"refresh turned off", func(f refreshFixture) *core.Record { return f.client }, "refresh_enabled", false},
		{"app disabled", func(f refreshFixture) *core.Record { return f.client }, "enabled", false},
		{"user disabled", func(f refreshFixture) *core.Record { return f.user }, "status", "disabled"},
		{"user unverified", func(f refreshFixture) *core.Record { return f.user }, "verified", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			rec := c.record(f)
			rec.Set(c.field, c.value)
			if err := f.app.Save(rec); err != nil {
				t.Fatalf("save: %v", err)
			}
			if _, oauthErr := f.refresh(f.tokens.RefreshToken, exchangedAt.Add(time.Hour), nil); oauthErr == nil || oauthErr.Code != InvalidGrant {
				t.Fatalf("refresh: %v, want invalid_grant", oauthErr)
			}
		})
	}
}
```

Append to `backend/internal/api/oidc_protocol_test.go`:

```go
func TestOIDCRefreshThroughTheTokenEndpoint(t *testing.T) {
	env := newOIDCTestEnv(t, map[string]any{"refresh_enabled": true})
	first := env.tokens(map[string]string{"scope": "openid email offline_access"})
	refresh := func(raw string) *httptest.ResponseRecorder {
		return env.postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {raw}})
	}

	res := refresh(first.RefreshToken)
	next := decodeBody[oidc.TokenResponse](t, res)
	if res.Code != http.StatusOK || next.RefreshToken == "" || next.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh = %d %s", res.Code, res.Body.String())
	}
	if claims := env.verifyIDToken(next.IDToken); claims["sub"] != env.user.Id {
		t.Fatalf("refreshed claims = %v", claims)
	}

	res = refresh(first.RefreshToken)
	if body := decodeBody[oauthErrorResponse](t, res); res.Code != http.StatusBadRequest || body.Error != "invalid_grant" {
		t.Fatalf("replay = %d %s", res.Code, res.Body.String())
	}
	if res := refresh(next.RefreshToken); res.Code != http.StatusBadRequest {
		t.Fatalf("the replay left the family usable: %d", res.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'Refresh'; go test ./internal/api -run 'OIDCRefresh'`
Expected: the oidc package fails to compile with `undefined: RefreshTokens`. The api test fails with `unsupported_grant_type`.

- [ ] **Step 3: Implement rotation**

Append to `backend/internal/oidc/tokens.go`:

```go
// RefreshTokens rotates a refresh token (RFC 6749 6, RFC 9700 4.14.2). The
// presented token stops working, and presenting it again revokes the family.
func RefreshTokens(app core.App, req TokenRequest) (*TokenResponse, *Error) {
	raw := req.Form.Get("refresh_token")
	if raw == "" {
		return nil, NewError(InvalidRequest, "refresh_token is required")
	}
	return runGrant(app, func(tx core.App) (*TokenResponse, error) {
		rec, grant, err := findIssued(tx, TokensCollection, "token_hash = {:hash} && kind = 'refresh'", raw, req.Client)
		if err != nil {
			return nil, err
		}
		if !rec.GetDateTime("rotated_at").IsZero() {
			return nil, replayed(tx, rec, req.Now)
		}
		if !rec.GetDateTime("revoked_at").IsZero() || !req.Now.Before(rec.GetDateTime("expires_at").Time()) ||
			!req.Client.GetBool("refresh_enabled") {
			return nil, errInvalidGrant
		}
		user, err := grantUser(tx, grant, req.Client)
		if err != nil {
			return nil, err
		}
		granted := grantableScopes(req.Client, strings.Fields(rec.GetString("scope")))
		scope, err := narrowScope(granted, req.Form.Get("scope"))
		if err != nil {
			return nil, err
		}
		if err := claimGrant(tx, TokensCollection, rec, "rotated_at", req.Now); err != nil {
			return nil, err
		}
		return issueTokens(tx, req, issuance{
			grant:        grant,
			user:         user,
			family:       rec.GetString("family"),
			scope:        scope,
			refreshScope: granted,
			authTime:     rec.GetDateTime("auth_time").Time(),
			refreshUntil: rec.GetDateTime("absolute_expires_at").Time(),
		})
	})
}

// narrowScope applies the optional scope of a refresh request. It may only
// drop scopes, and openid stays because the response carries an ID token.
func narrowScope(granted []string, requested string) ([]string, error) {
	if requested == "" {
		return granted, nil
	}
	scope := strings.Fields(requested)
	if !slices.Contains(scope, ScopeOpenID) || !isSubset(scope, granted) {
		return nil, NewError(InvalidScope, "a refresh may only narrow the granted scope and must keep openid")
	}
	return orderScopes(scope), nil
}
```

In `oidcToken` in `backend/internal/api/oidc_protocol.go`, add after the `authorization_code` case:

```go
	case oidc.GrantTypeRefreshToken:
		resp, oauthErr = oidc.RefreshTokens(h.app, req)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test -timeout 120s ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/oidc backend/internal/api
git commit -m "feat(backend): rotate oidc refresh tokens" -m "Every refresh issues a new refresh token in the same family and marks the old one rotated. Presenting a rotated token revokes the whole family. The idle lifetime slides by 30 days up to the 90-day cap, a narrower scope applies only to the new access and ID tokens, and the refreshed ID token keeps the original auth_time without a nonce."
```

### Task 19: Userinfo and revocation

**Files:**
- Create: `backend/internal/oidc/access.go`, `backend/internal/oidc/access_test.go`
- Modify: `backend/internal/api/oidc_protocol.go`, `backend/internal/api/oidc_protocol_test.go`

**Interfaces:**
- Consumes: `errInvalidGrant`, `grantUser`, `revokeFamily`, `claimOnce`, `UserClaims`, `oidcAuthenticatedClient`, `writeOAuthError`, `setNoStore`, the Task 17 and 18 test helpers.
- Produces:
  - `oidc.ErrInvalidToken`.
  - `oidc.LookupAccessToken(app core.App, raw string, now time.Time) (*core.Record, []string, error)`, which returns the User and the token's scope.
  - `oidc.RevokeToken(app core.App, client *core.Record, raw string, now time.Time) error`.
  - API side: `(h) oidcUserInfo`, `(h) oidcRevoke`, `oidcBearerToken(r) (string, bool)`.

Rulings:
- Userinfo reads the access token from the `Authorization: Bearer` header only. RFC 6750 makes the form body and query methods optional, and the query method leaks tokens into logs.
- Revocation of a token that belongs to another app is ignored with 200, like an unknown token, so the endpoint never confirms that a token exists. RFC 7009 2.1 allows refusing it, but a refusal tells the caller the token is real.
- PocketBase's global auth middleware also reads the `Authorization` header. An opaque access token or Basic credentials fail its JWT parsing, and it continues as a guest. Step 5 checks this with a test instead of assuming it.

- [ ] **Step 1: Write the failing tests**

`backend/internal/oidc/access_test.go`:

```go
package oidc

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestLookupAccessTokenReturnsTheUserAndScope(t *testing.T) {
	f := newRefreshFixture(t)
	user, scope, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt)
	if err != nil || user.Id != f.user.Id || !slices.Equal(scope, []string{"openid", "email", "offline_access"}) {
		t.Fatalf("lookup = %v, %v, %v", user, scope, err)
	}
	claims := UserClaims(user, scope)
	if claims["sub"] != f.user.Id || claims["email"] != "refresh@example.com" || claims["email_verified"] != true {
		t.Fatalf("claims = %v", claims)
	}
	if claims := UserClaims(user, []string{"openid"}); len(claims) != 1 {
		t.Fatalf("claims without the email scope = %v", claims)
	}
}

func TestLookupAccessTokenRejectsRefreshTokens(t *testing.T) {
	f := newRefreshFixture(t)
	if _, _, err := LookupAccessToken(f.app, f.tokens.RefreshToken, exchangedAt); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh token at userinfo: %v", err)
	}
}

func TestLookupAccessTokenRejectsUnusableTokens(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		f := newRefreshFixture(t)
		if _, _, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt.Add(AccessTokenTTL)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expired: %v", err)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		f := newRefreshFixture(t)
		if _, _, err := LookupAccessToken(f.app, "unknown", exchangedAt); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("unknown: %v", err)
		}
	})
	t.Run("revoked", func(t *testing.T) {
		f := newRefreshFixture(t)
		if err := RevokeToken(f.app, f.client, f.tokens.AccessToken, exchangedAt); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		if _, _, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("revoked: %v", err)
		}
	})
	for name, change := range map[string]func(refreshFixture){
		"user disabled": func(f refreshFixture) { f.user.Set("status", "disabled") },
		"app disabled":  func(f refreshFixture) { f.client.Set("enabled", false) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newRefreshFixture(t)
			change(f)
			if err := f.app.Save(f.user); err != nil {
				t.Fatalf("save user: %v", err)
			}
			if err := f.app.Save(f.client); err != nil {
				t.Fatalf("save client: %v", err)
			}
			if _, _, err := LookupAccessToken(f.app, f.tokens.AccessToken, exchangedAt); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("%s: %v", name, err)
			}
		})
	}
}

func TestRevokeToken(t *testing.T) {
	f := newRefreshFixture(t)
	other := exchangedTokens(t, f.app, f.box, f.client, f.user, offlineScope)
	stranger, _ := newTestClient(t, f.app, nil)

	for name, raw := range map[string]string{"unknown token": "unknown", "another app's token": f.tokens.AccessToken} {
		if err := RevokeToken(f.app, stranger, raw, exchangedAt); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if tokenRevoked(t, f.app, f.tokens.AccessToken) {
		t.Fatal("another app revoked the token")
	}

	if err := RevokeToken(f.app, f.client, other.AccessToken, exchangedAt); err != nil {
		t.Fatalf("revoke access token: %v", err)
	}
	if !tokenRevoked(t, f.app, other.AccessToken) || tokenRevoked(t, f.app, other.RefreshToken) {
		t.Fatal("revoking an access token must leave its refresh token")
	}

	if err := RevokeToken(f.app, f.client, f.tokens.RefreshToken, exchangedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke refresh token: %v", err)
	}
	if !tokenRevoked(t, f.app, f.tokens.RefreshToken) || !tokenRevoked(t, f.app, f.tokens.AccessToken) {
		t.Fatal("revoking a refresh token must end its family")
	}
}
```

Append to `backend/internal/api/oidc_protocol_test.go`:

```go
func (env *oidcTestEnv) userInfo(authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/oidc/userinfo", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return env.do(req)
}

func TestOIDCUserInfoAndRevocation(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	tokens := env.tokens(nil)

	res := env.userInfo("Bearer " + tokens.AccessToken)
	claims := decodeBody[map[string]any](t, res)
	if res.Code != http.StatusOK || claims["sub"] != env.user.Id || claims["email"] != "oidc-user@example.com" ||
		claims["email_verified"] != true || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("userinfo = %d %s", res.Code, res.Body.String())
	}
	post := httptest.NewRequest(http.MethodPost, "/api/oidc/userinfo", nil)
	post.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	if res := env.do(post); res.Code != http.StatusOK {
		t.Fatalf("POST userinfo = %d", res.Code)
	}

	if res := env.userInfo(""); res.Code != http.StatusUnauthorized || res.Header().Get("WWW-Authenticate") != `Bearer realm="oidc"` {
		t.Fatalf("without a token = %d %v", res.Code, res.Header())
	}
	if res := env.userInfo(env.session); res.Code != http.StatusUnauthorized {
		t.Fatalf("a panel session at userinfo = %d", res.Code)
	}

	if res := env.postClientForm("/api/oidc/revoke", url.Values{}, env.secret); res.Code != http.StatusBadRequest {
		t.Fatalf("revoke without a token = %d", res.Code)
	}
	if res := env.postClientForm("/api/oidc/revoke", url.Values{"token": {tokens.AccessToken}}, env.secret); res.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", res.Code, res.Body.String())
	}
	res = env.userInfo("Bearer " + tokens.AccessToken)
	if res.Code != http.StatusUnauthorized || res.Header().Get("WWW-Authenticate") != `Bearer realm="oidc", error="invalid_token"` {
		t.Fatalf("revoked token = %d %v", res.Code, res.Header())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'LookupAccessToken|RevokeToken'; go test ./internal/api -run 'OIDCUserInfo'`
Expected: the oidc package fails to compile with `undefined: LookupAccessToken`. The api test fails with 404 at `/api/oidc/userinfo`.

- [ ] **Step 3: Implement the lookup and revocation**

`backend/internal/oidc/access.go`:

```go
package oidc

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// ErrInvalidToken means the access token is unknown, expired, revoked or no
// longer backed by a usable grant.
var ErrInvalidToken = errors.New("invalid access token")

// LookupAccessToken returns the User and scope behind an access token. A
// refresh token is never accepted here.
func LookupAccessToken(app core.App, raw string, now time.Time) (*core.Record, []string, error) {
	if raw == "" {
		return nil, nil, ErrInvalidToken
	}
	rec, err := app.FindFirstRecordByFilter(TokensCollection,
		"token_hash = {:hash} && kind = 'access' && revoked_at = '' && expires_at > {:now}",
		dbx.Params{"hash": token.Sha256Hex(raw), "now": dbTime(now)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrInvalidToken
	}
	if err != nil {
		return nil, nil, err
	}
	grant, err := app.FindRecordById(GrantsCollection, rec.GetString("grant"))
	if err != nil {
		return nil, nil, err
	}
	client, err := app.FindRecordById(ClientsCollection, grant.GetString("client"))
	if err != nil {
		return nil, nil, err
	}
	user, err := grantUser(app, grant, client)
	if errors.Is(err, errInvalidGrant) {
		return nil, nil, ErrInvalidToken
	}
	if err != nil {
		return nil, nil, err
	}
	return user, strings.Fields(rec.GetString("scope")), nil
}

// RevokeToken implements RFC 7009. Unknown tokens and tokens of other apps
// are ignored, so the answer never confirms that a token exists. Revoking a
// refresh token ends its whole family.
func RevokeToken(app core.App, client *core.Record, raw string, now time.Time) error {
	return app.RunInTransaction(func(tx core.App) error {
		rec, err := tx.FindFirstRecordByFilter(TokensCollection, "token_hash = {:hash}", dbx.Params{"hash": token.Sha256Hex(raw)})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		grant, err := tx.FindRecordById(GrantsCollection, rec.GetString("grant"))
		if err != nil {
			return err
		}
		if grant.GetString("client") != client.Id {
			return nil
		}
		if rec.GetString("kind") == tokenKindRefresh {
			return revokeFamily(tx, rec.GetString("family"), now)
		}
		_, err = claimOnce(tx, TokensCollection, rec.Id, "revoked_at", now)
		return err
	})
}
```

- [ ] **Step 4: Add the endpoints**

In `backend/internal/api/oidc_protocol.go`, add `"errors"` and `"strings"` to the imports, register the routes in `bindOIDCRoutes`:

```go
	g.GET(oidc.UserInfoPath, h.oidcUserInfo)
	g.POST(oidc.UserInfoPath, h.oidcUserInfo)
	g.POST(oidc.RevocationPath, h.oidcRevoke)
```

and append:

```go
// oidcUserInfo returns the claims the access token's scope releases
// (OIDC Core 5.3). Errors follow RFC 6750 3.
func (h *Handlers) oidcUserInfo(e *core.RequestEvent) error {
	setNoStore(e)
	now := time.Now()
	if !h.oidcLimits.backchannel.allow(e.RealIP(), now) {
		return writeOAuthError(e, oidc.NewError(oidc.TemporarilyUnavailable, "too many requests"))
	}
	raw, found := oidcBearerToken(e.Request)
	if !found {
		e.Response.Header().Set("WWW-Authenticate", `Bearer realm="oidc"`)
		return e.NoContent(http.StatusUnauthorized)
	}
	user, scope, err := oidc.LookupAccessToken(h.app, raw, now)
	if errors.Is(err, oidc.ErrInvalidToken) {
		e.Response.Header().Set("WWW-Authenticate", `Bearer realm="oidc", error="invalid_token"`)
		return e.NoContent(http.StatusUnauthorized)
	}
	if err != nil {
		return writeOAuthError(e, oidc.WrapServerError(err))
	}
	return e.JSON(http.StatusOK, oidc.UserClaims(user, scope))
}

// oidcRevoke implements RFC 7009. It answers 200 for unknown tokens too.
func (h *Handlers) oidcRevoke(e *core.RequestEvent) error {
	setNoStore(e)
	client, oauthErr := h.oidcAuthenticatedClient(e)
	if oauthErr != nil {
		return writeOAuthError(e, oauthErr)
	}
	raw := e.Request.PostForm.Get("token")
	if raw == "" {
		return writeOAuthError(e, oidc.NewError(oidc.InvalidRequest, "token is required"))
	}
	if err := oidc.RevokeToken(h.app, client, raw, time.Now()); err != nil {
		return writeOAuthError(e, oidc.WrapServerError(err))
	}
	return e.NoContent(http.StatusOK)
}

// oidcBearerToken reads an RFC 6750 bearer token from the Authorization
// header. A panel session without the Bearer scheme does not count.
func oidcBearerToken(r *http.Request) (string, bool) {
	scheme, raw, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return "", false
	}
	return raw, true
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test -timeout 120s ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS. `TestOIDCUserInfoAndRevocation` sends an opaque bearer token and a Basic header through the real router, so it also proves PocketBase's auth middleware lets both through as a guest. If it answers 401 from PocketBase instead of the handler, read `apis.loadAuthToken` before changing anything.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/oidc backend/internal/api
git commit -m "feat(backend): add oidc userinfo and revocation" -m "GET or POST /api/oidc/userinfo takes a Bearer access token and returns sub, plus email and email_verified with the email scope. Refresh tokens, expired or revoked tokens, and tokens of a User or app that can no longer sign in get 401 invalid_token. POST /api/oidc/revoke revokes one access token or a refresh token's whole family and answers 200 for tokens it does not know."
```

### Task 20: Revoke grants when a User stops being usable

**Files:**
- Modify: `backend/internal/oidc/grants.go` (`RevokeUserGrants`, reason constants)
- Create: `backend/internal/oidc/grants_test.go`
- Create: `backend/internal/api/oidc_user_hook.go`, `backend/internal/api/oidc_user_hook_test.go`
- Modify: `backend/internal/api/api.go` (`h.bindOIDCGrantRevocation()`)

**Interfaces:**
- Consumes: `dbTime`, `grantUser`, `LookupAccessToken`, the Task 17 to 19 test helpers.
- Produces: `oidc.RevokeReasonUserDisabled`, `oidc.RevokeReasonUserUnverified`, `oidc.RevokeUserGrants(app core.App, userID, reason string, now time.Time) error`, `(h) bindOIDCGrantRevocation()`, `oidcRevokeReason(before, after *core.Record) string`.

Rulings:
- `grantUser` already rejects an unusable User on every request. The hook makes the revocation permanent: re-enabling the User does not bring old tokens or the remembered consent back.
- The hook fires only when a save turns a usable User unusable. Saves of Users who were already unusable cost nothing, and the collector's per-poll writes never reach it because they do not change `status` or `verified`.

- [ ] **Step 1: Write the failing tests**

`backend/internal/oidc/grants_test.go`:

```go
package oidc

import (
	"testing"
	"time"
)

func TestRevokeUserGrantsLeavesOtherUsers(t *testing.T) {
	f := newRefreshFixture(t)
	bystander := newTestUser(t, f.app, "bystander@example.com")
	kept := exchangedTokens(t, f.app, f.box, f.client, bystander, offlineScope)

	if err := RevokeUserGrants(f.app, f.user.Id, RevokeReasonUserDisabled, exchangedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	grants, err := f.app.FindAllRecords(GrantsCollection)
	if err != nil || len(grants) != 2 {
		t.Fatalf("grants = %d, %v", len(grants), err)
	}
	for _, grant := range grants {
		revoked := !grant.GetDateTime("revoked_at").IsZero()
		if mine := grant.GetString("user") == f.user.Id; revoked != mine ||
			(mine && grant.GetString("revoke_reason") != RevokeReasonUserDisabled) {
			t.Errorf("grant of %s = %v", grant.GetString("user"), grant.PublicExport())
		}
	}
	if _, _, err := LookupAccessToken(f.app, kept.AccessToken, exchangedAt); err != nil {
		t.Fatalf("the bystander lost access: %v", err)
	}
}
```

`backend/internal/api/oidc_user_hook_test.go`:

```go
package api

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"hysterical-panel/internal/oidc"
)

func TestDisablingUserRevokesGrantsOnAnySave(t *testing.T) {
	cases := []struct {
		field  string
		value  any
		reason string
	}{
		{"status", "disabled", oidc.RevokeReasonUserDisabled},
		{"verified", false, oidc.RevokeReasonUserUnverified},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			env := newOIDCTestEnv(t, map[string]any{"refresh_enabled": true})
			tokens := env.tokens(map[string]string{"scope": "openid email offline_access"})
			activeGrants := func() int {
				t.Helper()
				grants, err := env.app.FindRecordsByFilter(oidc.GrantsCollection, "revoked_at = ''", "", 0, 0)
				if err != nil {
					t.Fatalf("grants: %v", err)
				}
				return len(grants)
			}

			user, err := env.app.FindRecordById("users", env.user.Id)
			if err != nil {
				t.Fatalf("load user: %v", err)
			}
			user.Set("last_connected_at", time.Now().UTC())
			if err := env.app.Save(user); err != nil || activeGrants() != 1 {
				t.Fatalf("an unrelated save revoked the grant (err %v)", err)
			}

			user.Set(c.field, c.value)
			if err := env.app.Save(user); err != nil {
				t.Fatalf("save: %v", err)
			}
			grants, err := env.app.FindAllRecords(oidc.GrantsCollection)
			if err != nil || len(grants) != 1 || grants[0].GetString("revoke_reason") != c.reason {
				t.Fatalf("grants after the save = %d, %v", len(grants), err)
			}

			user.Set("status", "active")
			user.Set("verified", true)
			if err := env.app.Save(user); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if res := env.userInfo("Bearer " + tokens.AccessToken); res.Code != http.StatusUnauthorized {
				t.Fatalf("userinfo after re-enabling = %d", res.Code)
			}
			res := env.postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens.RefreshToken}})
			if res.Code != http.StatusBadRequest {
				t.Fatalf("refresh after re-enabling = %d", res.Code)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/oidc -run 'RevokeUserGrants'; go test ./internal/api -run 'DisablingUserRevokesGrants'`
Expected: the oidc package fails to compile with `undefined: RevokeUserGrants`. The api package fails to compile with `undefined: oidc.RevokeReasonUserDisabled`.

- [ ] **Step 3: Implement the revocation and the hook**

Append to `backend/internal/oidc/grants.go`:

```go
// Reasons stored in oidc_grants.revoke_reason.
const (
	RevokeReasonUserDisabled   = "user_disabled"
	RevokeReasonUserUnverified = "user_unverified"
)

// RevokeUserGrants ends every remembered consent of a User. Their tokens
// stop working because every lookup checks the grant.
func RevokeUserGrants(app core.App, userID, reason string, now time.Time) error {
	_, err := app.NonconcurrentDB().Update(GrantsCollection,
		dbx.Params{"revoked_at": dbTime(now), "revoke_reason": reason},
		dbx.NewExp("[[user]] = {:user} AND [[revoked_at]] = ''", dbx.Params{"user": userID}),
	).Execute()
	return err
}
```

`backend/internal/api/oidc_user_hook.go`:

```go
package api

import (
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

// bindOIDCGrantRevocation revokes a User's Connected App grants when a save
// makes the User unusable. It hooks the record save rather than the panel
// handler, so edits made in /_/ count too (ADR-0010). A failure is logged
// and never blocks the save, because every token lookup checks the User
// again anyway.
func (h *Handlers) bindOIDCGrantRevocation() {
	h.app.OnRecordUpdate("users").BindFunc(func(e *core.RecordEvent) error {
		reason := oidcRevokeReason(e.Record.Original(), e.Record)
		if err := e.Next(); err != nil || reason == "" {
			return err
		}
		if err := oidc.RevokeUserGrants(e.App, e.Record.Id, reason, time.Now()); err != nil {
			log.Printf("[oidc] revoke grants of user %s: %v", e.Record.Id, err)
		}
		return nil
	})
}

// oidcRevokeReason names why a save ends a User's grants, or returns "" when
// the User was already unusable or still is usable.
func oidcRevokeReason(before, after *core.Record) string {
	if before.GetString("status") != "active" || !before.GetBool("verified") {
		return ""
	}
	switch {
	case after.GetString("status") != "active":
		return oidc.RevokeReasonUserDisabled
	case !after.GetBool("verified"):
		return oidc.RevokeReasonUserUnverified
	}
	return ""
}
```

In `backend/internal/api/api.go`, after `h.bindAuthGate()` add:

```go
	h.bindOIDCGrantRevocation()
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && gofmt -l . ; go vet ./... && go test -timeout 120s ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS. If the revocation never fires, check that `Record.Original()` reflects the last saved state after `app.Save`. `TestDisablingUserRevokesGrantsOnAnySave` saves the same record three times to cover this.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/oidc backend/internal/api
git commit -m "feat(backend): revoke oidc grants of unusable users" -m "A users record hook revokes every Connected App grant when a save disables or unverifies a User, including saves made in /_/. Re-enabling the User does not bring the old tokens or the remembered consent back."
```

### Layer 3 exit check

- [ ] **Step 1: Run every gate**

Run: `cd backend && gofmt -l . ; go build ./... && go vet ./... && go test -timeout 300s ./...`
Expected: no gofmt output, PASS.

Run: `cd frontend && pnpm api:sync && pnpm typecheck && pnpm i18n:check && pnpm build`
Expected: PASS.

- [ ] **Step 2: Run the flow against a live server**

Use `127.0.0.1:8091` and a throwaway data directory. Never touch the dev servers on 3000 and 8090. The https base URLs only satisfy the availability check; every request goes to `http://127.0.0.1:8091`.

1. Start the server in the background with `PANEL_MASTER_KEY=live-check PANEL_BACKEND_URL_BASE=https://api.example.test PANEL_FRONTEND_URL_BASE=https://panel.example.test go run . serve --http 127.0.0.1:8091 --dir /tmp/hp-oidc-live`.
2. Create a superuser with `go run . superuser upsert`, then create an admin User through `/api/collections/users/records` as the superuser, and sign in as that User with `auth-with-password` to get a panel session.
3. `PATCH /api/panel/settings` with `oidc_enabled: true`, then `POST /api/panel/oidc-clients` with `refresh_enabled: true` and a redirect URI, and keep the client ID and secret.
4. Build a PKCE pair with `openssl`, call `/api/oidc/authorize`, read the handle from the 303 `Location`, and approve it with the session.
5. Exchange the code with `curl -u`, call userinfo with the access token, refresh once, revoke the refresh token, and confirm that userinfo and a second refresh then fail.
6. Stop the server and delete `/tmp/hp-oidc-live`.

Expected: every step answers as the tests predict.

- [ ] **Step 3: Review the layer**

Dispatch a `review:code-reviewer` agent on `git diff oidc/2-connected-apps...oidc/3-protocol` with this plan's Layer 3 decisions and Review Focus. Fix every Critical and Important finding in its own commit, then rerun Step 1.

- [ ] **Step 4: Update the ledger and stop**

Record the layer 3 commits, test counts and review outcome in `.superpowers/sdd/2026-10-12-oidc-provider/progress.md`. Stop for the user's review before starting layer 4.


---

## Layer 4: `oidc/4-consent-ui`

Branch `oidc/4-consent-ui` from `oidc/3-protocol` (including the review fixes `398b94b`, `23f04c3` and `d7c781a`).

### Decisions for this layer

- **`prompt=none` hint.** The consent page cannot read a request before the visitor signs in, but a `prompt=none` request must end with `login_required` at once instead of showing the sign-in form. `/api/oidc/authorize` therefore appends `&prompt=none` to the consent page URL for those requests. The hint only chooses between the abort call and the sign-in form. The abort endpoint decides the error from the stored request, so a forged hint cannot change what the app receives.
- **Authorized apps API.** `GET /api/panel/users/{id}/oidc-grants` lists a User's unrevoked grants, and `DELETE /api/panel/users/{id}/oidc-grants/{grantId}` revokes one with reason `user_revoked`. Both use `requireActiveAdminOrSelf`, like oauth-links, and stay ungated by the provider switch, so a User can still revoke after an admin turns the provider off. Revoking ends every token under the grant, because each token lookup checks the grant.
- **Authorized apps section.** The settings page shows it only when the User has at least one grant. Users who never authorized an app see nothing new. The admin user detail page does not get this section in this layer.
- **`/login` parameters.** `redirect` accepts only a same-origin path, checked in `validateSearch` by `safeRedirectPath`. `reauth=true` shows the form to a signed-in User. Password, passkey and OAuth2 sign-in all return to `redirect`. OAuth2 carries it through the existing sessionStorage entry as `returnTo` and checks it again in the callback.
- **Switch account** clears the panel cookie in the browser and opens `/login?redirect=<consent page>`. The panel has no server-side sign-out.
- **Frame and referrer headers.** The consent route sets `Content-Security-Policy: frame-ancestors 'none'`, `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer` through TanStack Start's route `headers` option. They apply to the server-rendered document. A relying party always sends the browser there with a full page load.
- **Exit check RP.** The plan table names a next-auth dev app. The exit check uses a 100-line `openid-client` v6 script instead. next-auth 4 runs on `openid-client` v5, so the protocol checks are the same library family, and no Next.js app has to be scaffolded. `openid-client` validates the ID token signature, `iss`, `aud`, `nonce`, `at_hash` and the `iss` authorization response parameter.
- **Not in this layer:** a cancel link on the sign-in page (the abort endpoint supports it, but closing the tab is enough), and Management API lookup by `id` (layer 5).

### File structure

| File | Responsibility |
|---|---|
| `backend/internal/oidc/grants.go` | adds `ActiveGrant`, `ListActiveGrants`, `RevokeGrant`, `ErrGrantNotFound`, `RevokeReasonUserRevoked` |
| `backend/internal/api/oidc_grants.go` | list and revoke handlers, `publicOIDCGrant` |
| `backend/internal/api/oidc_protocol.go` | `prompt=none` hint on the consent page URL |
| `backend/internal/api/dto.go`, `openapi.go`, `api.go` | `OIDCGrant` DTO, two paths, two routes |
| `frontend/src/api/queries.ts` | consent and grant fetchers, query options and keys |
| `frontend/src/lib/api-error.ts` | localized consent and grant errors |
| `frontend/src/lib/safe-redirect.ts` | `safeRedirectPath` |
| `frontend/src/lib/oidc-scopes.ts` | `scopeLabel`, `formatScopes` |
| `frontend/src/api/auth.ts`, `routes/login.tsx`, `routes/oauth-callback.tsx` | `redirect` and `reauth` |
| `frontend/src/routes/oidc/error.tsx` | error page for untrusted requests |
| `frontend/src/routes/oidc/authorize.tsx` | consent page |
| `frontend/src/components/authorized-apps.tsx`, `routes/settings/index.tsx` | authorized apps section |
| `frontend/messages/en.json`, `zh-cn.json` | all layer 4 copy |

### Review Focus

- A `redirect` value that leaves the panel must land on `/`: `//evil.example`, `/\evil.example`, `https://evil.example`, `/\t/evil.example`, `/.//evil.example`, `javascript:alert(1)`. Covered in Task 24 Step 2.
- A `prompt=none` request from a browser without a panel session must go back to the app with `login_required` and never show the sign-in form. Covered in Task 22 (`TestOIDCAuthorizeTellsTheConsentPageAboutPromptNone`) and exit check step 4.
- Opening the consent page again after deciding, with the back button or a reload, must show the expired message and not loop or approve twice. Covered in exit check step 4.
- Re-authentication must come back to the consent page whichever sign-in method the User picks. Password and passkey use `redirect` directly. OAuth2 goes through `returnTo` in Task 24, and the exit check covers the password path.
- Revoking an app must stop its existing access token and refresh token at once, and another User must not be able to list or revoke it. Covered in Task 21 (`TestOIDCGrantsListAndRevokeThroughThePanel`, `TestOIDCGrantsAreOwnerOrAdminOnly`).

### Task 21: Authorized apps endpoints

**Files:**
- Modify: `backend/internal/oidc/grants.go`
- Create: `backend/internal/api/oidc_grants.go`
- Modify: `backend/internal/api/dto.go`, `backend/internal/api/openapi.go`, `backend/internal/api/api.go`
- Test: `backend/internal/oidc/grants_test.go`, `backend/internal/api/oidc_grants_test.go`, `backend/internal/api/openapi_oidc_test.go`

**Interfaces:**
- Consumes: `upsertGrant`, `grantScopes`, `dbTime`, `GrantsCollection`, `ClientsCollection`, `ScopeOpenID`, `ScopeEmail` (package `oidc`); test helpers `newTestApp`, `newTestClient`, `newTestUser`, `testNow` (package `oidc`), `newOIDCTestEnv`, `env.tokens`, `env.panelGet`, `env.panelRequest`, `env.userInfo`, `env.approveCode`, `newUsersTestRecord`, `issueSessionToken`, `decodeBody` (package `api`).
- Produces: `oidc.ActiveGrant{ID, ClientName string; Scopes []string; Created, LastUsedAt time.Time}`, `oidc.ListActiveGrants(app core.App, userID string) ([]ActiveGrant, error)`, `oidc.RevokeGrant(app core.App, userID, grantID string, now time.Time) error`, `oidc.ErrGrantNotFound`, `oidc.RevokeReasonUserRevoked`. DTO `OIDCGrant{id, client_name, scopes, created, last_used_at?}`. Routes `GET /api/panel/users/{id}/oidc-grants` (`listOIDCGrants`) and `DELETE /api/panel/users/{id}/oidc-grants/{grantId}` (`revokeOIDCGrant`).

- [ ] **Step 1: Write the failing package test**

Append to `backend/internal/oidc/grants_test.go` (add `"errors"` and `"slices"` to its imports if missing):

```go
func TestListAndRevokeActiveGrants(t *testing.T) {
	app := newTestApp(t)
	client, _ := newTestClient(t, app, nil)
	user := newTestUser(t, app, "grants@example.com")
	other := newTestUser(t, app, "other-grants@example.com")
	grant, err := upsertGrant(app, user.Id, client.Id, []string{ScopeOpenID, ScopeEmail})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	otherGrant, err := upsertGrant(app, other.Id, client.Id, []string{ScopeOpenID})
	if err != nil {
		t.Fatalf("other grant: %v", err)
	}

	listed, err := ListActiveGrants(app, user.Id)
	if err != nil || len(listed) != 1 || listed[0].ID != grant.Id || listed[0].ClientName != client.GetString("name") ||
		!slices.Equal(listed[0].Scopes, []string{ScopeOpenID, ScopeEmail}) || listed[0].Created.IsZero() || !listed[0].LastUsedAt.IsZero() {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	if err := RevokeGrant(app, user.Id, otherGrant.Id, testNow); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("revoking another user's grant: %v", err)
	}
	if err := RevokeGrant(app, user.Id, grant.Id, testNow); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := RevokeGrant(app, user.Id, grant.Id, testNow); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
	stored, err := app.FindRecordById(GrantsCollection, grant.Id)
	if err != nil || stored.GetString("revoke_reason") != RevokeReasonUserRevoked || stored.GetDateTime("revoked_at").IsZero() {
		t.Fatalf("stored grant = %v, %v", stored, err)
	}
	if listed, err := ListActiveGrants(app, user.Id); err != nil || len(listed) != 0 {
		t.Fatalf("after revoke = %+v, %v", listed, err)
	}
	if listed, err := ListActiveGrants(app, other.Id); err != nil || len(listed) != 1 {
		t.Fatalf("other user's grants = %+v, %v", listed, err)
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `cd backend && go test ./internal/oidc -run TestListAndRevokeActiveGrants`
Expected: FAIL to compile with `undefined: ListActiveGrants`.

- [ ] **Step 3: Implement listing and revocation**

In `backend/internal/oidc/grants.go`, add `RevokeReasonUserRevoked` to the existing reason constants:

```go
// Reasons stored in oidc_grants.revoke_reason.
const (
	RevokeReasonUserDisabled   = "user_disabled"
	RevokeReasonUserUnverified = "user_unverified"
	RevokeReasonUserRevoked    = "user_revoked"
)
```

Append to the same file:

```go
// ErrGrantNotFound reports a grant that does not exist, is already revoked
// or belongs to another User.
var ErrGrantNotFound = errors.New("connected app grant not found")

// ActiveGrant is a remembered consent as the panel lists it. LastUsedAt is
// zero until the app first receives tokens.
type ActiveGrant struct {
	ID         string
	ClientName string
	Scopes     []string
	Created    time.Time
	LastUsedAt time.Time
}

// ListActiveGrants returns a User's unrevoked grants, newest first.
func ListActiveGrants(app core.App, userID string) ([]ActiveGrant, error) {
	grants, err := app.FindRecordsByFilter(GrantsCollection, "user = {:user} && revoked_at = ''", "-created", 0, 0,
		dbx.Params{"user": userID})
	if err != nil {
		return nil, err
	}
	out := make([]ActiveGrant, 0, len(grants))
	for _, grant := range grants {
		client, err := app.FindRecordById(ClientsCollection, grant.GetString("client"))
		if err != nil {
			return nil, err
		}
		out = append(out, ActiveGrant{
			ID:         grant.Id,
			ClientName: client.GetString("name"),
			Scopes:     grantScopes(grant),
			Created:    grant.GetDateTime("created").Time(),
			LastUsedAt: grant.GetDateTime("last_used_at").Time(),
		})
	}
	return out, nil
}

// RevokeGrant ends one of a User's grants. Every token issued under it stops
// working at once, because each token lookup checks the grant.
func RevokeGrant(app core.App, userID, grantID string, now time.Time) error {
	result, err := app.NonconcurrentDB().Update(GrantsCollection,
		dbx.Params{"revoked_at": dbTime(now), "revoke_reason": RevokeReasonUserRevoked},
		dbx.NewExp("[[id]] = {:id} AND [[user]] = {:user} AND [[revoked_at]] = ''",
			dbx.Params{"id": grantID, "user": userID}),
	).Execute()
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrGrantNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run it to make sure it passes**

Run: `cd backend && go test ./internal/oidc -run TestListAndRevokeActiveGrants`
Expected: PASS.

- [ ] **Step 5: Write the failing route tests**

Create `backend/internal/api/oidc_grants_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestOIDCGrantsListAndRevokeThroughThePanel(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	tokens := env.tokens(nil)
	path := "/api/panel/users/" + env.user.Id + "/oidc-grants"

	res := env.panelGet(path)
	grants := decodeBody[[]OIDCGrant](t, res)
	if res.Code != http.StatusOK || len(grants) != 1 || grants[0].ClientName != "Help center" ||
		!slices.Equal(grants[0].Scopes, []string{"openid", "email"}) || grants[0].Created == "" || grants[0].LastUsedAt == nil {
		t.Fatalf("list = %d %s", res.Code, res.Body.String())
	}

	if res := env.panelRequest(http.MethodDelete, path+"/"+grants[0].ID); res.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", res.Code, res.Body.String())
	}
	if res := env.userInfo("Bearer " + tokens.AccessToken); res.Code != http.StatusUnauthorized {
		t.Fatalf("userinfo after revoke = %d", res.Code)
	}
	if res := env.panelRequest(http.MethodDelete, path+"/"+grants[0].ID); res.Code != http.StatusNotFound {
		t.Fatalf("second revoke = %d %s", res.Code, res.Body.String())
	}
	if res := env.panelGet(path); len(decodeBody[[]OIDCGrant](t, res)) != 0 {
		t.Fatalf("list after revoke = %s", res.Body.String())
	}
}

func TestOIDCGrantsAreOwnerOrAdminOnly(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	env.approveCode(nil)
	other := newUsersTestRecord(t, env.app, "other-oidc@example.com", "OtherOIDCSecret123")
	session, err := issueSessionToken(env.app, other)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	grants := decodeBody[[]OIDCGrant](t, env.panelGet("/api/panel/users/"+env.user.Id+"/oidc-grants"))
	if len(grants) != 1 {
		t.Fatalf("grants = %+v", grants)
	}
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/panel/users/"+env.user.Id+"/oidc-grants", nil),
		httptest.NewRequest(http.MethodDelete, "/api/panel/users/"+env.user.Id+"/oidc-grants/"+grants[0].ID, nil),
		// The other User's own path must not reach a grant that is not theirs.
		httptest.NewRequest(http.MethodDelete, "/api/panel/users/"+other.Id+"/oidc-grants/"+grants[0].ID, nil),
	} {
		req.Header.Set("Authorization", session)
		if res := env.do(req); res.Code != http.StatusForbidden && res.Code != http.StatusNotFound {
			t.Fatalf("%s %s = %d", req.Method, req.URL.Path, res.Code)
		}
	}
	if grants := decodeBody[[]OIDCGrant](t, env.panelGet("/api/panel/users/"+env.user.Id+"/oidc-grants")); len(grants) != 1 {
		t.Fatalf("grant was revoked by another user: %+v", grants)
	}
}
```

Add the two paths to the map in `TestOpenAPIDescribesConnectedApps` in `backend/internal/api/openapi_oidc_test.go`:

```go
		"/api/panel/users/{id}/oidc-grants":           {"GET"},
		"/api/panel/users/{id}/oidc-grants/{grantId}": {"DELETE"},
```

- [ ] **Step 6: Run them to make sure they fail**

Run: `cd backend && go test ./internal/api -run 'TestOIDCGrants|TestOpenAPIDescribesConnectedApps'`
Expected: FAIL to compile with `undefined: OIDCGrant`.

- [ ] **Step 7: Add the DTO, handlers, routes and OpenAPI paths**

In `backend/internal/api/dto.go`, after `OIDCAuthorizationDecision`:

```go
// OIDCGrant is a Connected App a User has authorized. last_used_at is absent
// until the app first receives tokens.
type OIDCGrant struct {
	ID         string   `json:"id"`
	ClientName string   `json:"client_name"`
	Scopes     []string `json:"scopes"`
	Created    string   `json:"created"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
}
```

Create `backend/internal/api/oidc_grants.go`:

```go
package api

import (
	"errors"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/oidc"
)

// listOIDCGrants returns the Connected Apps a User has authorized.
func (h *Handlers) listOIDCGrants(e *core.RequestEvent) error {
	userID := e.Request.PathValue("id")
	if _, err := h.app.FindRecordById("users", userID); err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	grants, err := oidc.ListActiveGrants(h.app, userID)
	if err != nil {
		return apis.NewInternalServerError("failed to list connected app grants", err)
	}
	out := make([]OIDCGrant, 0, len(grants))
	for _, grant := range grants {
		out = append(out, publicOIDCGrant(grant))
	}
	return ok(e, out)
}

func (h *Handlers) revokeOIDCGrant(e *core.RequestEvent) error {
	err := oidc.RevokeGrant(h.app, e.Request.PathValue("id"), e.Request.PathValue("grantId"), time.Now())
	if errors.Is(err, oidc.ErrGrantNotFound) {
		return apis.NewNotFoundError("connected app grant not found", nil)
	}
	if err != nil {
		return apis.NewInternalServerError("failed to revoke connected app grant", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func publicOIDCGrant(grant oidc.ActiveGrant) OIDCGrant {
	out := OIDCGrant{
		ID:         grant.ID,
		ClientName: grant.ClientName,
		Scopes:     grant.Scopes,
		Created:    grant.Created.UTC().Format(time.RFC3339),
	}
	if !grant.LastUsedAt.IsZero() {
		lastUsed := grant.LastUsedAt.UTC().Format(time.RFC3339)
		out.LastUsedAt = &lastUsed
	}
	return out
}
```

In `backend/internal/api/api.go`, after the two `/users/{id}/oauth-links` routes:

```go
	g.GET("/users/{id}/oidc-grants", h.listOIDCGrants).Bind(activeAdminOrSelf)
	g.DELETE("/users/{id}/oidc-grants/{grantId}", h.revokeOIDCGrant).Bind(activeAdminOrSelf)
```

In `backend/internal/api/openapi.go`, register the schema next to `"OIDCAuthorizationRequest": OIDCAuthorizationRequest{},`:

```go
		"OIDCGrant":                         OIDCGrant{},
```

and its required fields next to the `"OIDCAuthorizationRequest"` entry of the required-fields map:

```go
		"OIDCGrant":                      {"id", "client_name", "scopes", "created"},
```

Then add the two paths right after the `/api/panel/oidc/requests/{handle}/abort` path, in the same function:

```go
	grantIDParam := &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:     "grantId",
			In:       "path",
			Required: true,
			Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		},
	}
	t.Paths.Set("/api/panel/users/{id}/oidc-grants", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID")},
		Get: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "listOIDCGrants",
				Summary:     "List the Connected Apps a user has authorized (admin or self)",
				Tags:        []string{"connected apps"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Authorized apps, newest first"),
						Content:     content(arrayRef("OIDCGrant")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})
	t.Paths.Set("/api/panel/users/{id}/oidc-grants/{grantId}", &openapi3.PathItem{
		Parameters: openapi3.Parameters{idParam("User ID"), grantIDParam},
		Delete: func() *openapi3.Operation {
			op := &openapi3.Operation{
				OperationID: "revokeOIDCGrant",
				Summary:     "Revoke a Connected App's access to a user (admin or self)",
				Tags:        []string{"connected apps"},
				Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Revocation confirmed"),
						Content:     content(ref("DeleteResponse")),
					},
				})),
			}
			op.Responses.Set("404", notFound)
			withAuth(op)
			return op
		}(),
	})
```

- [ ] **Step 8: Run the tests and the gates**

Run: `cd backend && gofmt -l . ; go build ./... && go vet ./... && go test ./internal/oidc ./internal/api`
Expected: no gofmt output, PASS.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/oidc/grants.go backend/internal/oidc/grants_test.go backend/internal/api/oidc_grants.go backend/internal/api/oidc_grants_test.go backend/internal/api/dto.go backend/internal/api/openapi.go backend/internal/api/openapi_oidc_test.go backend/internal/api/api.go
git commit -m "feat(backend): list and revoke authorized apps" -m "A User, or an admin for them, can list the Connected Apps the User has authorized and revoke one. Revoking marks the grant user_revoked, which ends every token issued under it, because each token lookup checks the grant. Another User gets 403 or 404 and the grant stays."
```

### Task 22: Tell the consent page about `prompt=none`

**Files:**
- Modify: `backend/internal/api/oidc_protocol.go` (`oidcAuthorize`)
- Test: `backend/internal/api/oidc_protocol_test.go`

**Interfaces:**
- Consumes: `oidc.AuthorizeRequest.Prompt []string`, `oidc.PromptNone`, test helpers `newOIDCTestEnv`, `env.get`, `env.authorizeQuery`.
- Produces: the consent page URL `/oidc/authorize?request=<handle>&prompt=none` for `prompt=none` requests, read by Task 26.

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/api/oidc_protocol_test.go`:

```go
func TestOIDCAuthorizeTellsTheConsentPageAboutPromptNone(t *testing.T) {
	env := newOIDCTestEnv(t, nil)
	for prompt, want := range map[string]string{"none": "none", "": "", "consent": "", "login": ""} {
		res := env.get("/api/oidc/authorize?" + env.authorizeQuery(map[string]string{"prompt": prompt}).Encode())
		location, err := url.Parse(res.Header().Get("Location"))
		if res.Code != http.StatusSeeOther || err != nil || location.Path != "/oidc/authorize" ||
			location.Query().Get("request") == "" || location.Query().Get("prompt") != want {
			t.Errorf("prompt=%q: %d %q", prompt, res.Code, res.Header().Get("Location"))
		}
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `cd backend && go test ./internal/api -run TestOIDCAuthorizeTellsTheConsentPageAboutPromptNone`
Expected: FAIL with `prompt="none": 303 ".../oidc/authorize?request=..."`.

- [ ] **Step 3: Append the hint**

In `oidcAuthorize`, replace the final `return e.Redirect(...)` with:

```go
	consentPage := h.publicConfig.FrontendURL + "/oidc/authorize?request=" + url.QueryEscape(handle)
	if slices.Contains(req.Prompt, oidc.PromptNone) {
		// The page cannot read the request before the visitor signs in, and a
		// prompt=none request must end at once instead of showing the sign-in
		// form. The abort endpoint still decides the error from the stored
		// request, so a forged hint changes nothing the app receives.
		consentPage += "&prompt=none"
	}
	return e.Redirect(http.StatusSeeOther, consentPage)
```

Add `"slices"` to the standard library imports of `oidc_protocol.go`.

- [ ] **Step 4: Run the tests**

Run: `cd backend && gofmt -l . ; go vet ./... && go test ./internal/api -run 'OIDC'`
Expected: no gofmt output, PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/oidc_protocol.go backend/internal/api/oidc_protocol_test.go
git commit -m "feat(backend): mark prompt=none on the consent url" -m "The consent page cannot read a request before the visitor signs in. A prompt=none request from a browser without a panel session must go back to the app with login_required at once, so /authorize now adds prompt=none to the consent page URL. The abort endpoint still picks the error from the stored request."
```

### Task 23: Frontend API layer and copy

**Files:**
- Modify: `frontend/src/api/queries.ts`, `frontend/src/lib/api-error.ts`
- Create: `frontend/src/lib/oidc-scopes.ts`
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`
- Generated: `frontend/src/api/schema.d.ts` (gitignored)

**Interfaces:**
- Consumes: OpenAPI schemas `OIDCAuthorizationRequest`, `OIDCAuthorizationDecision`, `OIDCGrant` and the paths from Task 21 and layer 3.
- Produces (all exported from `~/api/queries`): types `OIDCAuthorizationRequest`, `OIDCAuthorizationDecision`, `OIDCGrant`; `queryKeys.oidcRequest(handle)`, `queryKeys.userOIDCGrants(userId)`; `oidcAuthorizationRequestQueryOptions(handle)`, `approveOIDCAuthorizationRequest(handle)`, `denyOIDCAuthorizationRequest(handle)`, `abortOIDCAuthorizationRequest(handle)` (all resolve to `OIDCAuthorizationDecision`), `userOIDCGrantsQueryOptions(userId)`, `revokeUserOIDCGrant(userId, grantId)`. From `~/lib/oidc-scopes`: `scopeLabel(scope: string): string`, `formatScopes(scopes: string[]): string`. Every message key listed in Step 4.

- [ ] **Step 1: Regenerate the API types**

Run: `cd frontend && pnpm api:sync`
Expected: `src/api/schema.d.ts` contains `OIDCGrant` and `/api/panel/users/{id}/oidc-grants`.

- [ ] **Step 2: Add the fetchers**

In `frontend/src/api/queries.ts`, next to the other `OIDCClient*` aliases:

```ts
type OIDCAuthorizationRequest = components["schemas"]["OIDCAuthorizationRequest"];
type OIDCAuthorizationDecision = components["schemas"]["OIDCAuthorizationDecision"];
type OIDCGrant = components["schemas"]["OIDCGrant"];
```

Add `OIDCAuthorizationRequest`, `OIDCAuthorizationDecision` and `OIDCGrant` to the `export type { ... }` list that already exports `OIDCClient`.

In `queryKeys`, after `oidcClients`:

```ts
  oidcRequest: (handle: string) => [...queryKeys.all, "oidc-requests", handle] as const,
  userOIDCGrants: (userId: string) => [...queryKeys.all, "users", userId, "oidc-grants"] as const,
```

After `rotateOIDCClientSecret`:

```ts
export function fetchOIDCAuthorizationRequest(handle: string): Promise<OIDCAuthorizationRequest> {
  return apiRequest<OIDCAuthorizationRequest>(
    apiClient.GET("/api/panel/oidc/requests/{handle}", { params: { path: { handle } } }),
    m.error_oidc_request_load()
  );
}

export function oidcAuthorizationRequestQueryOptions(handle: string) {
  return queryOptions({
    queryKey: queryKeys.oidcRequest(handle),
    queryFn: () => fetchOIDCAuthorizationRequest(handle),
    enabled: canQueryPanelApi() && handle !== "",
    // A request is single-use. A cached view from an earlier visit could
    // offer a decision the backend no longer accepts.
    gcTime: 0,
    retry: false,
  });
}

export function approveOIDCAuthorizationRequest(handle: string): Promise<OIDCAuthorizationDecision> {
  return apiRequest<OIDCAuthorizationDecision>(
    apiClient.POST("/api/panel/oidc/requests/{handle}/approve", { params: { path: { handle } } }),
    m.error_oidc_request_decide()
  );
}

export function denyOIDCAuthorizationRequest(handle: string): Promise<OIDCAuthorizationDecision> {
  return apiRequest<OIDCAuthorizationDecision>(
    apiClient.POST("/api/panel/oidc/requests/{handle}/deny", { params: { path: { handle } } }),
    m.error_oidc_request_decide()
  );
}

export function abortOIDCAuthorizationRequest(handle: string): Promise<OIDCAuthorizationDecision> {
  return apiRequest<OIDCAuthorizationDecision>(
    apiClient.POST("/api/panel/oidc/requests/{handle}/abort", { params: { path: { handle } } }),
    m.error_oidc_request_decide()
  );
}

export function fetchUserOIDCGrants(userId: string): Promise<OIDCGrant[]> {
  return apiRequest<OIDCGrant[]>(
    apiClient.GET("/api/panel/users/{id}/oidc-grants", { params: { path: { id: userId } } }),
    m.error_oidc_grants_load()
  );
}

export function userOIDCGrantsQueryOptions(userId: string) {
  return queryOptions({
    queryKey: queryKeys.userOIDCGrants(userId),
    queryFn: () => fetchUserOIDCGrants(userId),
    enabled: canQueryPanelApi(),
  });
}

export function revokeUserOIDCGrant(userId: string, grantId: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/users/{id}/oidc-grants/{grantId}", {
      params: { path: { id: userId, grantId } },
    }),
    m.error_oidc_grant_revoke()
  );
}
```

- [ ] **Step 3: Map the backend errors and add the scope helpers**

In `frontend/src/lib/api-error.ts`, add to `API_ERROR_MAP` after the `oidc provider requires ...` entry:

```ts
  "authorization request not found or expired": () => m.error_oidc_request_expired(),
  "reauthentication required": () => m.error_oidc_reauthentication_required(),
  "account cannot sign in to connected apps": () => m.error_oidc_account_not_allowed(),
  "connected app grant not found": () => m.error_oidc_grant_not_found(),
```

Create `frontend/src/lib/oidc-scopes.ts`:

```ts
import * as m from "~/paraglide/messages.js";

const SCOPE_LABELS: Record<string, () => string> = {
  openid: m.oidc_scope_openid,
  email: m.oidc_scope_email,
  offline_access: m.oidc_scope_offline_access,
};

/** Describes what a Connected App receives for a scope. */
export function scopeLabel(scope: string): string {
  return Object.hasOwn(SCOPE_LABELS, scope) ? SCOPE_LABELS[scope]() : scope;
}

/** Joins scope labels for a one-line summary. The labels are written to stand alone. */
export function formatScopes(scopes: string[]): string {
  return scopes.map(scopeLabel).join(" · ");
}
```

- [ ] **Step 4: Add the copy**

Add these keys to `frontend/messages/en.json`:

```json
  "auth_reauth_desc": "Sign in again to continue",
  "oidc_error_title": "Can't continue to the app",
  "oidc_error_invalid_request": "The app sent an incomplete sign-in request. Go back to the app and try again.",
  "oidc_error_invalid_client": "This app is not registered with this panel, or an admin turned it off.",
  "oidc_error_invalid_redirect_uri": "The app asked to return to an address it has not registered. Ask the app's owner to check its settings.",
  "oidc_error_rate_limited": "Too many sign-in attempts. Wait a minute and try again.",
  "oidc_error_unknown": "The sign-in request could not be completed. Go back to the app and try again.",
  "oidc_error_home": "Go to the panel",
  "oidc_consent_title": "{client} wants to use your account",
  "oidc_consent_desc": "{client} will receive",
  "oidc_consent_account": "Signed in as {email}",
  "oidc_consent_switch_account": "Switch account",
  "oidc_consent_redirect": "You will return to {origin}",
  "oidc_consent_allow": "Allow",
  "oidc_consent_deny": "Cancel",
  "oidc_consent_loading": "Loading the sign-in request",
  "oidc_consent_redirecting": "Returning to the app",
  "oidc_consent_reauth_title": "Confirm it's you",
  "oidc_consent_reauth_desc": "{client} asks you to sign in again before it continues.",
  "oidc_consent_reauth_action": "Sign in again",
  "oidc_consent_expired": "This sign-in request has expired or was already used. Go back to the app and start again.",
  "oidc_consent_not_allowed": "Your account can't sign in to connected apps. It must be active and have a verified email.",
  "oidc_consent_session_expired": "Your panel session has ended. Sign in again to continue.",
  "oidc_consent_failed": "Something went wrong. Go back to the app and try again.",
  "oidc_scope_openid": "Your account ID",
  "oidc_scope_email": "Your email address",
  "oidc_scope_offline_access": "Access while you are away from the app",
  "settings_authorized_apps_title": "Authorized apps",
  "settings_authorized_apps_desc": "Apps you allowed to sign you in with this account",
  "settings_authorized_apps_authorized_at": "Authorized {time}",
  "settings_authorized_apps_last_used": "Last used {time}",
  "settings_authorized_apps_revoke": "Revoke",
  "settings_authorized_apps_revoking": "Revoking",
  "settings_authorized_apps_revoke_title": "Revoke {app}",
  "settings_authorized_apps_revoke_confirm": "{app} loses access to your account. Next time you sign in to it, it asks for your approval again.",
  "error_oidc_request_load": "Failed to load the sign-in request",
  "error_oidc_request_decide": "Failed to finish the sign-in request",
  "error_oidc_request_expired": "This sign-in request has expired",
  "error_oidc_reauthentication_required": "Sign in again to continue",
  "error_oidc_account_not_allowed": "This account can't sign in to connected apps",
  "error_oidc_grant_not_found": "This app is no longer authorized",
  "error_oidc_grants_load": "Failed to load authorized apps",
  "error_oidc_grant_revoke": "Failed to revoke access"
```

Add the same keys to `frontend/messages/zh-cn.json`:

```json
  "auth_reauth_desc": "请重新登录以继续",
  "oidc_error_title": "无法继续前往应用",
  "oidc_error_invalid_request": "应用发来的登录请求不完整。请返回应用重试。",
  "oidc_error_invalid_client": "这个应用没有在本面板登记，或已被管理员停用。",
  "oidc_error_invalid_redirect_uri": "应用要求跳回一个未登记的地址。请联系应用的维护者检查配置。",
  "oidc_error_rate_limited": "登录尝试过多。请等一分钟再试。",
  "oidc_error_unknown": "无法完成这次登录请求。请返回应用重试。",
  "oidc_error_home": "前往面板",
  "oidc_consent_title": "{client} 请求使用你的账号",
  "oidc_consent_desc": "{client} 将获得",
  "oidc_consent_account": "当前账号 {email}",
  "oidc_consent_switch_account": "切换账号",
  "oidc_consent_redirect": "随后返回 {origin}",
  "oidc_consent_allow": "允许",
  "oidc_consent_deny": "取消",
  "oidc_consent_loading": "正在读取登录请求",
  "oidc_consent_redirecting": "正在返回应用",
  "oidc_consent_reauth_title": "确认是你本人",
  "oidc_consent_reauth_desc": "{client} 要求你重新登录后再继续。",
  "oidc_consent_reauth_action": "重新登录",
  "oidc_consent_expired": "这个登录请求已过期或已使用。请返回应用重新开始。",
  "oidc_consent_not_allowed": "你的账号无法登录接入应用，账号需已启用且邮箱已验证。",
  "oidc_consent_session_expired": "面板登录已失效。请重新登录以继续。",
  "oidc_consent_failed": "出现问题。请返回应用重试。",
  "oidc_scope_openid": "你的账号 ID",
  "oidc_scope_email": "你的邮箱地址",
  "oidc_scope_offline_access": "你离开应用后仍可访问",
  "settings_authorized_apps_title": "已授权的应用",
  "settings_authorized_apps_desc": "你允许用此账号登录的应用。",
  "settings_authorized_apps_authorized_at": "授权于 {time}",
  "settings_authorized_apps_last_used": "最近使用 {time}",
  "settings_authorized_apps_revoke": "撤销",
  "settings_authorized_apps_revoking": "正在撤销",
  "settings_authorized_apps_revoke_title": "撤销 {app}",
  "settings_authorized_apps_revoke_confirm": "{app} 将无法再访问你的账号。下次登录该应用时，它会重新请求你的同意。",
  "error_oidc_request_load": "读取登录请求失败",
  "error_oidc_request_decide": "完成登录请求失败",
  "error_oidc_request_expired": "登录请求已过期",
  "error_oidc_reauthentication_required": "请重新登录后继续",
  "error_oidc_account_not_allowed": "此账号无法登录接入应用",
  "error_oidc_grant_not_found": "该应用已不再被授权",
  "error_oidc_grants_load": "读取已授权应用失败",
  "error_oidc_grant_revoke": "撤销授权失败"
```

- [ ] **Step 5: Run the gates**

Run: `cd frontend && pnpm build && pnpm typecheck && pnpm i18n:check`
Expected: PASS. `pnpm build` runs first, because it compiles the new keys into `src/paraglide/`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/queries.ts frontend/src/lib/api-error.ts frontend/src/lib/oidc-scopes.ts frontend/messages/en.json frontend/messages/zh-cn.json
git commit -m "feat(frontend): add consent and grant api layer" -m "Fetchers and query options for the consent endpoints and the authorized apps endpoints, localized backend errors, scope labels, and the copy for the layer 4 pages in en and zh-cn."
```

### Task 24: Return to the requesting page after sign-in

**Files:**
- Create: `frontend/src/lib/safe-redirect.ts`
- Modify: `frontend/src/routes/login.tsx`, `frontend/src/api/auth.ts`, `frontend/src/routes/oauth-callback.tsx`

**Interfaces:**
- Consumes: `m.auth_reauth_desc` (Task 23).
- Produces: `safeRedirectPath(value: unknown): string | undefined`; `/login` search `{ redirect?: string; reauth?: true }`; `startOAuthLogin(providerName: string, returnTo?: string)`; `PendingOAuth.returnTo?: string`.

- [ ] **Step 1: Add the redirect check**

Create `frontend/src/lib/safe-redirect.ts`:

```ts
const PANEL_ORIGIN = "http://panel.invalid";

/**
 * Returns value when it is a path on this panel, normalized, or undefined.
 * A protocol-relative or absolute value would turn the sign-in page into an
 * open redirect. Parsing against a fixed origin also catches the forms that
 * browsers rewrite, such as backslashes and embedded tabs.
 */
export function safeRedirectPath(value: unknown): string | undefined {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//")) {
    return undefined;
  }
  let url: URL;
  try {
    url = new URL(value, PANEL_ORIGIN);
  } catch {
    return undefined;
  }
  // Dot segments can normalize into a protocol-relative path, as in /.//host.
  if (url.origin !== PANEL_ORIGIN || url.pathname.startsWith("//")) return undefined;
  return url.pathname + url.search + url.hash;
}
```

- [ ] **Step 2: Check it against hostile values**

Run from `frontend/`:

```bash
node --experimental-strip-types -e '
import("./src/lib/safe-redirect.ts").then(({ safeRedirectPath }) => {
  const cases = {
    "/oidc/authorize?request=abc": "/oidc/authorize?request=abc",
    "/settings#apps": "/settings#apps",
    "//evil.example": undefined,
    "/\\evil.example": undefined,
    "/\t/evil.example": undefined,
    "https://evil.example": undefined,
    "javascript:alert(1)": undefined,
    "/.//evil.example": undefined,
    "/a/..//evil.example": undefined,
    "": undefined,
  };
  let failed = 0;
  for (const [input, want] of Object.entries(cases)) {
    const got = safeRedirectPath(input);
    if (got !== want) { failed++; console.error(JSON.stringify(input), "->", got, "want", want); }
  }
  console.log(failed ? "FAIL" : "PASS");
  process.exit(failed ? 1 : 0);
});'
```

Expected: `PASS`. This check is not committed, because the frontend has no test runner (AGENTS.md).

- [ ] **Step 3: Accept `redirect` and `reauth` on `/login`**

In `frontend/src/routes/login.tsx`, add the import:

```ts
import { safeRedirectPath } from "~/lib/safe-redirect";
```

Replace the route definition with:

```tsx
interface LoginSearch {
  redirect?: string;
  reauth?: true;
}

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: safeRedirectPath(search.redirect),
    reauth: search.reauth === true || search.reauth === "true" ? true : undefined,
  }),
  beforeLoad: ({ context, search }) => {
    // A Connected App that asked for a fresh sign-in needs the form even
    // when a panel session exists.
    if (context.auth && !search.reauth) {
      if (search.redirect) throw redirect({ href: search.redirect });
      throw redirect({ to: "/", search: defaultDashboardSearch() });
    }
  },
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(panelConfigQueryOptions()).catch(() => undefined);
  },
  component: LoginPage,
});
```

In `LoginPage`, read the search after `useAppName()`:

```tsx
  const search = Route.useSearch();
  const destination = search.redirect ?? "/";
```

Change both `onSuccess` handlers of `loginMutation` and `passkeyMutation` from `window.location.href = "/";` to:

```tsx
      window.location.href = destination;
```

Change the OAuth mutation to carry the destination:

```tsx
  const oauthMutation = useMutation({
    mutationFn: (providerName: string) => startOAuthLogin(providerName, search.redirect),
  });
```

Change the card description to:

```tsx
        <Card.Description className="text-[13px] text-muted">
          {search.reauth ? m.auth_reauth_desc() : m.auth_sign_in_continue()}
        </Card.Description>
```

- [ ] **Step 4: Carry the destination through OAuth2**

In `frontend/src/api/auth.ts`, add the field to `PendingOAuth` after `userId`:

```ts
  // Where a login returns afterwards. The callback checks it again.
  returnTo?: string;
```

Replace `startOAuthLogin` and the `target` parameter type of `startOAuthFlow`:

```ts
export function startOAuthLogin(providerName: string, returnTo?: string): Promise<void> {
  return startOAuthFlow(providerName, { intent: "login", returnTo });
}
```

```ts
async function startOAuthFlow(
  providerName: string,
  target: Pick<PendingOAuth, "intent" | "userId" | "returnTo">
): Promise<void> {
```

In `frontend/src/routes/oauth-callback.tsx`, import the check:

```ts
import { safeRedirectPath } from "~/lib/safe-redirect";
```

and replace the `.then(...)` of `completeOAuth` with:

```tsx
      .then(() => {
        const returnTo = pendingIntent === "login" ? safeRedirectPath(pending?.returnTo) : undefined;
        window.location.replace(returnTo ?? DONE_PATH[pendingIntent]);
      })
```

- [ ] **Step 5: Run the gates**

Run: `cd frontend && pnpm typecheck && pnpm build`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/safe-redirect.ts frontend/src/routes/login.tsx frontend/src/api/auth.ts frontend/src/routes/oauth-callback.tsx
git commit -m "feat(frontend): return to the requesting page after sign-in" -m "/login accepts redirect, limited to paths on this panel, and reauth=true, which shows the form to a signed-in User. Password, passkey and OAuth2 sign-in return to redirect. OAuth2 carries it in the pending sessionStorage entry and the callback checks it again. The consent page needs both for prompt=login and max_age."
```

### Task 25: OIDC error page

**Files:**
- Create: `frontend/src/routes/oidc/error.tsx`
- Generated: `frontend/src/routeTree.gen.ts` (committed)

**Interfaces:**
- Consumes: reasons `invalid_request`, `invalid_client`, `invalid_redirect_uri`, `rate_limited` from `/api/oidc/authorize` (layer 3); `m.oidc_error_*` (Task 23).
- Produces: route `/oidc/error?reason=`.

- [ ] **Step 1: Create the page**

```tsx
import { Link, createFileRoute } from "@tanstack/react-router";
import { Card } from "@heroui/react";
import { AuthBrandMark, AuthShell } from "~/components/ui";
import { defaultDashboardSearch } from "~/lib/dashboard-search";
import * as m from "~/paraglide/messages.js";

// The backend sends the browser here when it cannot trust the app or its
// redirect_uri, so the error must not go back to the app (RFC 6749 4.1.2.1).
const REASONS: Record<string, () => string> = {
  invalid_request: m.oidc_error_invalid_request,
  invalid_client: m.oidc_error_invalid_client,
  invalid_redirect_uri: m.oidc_error_invalid_redirect_uri,
  rate_limited: m.oidc_error_rate_limited,
};

export const Route = createFileRoute("/oidc/error")({
  validateSearch: (search: Record<string, unknown>): { reason?: string } => ({
    reason: typeof search.reason === "string" ? search.reason : undefined,
  }),
  component: OIDCErrorPage,
});

function OIDCErrorPage() {
  const { reason } = Route.useSearch();
  const message =
    reason && Object.hasOwn(REASONS, reason) ? REASONS[reason]() : m.oidc_error_unknown();

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.oidc_error_title()}
        </Card.Title>
      </Card.Header>
      <Card.Content className="flex flex-col gap-3 px-6 pt-4 pb-6">
        <p className="text-[13px] text-muted" role="alert">
          {message}
        </p>
        <Link
          to="/"
          search={defaultDashboardSearch()}
          className="text-[13px] font-medium text-accent hover:opacity-80"
        >
          {m.oidc_error_home()}
        </Link>
      </Card.Content>
    </AuthShell>
  );
}
```

- [ ] **Step 2: Run the gates**

Run: `cd frontend && pnpm build && pnpm typecheck`
Expected: PASS. `pnpm build` regenerates `src/routeTree.gen.ts` with `/oidc/error`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/oidc/error.tsx frontend/src/routeTree.gen.ts
git commit -m "feat(frontend): add the oidc error page" -m "/api/oidc/authorize sends the browser here when the app or its redirect_uri cannot be trusted, so the error cannot go back to the app. Unknown reasons, including inherited object keys, get a generic message."
```

### Task 26: Consent page

**Files:**
- Create: `frontend/src/routes/oidc/authorize.tsx`
- Generated: `frontend/src/routeTree.gen.ts` (committed)

**Interfaces:**
- Consumes: `oidcAuthorizationRequestQueryOptions`, `approveOIDCAuthorizationRequest`, `denyOIDCAuthorizationRequest`, `abortOIDCAuthorizationRequest`, `PanelApiError`, `type OIDCAuthorizationRequest` (`~/api/queries`); `clearAuth` (`~/api/auth`); `scopeLabel` (`~/lib/oidc-scopes`); `/login` search `{ redirect, reauth }` (Task 24); the `prompt=none` hint (Task 22); `m.oidc_consent_*`, `m.oidc_scope_*` (Task 23). The backend answers 404 for an unknown or used request, 403 for a User who may not sign in, and 400 when approve needs a fresh sign-in.
- Produces: route `/oidc/authorize?request=<handle>[&prompt=none]`.

- [ ] **Step 1: Create the page**

```tsx
import { useEffect, useRef } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { Button, Card } from "@heroui/react";
import { clearAuth } from "~/api/auth";
import {
  PanelApiError,
  abortOIDCAuthorizationRequest,
  approveOIDCAuthorizationRequest,
  denyOIDCAuthorizationRequest,
  oidcAuthorizationRequestQueryOptions,
  type OIDCAuthorizationRequest,
} from "~/api/queries";
import { AuthBrandMark, AuthShell } from "~/components/ui";
import { scopeLabel } from "~/lib/oidc-scopes";
import { useAppName } from "~/lib/use-app-name";
import * as m from "~/paraglide/messages.js";

interface ConsentSearch {
  request: string;
  // Set by /api/oidc/authorize for prompt=none. It only chooses between an
  // immediate abort and the sign-in form. The backend picks the error.
  prompt?: "none";
}

export const Route = createFileRoute("/oidc/authorize")({
  validateSearch: (search: Record<string, unknown>): ConsentSearch => ({
    request: typeof search.request === "string" ? search.request : "",
    prompt: search.prompt === "none" ? "none" : undefined,
  }),
  // The page grants access to the account, so no other site may frame it,
  // and the request handle in its URL stays out of Referer headers.
  headers: () => ({
    "Content-Security-Policy": "frame-ancestors 'none'",
    "X-Frame-Options": "DENY",
    "Referrer-Policy": "no-referrer",
  }),
  beforeLoad: ({ context, search, location }) => {
    if (!context.auth && search.request && search.prompt !== "none") {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: ConsentPage,
});

function ConsentPage() {
  const { auth } = Route.useRouteContext();
  const { request } = Route.useSearch();
  if (!request) return <ConsentMessage message={m.oidc_consent_expired()} />;
  if (!auth) return <SilentAbort handle={request} />;
  return <SignedInConsent handle={request} email={auth.user.email} />;
}

function consentPath(handle: string): string {
  return `/oidc/authorize?request=${encodeURIComponent(handle)}`;
}

function errorStatus(error: unknown): number | undefined {
  return error instanceof PanelApiError ? error.status : undefined;
}

function decisionErrorMessage(error: unknown): string {
  switch (errorStatus(error)) {
    case 404:
      return m.oidc_consent_expired();
    case 403:
      return m.oidc_consent_not_allowed();
    case 401:
      return m.oidc_consent_session_expired();
  }
  return m.oidc_consent_failed();
}

// A prompt=none request from a browser without a panel session goes back to
// the app with login_required without showing anything.
function SilentAbort({ handle }: { handle: string }) {
  const abort = useMutation({
    mutationFn: () => abortOIDCAuthorizationRequest(handle),
    onSuccess: (decision) => window.location.replace(decision.redirect_to),
  });
  const { mutate } = abort;
  const ran = useRef(false);

  useEffect(() => {
    // The request is single-use, so StrictMode's second run must not resend it.
    if (ran.current) return;
    ran.current = true;
    mutate();
  }, [mutate]);

  return (
    <ConsentMessage
      message={abort.isError ? decisionErrorMessage(abort.error) : m.oidc_consent_redirecting()}
    />
  );
}

function SignedInConsent({ handle, email }: { handle: string; email: string }) {
  const navigate = useNavigate();
  const requestQuery = useQuery(oidcAuthorizationRequestQueryOptions(handle));
  const decision = useMutation({
    mutationFn: (allow: boolean) =>
      allow ? approveOIDCAuthorizationRequest(handle) : denyOIDCAuthorizationRequest(handle),
    onSuccess: (result) => window.location.assign(result.redirect_to),
  });
  const view = requestQuery.data;
  // Remembered consent and prompt=none skip the page. The backend turns a
  // prompt=none request it cannot answer into an error for the app.
  const automatic =
    view !== undefined &&
    (view.prompt_none || (!view.consent_required && !view.reauthentication_required));
  const { mutate } = decision;
  const started = useRef(false);

  useEffect(() => {
    if (!automatic || started.current) return;
    started.current = true;
    mutate(true);
  }, [automatic, mutate]);

  function signInAgain() {
    void navigate({ to: "/login", search: { reauth: true, redirect: consentPath(handle) } });
  }

  function switchAccount() {
    clearAuth();
    void navigate({ to: "/login", search: { redirect: consentPath(handle) } });
  }

  const failure = requestQuery.error ?? decision.error;
  if (failure) {
    if (errorStatus(failure) === 400 && view) {
      return <ReauthCard clientName={view.client_name} onSignIn={signInAgain} />;
    }
    const signIn =
      errorStatus(failure) === 401
        ? { label: m.oidc_consent_reauth_action(), onPress: signInAgain }
        : undefined;
    return <ConsentMessage message={decisionErrorMessage(failure)} action={signIn} />;
  }
  if (!view) return <ConsentMessage message={m.oidc_consent_loading()} />;
  if (automatic || decision.isSuccess) {
    return <ConsentMessage message={m.oidc_consent_redirecting()} />;
  }
  if (view.reauthentication_required) {
    return <ReauthCard clientName={view.client_name} onSignIn={signInAgain} />;
  }
  return (
    <ConsentCard
      view={view}
      email={email}
      pending={decision.isPending}
      onDecide={(allow) => decision.mutate(allow)}
      onSwitchAccount={switchAccount}
    />
  );
}

function ConsentCard({
  view,
  email,
  pending,
  onDecide,
  onSwitchAccount,
}: {
  view: OIDCAuthorizationRequest;
  email: string;
  pending: boolean;
  onDecide: (allow: boolean) => void;
  onSwitchAccount: () => void;
}) {
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.oidc_consent_title({ client: view.client_name })}
        </Card.Title>
        <Card.Description className="text-[13px] text-muted">
          {m.oidc_consent_redirect({ origin: view.redirect_origin })}
        </Card.Description>
      </Card.Header>
      <Card.Content className="flex flex-col gap-4 px-6 pt-4 pb-6">
        <div>
          <p className="text-[13px] font-medium text-foreground">
            {m.oidc_consent_desc({ client: view.client_name })}
          </p>
          <ul className="mt-2 flex list-disc flex-col gap-1 pl-5">
            {view.scopes.map((scope) => (
              <li key={scope} className="text-[13px] text-muted">
                {scopeLabel(scope)}
              </li>
            ))}
          </ul>
        </div>
        <div className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
          <span className="min-w-0 truncate text-[13px] text-muted">
            {m.oidc_consent_account({ email })}
          </span>
          <Button size="sm" variant="ghost" isDisabled={pending} onPress={onSwitchAccount}>
            {m.oidc_consent_switch_account()}
          </Button>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" fullWidth isDisabled={pending} onPress={() => onDecide(false)}>
            {m.oidc_consent_deny()}
          </Button>
          <Button variant="primary" fullWidth isDisabled={pending} onPress={() => onDecide(true)}>
            {m.oidc_consent_allow()}
          </Button>
        </div>
      </Card.Content>
    </AuthShell>
  );
}

function ReauthCard({ clientName, onSignIn }: { clientName: string; onSignIn: () => void }) {
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.oidc_consent_reauth_title()}
        </Card.Title>
        <Card.Description className="text-[13px] text-muted">
          {m.oidc_consent_reauth_desc({ client: clientName })}
        </Card.Description>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        <Button variant="primary" fullWidth onPress={onSignIn}>
          {m.oidc_consent_reauth_action()}
        </Button>
      </Card.Content>
    </AuthShell>
  );
}

function ConsentMessage({
  message,
  action,
}: {
  message: string;
  action?: { label: string; onPress: () => void };
}) {
  const appName = useAppName();
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {appName}
        </Card.Title>
      </Card.Header>
      <Card.Content className="flex flex-col gap-3 px-6 pt-4 pb-6">
        <p className="text-[13px] text-muted" role="status">
          {message}
        </p>
        {action && (
          <Button variant="secondary" fullWidth onPress={action.onPress}>
            {action.label}
          </Button>
        )}
      </Card.Content>
    </AuthShell>
  );
}
```

- [ ] **Step 2: Run the gates**

Run: `cd frontend && pnpm build && pnpm typecheck && pnpm i18n:check`
Expected: PASS. `pnpm build` regenerates `src/routeTree.gen.ts` with `/oidc/authorize`.

- [ ] **Step 3: Check the response headers**

Start the dev server on a free port (`pnpm exec vite dev --host 127.0.0.1 --port 3001`) and run:

```bash
curl -s -o /dev/null -D - 'http://127.0.0.1:3001/oidc/authorize?request=x&prompt=none' | grep -i -E 'content-security-policy|x-frame-options|referrer-policy'
```

Expected: `content-security-policy: frame-ancestors 'none'`, `x-frame-options: DENY` and `referrer-policy: no-referrer`. Stop the server.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/routes/oidc/authorize.tsx frontend/src/routeTree.gen.ts
git commit -m "feat(frontend): add the connected app consent page" -m "The page names the app, the claims it receives, the address it returns to and the signed-in account, with Allow, Cancel and Switch account. Remembered consent and prompt=none approve without showing the page. prompt=login and max_age send the User through /login?reauth=true. A visitor without a panel session signs in first, except for prompt=none, which aborts at once. The document forbids framing and sends no Referer."
```

### Task 27: Authorized apps in settings

**Files:**
- Create: `frontend/src/components/authorized-apps.tsx`
- Modify: `frontend/src/routes/settings/index.tsx`

**Interfaces:**
- Consumes: `userOIDCGrantsQueryOptions`, `revokeUserOIDCGrant`, `queryKeys.userOIDCGrants`, `queryErrorMessage`, `type OIDCGrant` (`~/api/queries`); `formatScopes` (`~/lib/oidc-scopes`); `DestructiveConfirmModal`, `ErrorAlert` (`~/components/ui`); `relTimeFromISO`, `useHydratedNow`; `m.settings_authorized_apps_*`, `m.error_oidc_grant_revoke` (Task 23).
- Produces: `AuthorizedAppsSection({ userId }: { userId: string })`.

- [ ] **Step 1: Create the section**

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@heroui/react";
import {
  queryErrorMessage,
  queryKeys,
  revokeUserOIDCGrant,
  userOIDCGrantsQueryOptions,
  type OIDCGrant,
} from "~/api/queries";
import { DestructiveConfirmModal, ErrorAlert } from "~/components/ui";
import { relTimeFromISO } from "~/lib/format";
import { formatScopes } from "~/lib/oidc-scopes";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

/** Settings page section listing the Connected Apps the User has authorized. */
export function AuthorizedAppsSection({ userId }: { userId: string }) {
  const queryClient = useQueryClient();
  const now = useHydratedNow();
  const [pendingGrant, setPendingGrant] = useState<OIDCGrant | null>(null);
  const grantsQuery = useQuery(userOIDCGrantsQueryOptions(userId));
  const revokeMutation = useMutation({
    mutationFn: (grantId: string) => revokeUserOIDCGrant(userId, grantId),
    onSuccess: () => {
      setPendingGrant(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.userOIDCGrants(userId) });
    },
  });
  const grants = grantsQuery.data ?? [];

  // Users who never authorized an app have nothing to manage here.
  if (grantsQuery.isPending || (grants.length === 0 && !grantsQuery.isError)) return null;

  const appName = pendingGrant?.client_name ?? "";
  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">
          {m.settings_authorized_apps_title()}
        </h1>
        <p className="mt-0.5 text-[13px] text-muted">{m.settings_authorized_apps_desc()}</p>
      </div>
      <div className="overflow-hidden rounded-lg border bg-surface">
        <ErrorAlert
          message={grantsQuery.error ? queryErrorMessage(grantsQuery.error) : ""}
          className="m-3"
        />
        <ul className="divide-y divide-separator">
          {grants.map((grant) => (
            <li key={grant.id} className="flex items-center justify-between gap-3 px-5 py-3">
              <div className="min-w-0">
                <p className="truncate text-[13px] font-medium text-foreground">
                  {grant.client_name}
                </p>
                <p className="mt-0.5 text-xs text-muted">{formatScopes(grant.scopes)}</p>
                <p className="mt-0.5 text-xs text-muted">
                  {grant.last_used_at
                    ? m.settings_authorized_apps_last_used({
                        time: relTimeFromISO(grant.last_used_at, now),
                      })
                    : m.settings_authorized_apps_authorized_at({
                        time: relTimeFromISO(grant.created, now),
                      })}
                </p>
              </div>
              <Button
                size="sm"
                variant="secondary"
                isDisabled={revokeMutation.isPending}
                onPress={() => setPendingGrant(grant)}
                className="border-danger text-danger hover:bg-danger-soft"
              >
                {m.settings_authorized_apps_revoke()}
              </Button>
            </li>
          ))}
        </ul>
      </div>
      <DestructiveConfirmModal
        isOpen={pendingGrant !== null}
        title={m.settings_authorized_apps_revoke_title({ app: appName })}
        body={m.settings_authorized_apps_revoke_confirm({ app: appName })}
        confirmLabel={m.settings_authorized_apps_revoke()}
        pendingLabel={m.settings_authorized_apps_revoking()}
        pending={revokeMutation.isPending}
        error={
          revokeMutation.error
            ? queryErrorMessage(revokeMutation.error, m.error_oidc_grant_revoke())
            : ""
        }
        onOpenChange={(open) => {
          if (open) return;
          setPendingGrant(null);
          revokeMutation.reset();
        }}
        onConfirm={() => pendingGrant && revokeMutation.mutate(pendingGrant.id)}
      />
    </>
  );
}
```

- [ ] **Step 2: Mount it in settings**

In `frontend/src/routes/settings/index.tsx`, import it next to `SocialLoginsSection`:

```tsx
import { AuthorizedAppsSection } from "~/components/authorized-apps";
```

and render it right after `{auth && <SocialLoginsSection userId={auth.user.id} />}`:

```tsx
      {auth && <AuthorizedAppsSection userId={auth.user.id} />}
```

- [ ] **Step 3: Run the gates**

Run: `cd frontend && pnpm typecheck && pnpm i18n:check && pnpm build`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/authorized-apps.tsx frontend/src/routes/settings/index.tsx
git commit -m "feat(frontend): list and revoke authorized apps" -m "Settings shows the Connected Apps the User has authorized, with the claims each receives and when it was last used. Revoking asks for confirmation and signs the User out of the app. The section stays hidden for Users who never authorized an app."
```

### Layer 4 exit check

- [ ] **Step 1: Run every gate**

Run: `cd backend && gofmt -l . ; go build ./... && go vet ./... && go test -timeout 300s ./...`
Expected: no gofmt output, PASS.

Run: `cd frontend && pnpm api:sync && pnpm build && pnpm typecheck && pnpm i18n:check`
Expected: PASS.

- [ ] **Step 2: Start the panel on throwaway ports**

Never touch the dev servers on 3000 and 8090. Loopback http satisfies the provider's availability check.

1. Backend in the background: `cd backend && go build -o /tmp/hp-oidc-bin . && PANEL_MASTER_KEY=live-check PANEL_BACKEND_URL_BASE=http://127.0.0.1:8091 PANEL_FRONTEND_URL_BASE=http://127.0.0.1:3001 MMDB_DIR=$PWD/mmdb /tmp/hp-oidc-bin serve --http 127.0.0.1:8091 --dir /tmp/hp-oidc-live`.
2. Frontend in the background: `cd frontend && VITE_API_BASE_URL=http://127.0.0.1:8091 pnpm exec vite dev --host 127.0.0.1 --port 3001`.
3. Create a superuser with `/tmp/hp-oidc-bin superuser upsert root@example.test RootPassword123 --dir /tmp/hp-oidc-live`, create an admin User through `/api/collections/users/records` as the superuser, enable the provider with `PATCH /api/panel/settings`, and create a Connected App with `refresh_enabled: true` and redirect URI `http://127.0.0.1:3002/callback`. Keep the client ID and secret.

- [ ] **Step 3: Start the test RP**

In `/tmp/hp-rp`, run `npm init -y && npm install openid-client@6`, write `rp.mjs` below, and start it in the background with `CLIENT_ID=... CLIENT_SECRET=... node rp.mjs`.

```js
import http from "node:http";
import * as client from "openid-client";

const issuer = new URL("http://127.0.0.1:8091/api/oidc");
const redirectUri = "http://127.0.0.1:3002/callback";
const config = await client.discovery(issuer, process.env.CLIENT_ID, process.env.CLIENT_SECRET, undefined, {
  execute: [client.allowInsecureRequests],
});
let pending = null;
let tokens = null;

function send(res, status, body) {
  res.writeHead(status, { "content-type": "text/html; charset=utf-8" });
  res.end(`<pre>${typeof body === "string" ? body : JSON.stringify(body, null, 2)}</pre>`);
}

http
  .createServer(async (req, res) => {
    const url = new URL(req.url, "http://127.0.0.1:3002");
    try {
      if (url.pathname === "/login") {
        const verifier = client.randomPKCECodeVerifier();
        pending = { verifier, state: client.randomState(), nonce: client.randomNonce() };
        const params = {
          redirect_uri: redirectUri,
          scope: "openid email offline_access",
          code_challenge: await client.calculatePKCECodeChallenge(verifier),
          code_challenge_method: "S256",
          state: pending.state,
          nonce: pending.nonce,
        };
        for (const key of ["prompt", "max_age"]) {
          if (url.searchParams.has(key)) params[key] = url.searchParams.get(key);
        }
        res.writeHead(302, { location: client.buildAuthorizationUrl(config, params).href });
        return res.end();
      }
      if (url.pathname === "/callback") {
        if (url.searchParams.has("error")) return send(res, 400, Object.fromEntries(url.searchParams));
        tokens = await client.authorizationCodeGrant(config, url, {
          pkceCodeVerifier: pending.verifier,
          expectedState: pending.state,
          expectedNonce: pending.nonce,
          idTokenExpected: true,
        });
        return send(res, 200, { claims: tokens.claims(), scope: tokens.scope });
      }
      if (url.pathname === "/userinfo") {
        return send(res, 200, await client.fetchUserInfo(config, tokens.access_token, tokens.claims().sub));
      }
      if (url.pathname === "/refresh") {
        tokens = await client.refreshTokenGrant(config, tokens.refresh_token);
        return send(res, 200, { claims: tokens.claims() });
      }
      send(res, 404, "not found");
    } catch (error) {
      send(res, 500, `${error.name}: ${error.message} ${error.code ?? ""}`);
    }
  })
  .listen(3002, "127.0.0.1");
```

- [ ] **Step 4: Walk through the flows in a browser**

Use the preview browser on `http://127.0.0.1:3002`. Expected at each step:

1. `/login` goes to the panel sign-in page. Signing in as the admin shows the consent page with the app name, "Your account ID", "Your email address", the access-while-away line, `http://127.0.0.1:3002` and the account email. Allow returns to `/callback` with claims `sub`, `email`, `email_verified`, `auth_time` and `nonce` accepted by `openid-client`.
2. The browser back button to the consent page shows the expired message.
3. `/login` again returns to `/callback` without showing the consent page.
4. `/login?prompt=none` returns with claims. After clearing the panel cookies, `/login?prompt=none` returns to `/callback` with `error=login_required` and no sign-in form.
5. Signed in again, `/login?prompt=login` shows "Confirm it's you". Sign in again returns to the consent flow and then to `/callback`, with a newer `auth_time`.
6. `/login?prompt=consent` shows the consent page. Cancel returns with `error=access_denied`.
7. Switch account on the consent page opens the sign-in form, and signing in as a second User shows the consent page for that User.
8. `/userinfo` and `/refresh` succeed. Panel settings shows the app under Authorized apps. Revoke, then `/userinfo` and `/refresh` fail with `invalid_token` and `invalid_grant`, and the section disappears.
9. `http://127.0.0.1:3001/login?redirect=//evil.example` and `?redirect=https://evil.example`, when signed in, land on the panel dashboard.
10. `http://127.0.0.1:3001/oidc/error?reason=invalid_redirect_uri` shows the redirect URI message, and `?reason=constructor` shows the generic one.

Check the zh-cn copy on the consent page and settings section, and the dark theme on the consent page.

- [ ] **Step 5: Clean up**

Stop the RP, the frontend and the backend. Delete `/tmp/hp-oidc-live`, `/tmp/hp-oidc-bin` and `/tmp/hp-rp`.

- [ ] **Step 6: Review the layer**

Dispatch a `review:code-reviewer` agent on `git diff oidc/3-protocol...oidc/4-consent-ui` with this plan's Layer 4 decisions and Review Focus. Fix every Critical and Important finding in its own commit, then rerun Step 1.

The review of this layer changed the code above in these places. The commits are the source of truth.

- `SignedInConsent` takes `promptNone`. On a prompt=none request, a 401 or 403 from the request or decision endpoints renders `SilentAbort`, so a panel cookie the backend rejects still ends with `login_required`.
- Decisions call `window.location.replace` instead of `assign`, so Back cannot restore a used consent page from the back-forward cache.
- The four `error_oidc_*` keys and their `api-error.ts` entries are gone. `apiRequest` maps every 404 to `error_not_found`, and the consent page maps statuses itself, so the entries were unreachable.
- `settings_authorized_apps_revoke_confirm` reads "{app} loses access to your account until you sign in to it again." `oidc_consent_not_allowed` names both the app and the account as possible causes.
- Revoking a grant that answers 404 counts as success in `AuthorizedAppsSection`.
- The back link on a failed OAuth2 sign-in keeps `redirect`, so the consent request is not lost.
- `revokeOIDCGrant` returns `DeleteResponse{Deleted: true}`.

- [ ] **Step 7: Update the ledger and stop**

Record the layer 4 commits, the browser results and the review outcome in `.superpowers/sdd/2026-10-12-oidc-provider/progress.md`. Stop for the user's review before starting layer 5.

---

## Layer 5: `oidc/5-finish`

Start: `git switch -c oidc/5-finish oidc/4-consent-ui`.

### Decisions for this layer

- **Management API lookup by `id`.** `GET /api/mgmt/users?id=` returns the same `PanelUser` shape as the email and Auth String lookups. A Connected App receives the User ID as `sub`, so the help center can look the account up without keeping the email. Exactly one of `id`, `email` and `auth_string` is allowed.
- **Docs.** AGENTS.md gains the provider in the scope list, a core decision, the data model, the endpoint lists and the directory tree. CONTEXT.md gains Connected App and Authorized App Grant. The backend README gains an "OIDC Provider" section with setup, the endpoints, the relying party settings and the key rotation CLI.
- **Conformance.** Run the OpenID Foundation conformance suite's `oidcc-basic-certification-test-plan` against a local panel, with static clients and discovery. The plan's modules do not send PKCE (except one), and the provider requires S256 PKCE from every client. The test setup therefore puts a small proxy in front of the backend that adds a fixed S256 challenge to `/authorize` and the matching verifier to `/token` when the request has none. The proxy and the browser driver live in `scripts/oidc-conformance/` so the run can be repeated. Results go to `docs/oidc-conformance.md`, with every non-passing module explained.
- **Not in this layer:** making PKCE optional, `request`/`request_uri` support, extra scopes. Conformance failures caused by ADR-0010 decisions are documented, not fixed.

### File structure

| File | Responsibility |
|---|---|
| `backend/internal/api/management_api.go` | `mgmtGetUser` accepts `id` |
| `backend/internal/api/management_api_test.go` | Lookup by `id` and parameter validation |
| `backend/README.md` | Management API row, OIDC Provider section |
| `AGENTS.md`, `CONTEXT.md` | Scope, decision, data model, endpoints, glossary |
| `scripts/oidc-conformance/` | PKCE proxy, browser driver, suite config template, README |
| `docs/oidc-conformance.md` | Conformance results and explanations |

### Review Focus

- `?id=` with an `email` or `auth_string` also present must be rejected, not resolved by precedence. Covered in Task 28.
- `?id=` naming a User that does not exist answers 404 with the same body as the other lookups, so the id space cannot be probed differently. Covered in Task 28.
- The conformance proxy must only add PKCE when the request has none, so `oidcc-ensure-request-with-valid-pkce-succeeds` still tests the provider's own PKCE handling. Checked in Task 30 by that module passing with the client's challenge.
- The docs must not tell operators to put the issuer on plain http outside loopback. Checked in Task 29 review.

### Task 28: Management API lookup by `id`

**Files:**
- Modify: `backend/internal/api/management_api.go`
- Create: `backend/internal/api/management_api_test.go`
- Modify: `backend/README.md` (Management API table)

- [ ] **Step 1: Write the failing test**

```go
func TestManagementAPILooksUsersUpByID(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "mgmt-id@example.com", "IDLookupSecret")
	h := &Handlers{app: app}

	cases := []struct {
		query  string
		status int
	}{
		{"id=" + user.Id, http.StatusOK},
		{"id=missinguser0000", http.StatusNotFound},
		{"id=" + user.Id + "&email=mgmt-id@example.com", http.StatusBadRequest},
		{"id=" + user.Id + "&auth_string=IDLookupSecret", http.StatusBadRequest},
		{"", http.StatusBadRequest},
	}
	for _, tc := range cases {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/mgmt/users?"+tc.query, nil)
		err := h.mgmtGetUser(&core.RequestEvent{App: app, Event: router.Event{Request: req, Response: res}})
		if status := mgmtStatus(t, err); status != tc.status {
			t.Fatalf("%q: status = %d, want %d", tc.query, status, tc.status)
		}
		if tc.status != http.StatusOK {
			continue
		}
		var body PanelUser
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || body.ID != user.Id || body.AuthString != "IDLookupSecret" {
			t.Fatalf("body = %+v, err = %v", body, err)
		}
	}
}

func mgmtStatus(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return http.StatusOK
	}
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an ApiError", err)
	}
	return apiErr.Status
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `cd backend && go test ./internal/api -run TestManagementAPILooksUsersUpByID`
Expected: FAIL, the `id` case answers 400 because `id` is not read.

- [ ] **Step 3: Implement**

Replace the lookup selection in `mgmtGetUser`:

```go
// mgmtGetUser looks up a single user by User ID, exact email or Current Auth
// String. Exactly one of them must be provided. Connected Apps receive the
// User ID as sub, so apps signing Users in through OIDC look them up by id.
func (h *Handlers) mgmtGetUser(e *core.RequestEvent) error {
	query := e.Request.URL.Query()
	id := strings.TrimSpace(query.Get("id"))
	email := strings.TrimSpace(query.Get("email"))
	authString := strings.TrimSpace(query.Get("auth_string"))
	if countNonEmpty(id, email, authString) > 1 {
		return apis.NewBadRequestError("provide only one of id, email or auth_string", nil)
	}

	var user *core.Record
	var err error
	switch {
	case id != "":
		user, err = h.app.FindRecordById("users", id)
	case email != "":
		user, err = h.app.FindFirstRecordByFilter("users", "email = {:v}", map[string]any{"v": email})
	case authString != "":
		user, err = authstrings.FindCurrentUserByAuthString(h.app, authString)
	default:
		return apis.NewBadRequestError("id, email or auth_string is required", nil)
	}
	// unchanged from here
}

func countNonEmpty(values ...string) int {
	count := 0
	for _, value := range values {
		if value != "" {
			count++
		}
	}
	return count
}
```

- [ ] **Step 4: Run the test and the existing Management API tests**

Run: `cd backend && go test ./internal/api -run 'Management|Mgmt|AnyTLS|AuthStringLifecycle'`
Expected: PASS.

- [ ] **Step 5: Update the README row and commit**

The row becomes: `GET | /api/mgmt/users?id=`、`?email=` 或 `?auth_string=` | 按 User ID、email 或 Current `auth_string` 精确查询单个用户（三选一）...

```bash
git commit -m "feat(backend): look management api users up by id"
```

### Task 29: Documentation

**Files:** `AGENTS.md`, `CONTEXT.md`, `backend/README.md`

- [ ] **Step 1: AGENTS.md**
  - Scope list: the panel is an OIDC provider for admin-registered Connected Apps (ADR-0010).
  - Core decision 11: hand-rolled narrow profile, PKCE S256 required, confidential clients only, `/api/oidc/*` outside OpenAPI and 404 while off, issuer pinned, `oidcUserMayUse` style single check, grant revocation hook, CLI-only key rotation.
  - Data model: the seven `oidc_*` collections and `app_settings.oidc_enabled` / `oidc_issuer`.
  - Endpoints: admin `/oidc-clients`, consent `/oidc/requests/*`, `/users/{id}/oidc-grants`, the protocol endpoints, Management API `id`.
  - Directory tree: `internal/oidc/` and the `oidc_*.go` handler files; frontend `routes/oidc/`, `routes/settings/connected-apps` and `components/authorized-apps.tsx`.
- [ ] **Step 2: CONTEXT.md** adds Connected App and Authorized App Grant, each with an `_Avoid_` line.
- [ ] **Step 3: backend README** adds an "OIDC Provider" section: requirements (`PANEL_BACKEND_URL_BASE` https or loopback, `PANEL_FRONTEND_URL_BASE`), enabling, the issuer pin, the endpoint table, relying party settings, lifetimes, `oidc rotate-signing-key`, and the reverse proxy note.
- [ ] **Step 4: Commit** `docs: document the oidc provider`.

### Task 30: Conformance run

**Files:** `scripts/oidc-conformance/{README.md,pkce-proxy.mjs,driver.mjs,config.template.json,package.json}`, `docs/oidc-conformance.md`

- [ ] **Step 1:** Build and start the suite from a local clone of `gitlab.com/openid/conformance-suite` with its `builder-compose.yml` and `docker-compose-dev.yml`, published at `https://localhost.emobix.co.uk:8443`.
- [ ] **Step 2:** Start the backend on 127.0.0.1:8091 with `PANEL_BACKEND_URL_BASE=http://127.0.0.1:8092`, the PKCE proxy on 127.0.0.1:8092 and the frontend on 127.0.0.1:3001. Create two Connected Apps with redirect URI `https://localhost.emobix.co.uk:8443/test/a/hysterical-panel/callback`, refresh enabled, and two Users.
- [ ] **Step 3:** Run the driver. It creates the plan through the suite API, starts each module, completes sign-in and consent in Playwright when the module waits for the browser, uploads nothing, and writes a JSON summary.
- [ ] **Step 4:** Fix any failure that is a provider bug, with a regression test, in its own commit. Document the rest in `docs/oidc-conformance.md`.
- [ ] **Step 5:** Commit `test: add the oidc conformance harness and results`.

### Layer 5 exit check

- [ ] Gates as in layer 4 Step 1.
- [ ] Review `git diff oidc/4-consent-ui...oidc/5-finish` with `review:code-reviewer`. Fix Critical findings.
- [ ] Update the ledger and report.
