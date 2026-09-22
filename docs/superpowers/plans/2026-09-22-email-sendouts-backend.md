# Email Sendouts Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admins can queue a browser-composed transactional email to one or every active and Verified User. A single in-process worker delivers each Sendout Recipient once through PocketBase SMTP, and admins can cancel or resend.

**Architecture:** A new package `internal/sendouts` owns the two collections' state transitions (create, cancel, requeue, counts) and the rate-limited worker, so none of that lives in HTTP handlers. `internal/api/email_sendouts.go` validates input, expands the audience, and maps records to DTOs. The worker is started from `main.go` next to the Collector and the Monitoring service.

**Tech Stack:** Go 1.26, PocketBase v0.39.0 (framework mode, `core`, `dbx`, `tools/mailer`), kin-openapi v0.139.0.

**Spec:** `docs/superpowers/specs/2026-09-22-email-sendouts-design.md`. Decisions are in `docs/adr/0008-browser-composed-email-sendouts.md`; terms are in `CONTEXT.md` (Email Sendout, Sendout Recipient).

## Global Constraints

- Store every datetime in UTC; raw SQL dates use the layout `2006-01-02 15:04:05.000Z`.
- Recipient status values: `pending`, `sending`, `sent`, `failed`, `skipped`, `cancelled`.
- Recipient reason values: `delivery_failed`, `smtp_disabled`, `interrupted`, `user_ineligible`, `user_deleted`.
- Sendout status is derived, never stored: `cancelled` if `cancelled_at` is set, otherwise `sending` while any row is `pending` or `sending`, otherwise `completed`.
- Eligible recipient means `status = 'active' && verified = true`, including admins.
- `email_sendout_rate_per_minute`: accepted range 1–600, default 30.
- Limits: subject 1–200 runes with no control characters; html ≤ 1,000,000 bytes; text ≤ 256,000 bytes; content JSON ≤ 1 MiB.
- Every endpoint is admin-only, lives under `/api/panel/email-sendouts`, and appears in OpenAPI.
- Creating or resending while `app.Settings().SMTP.Enabled == false` returns 503.
- Each row gets one delivery attempt with no automatic retry. On startup, rows left in `sending` become `failed` / `interrupted`.
- After every task: `go build ./...` and `go vet ./...` pass with no output, and the full backend suite `go test ./...` passes. Run all commands from `backend/`.

## File structure

| File | Responsibility |
|---|---|
| `backend/migrations/1730000027_create_email_sendouts.go` | Create both collections; add and seed `app_settings.email_sendout_rate_per_minute` |
| `backend/internal/sendouts/sendouts.go` | Constants, eligibility, rate helpers |
| `backend/internal/sendouts/queue.go` | `Create`, `CountRecipients`, `Status`, `Cancel`, `Requeue` |
| `backend/internal/sendouts/worker.go` | `Service`: recover, claim, deliver, pace |
| `backend/internal/sendouts/*_test.go` | Schema, queue, and worker tests |
| `backend/internal/api/email_sendouts.go` | HTTP handlers, validation, DTO mapping |
| `backend/internal/api/email_sendouts_test.go` | Handler and validation tests |
| `backend/internal/api/settings.go` | Rate setting read/write |
| `backend/internal/api/dto.go` | Sendout DTOs and the settings field |
| `backend/internal/api/openapi.go` | Schemas and paths |
| `backend/internal/api/api.go` | `Handlers.sendoutQueue`, routes, `Register` signature |
| `backend/main.go` | Construct and start the worker |
| `AGENTS.md`, `backend/README.md` | Documentation |

---

### Task 1: Migration, package constants, and schema test

**Files:**
- Create: `backend/migrations/1730000027_create_email_sendouts.go`
- Create: `backend/internal/sendouts/sendouts.go`
- Create: `backend/internal/sendouts/testutil_test.go`
- Test: `backend/internal/sendouts/migration_test.go`

**Interfaces:**
- Produces: collection names `SendoutsCollection = "email_sendouts"` and `RecipientsCollection = "email_sendout_recipients"`; `AudienceSingle`, `AudienceAll`; `Status*`, `Reason*`, `SendoutStatus*` constants; `Languages`, `RecipientStatuses`, `Reasons` slices; `DefaultRatePerMinute`, `MinRatePerMinute`, `MaxRatePerMinute`; `func Eligible(user *core.Record) bool`; `func CountEligible(app core.App) (int64, error)`; `func FindEligible(app core.App, search string, limit int) ([]*core.Record, error)`; `func NormalizeRate(rate int) int`; `func RatePerMinute(app core.App) int`; `func Interval(rate int) time.Duration`.
- Produces test helpers (package `sendouts`, `_test.go` only): `newTestApp`, `newTestUser`, `testDraft`, `newTestSendout`, `recipientsOf`, `setRecipientStatus`, `reload`. `newTestSendout` depends on `Create` from Task 2, so `testutil_test.go` gains it in Task 2.

- [ ] **Step 1: Write the migration**

Create `backend/migrations/1730000027_create_email_sendouts.go`. Values are hardcoded rather than imported from `internal/sendouts`, so a later constant change cannot rewrite history.

```go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Email Sendouts: one row per administrator-initiated transactional email and
// one row per addressed User. The browser composes html/text; the backend
// stores and sends them unchanged (ADR 0008). Both collections keep nil API
// rules, so only the custom /api/panel routes can reach them.
func init() {
	m.Register(func(app core.App) error {
		if err := addSendoutRateSetting(app); err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		sendouts := core.NewBaseCollection("email_sendouts")
		sendouts.Fields.Add(&core.TextField{Name: "subject", Required: true, Max: 200})
		sendouts.Fields.Add(&core.SelectField{Name: "language", Required: true, MaxSelect: 1, Values: []string{"en", "zh-cn"}})
		sendouts.Fields.Add(&core.SelectField{Name: "audience", Required: true, MaxSelect: 1, Values: []string{"single", "all"}})
		sendouts.Fields.Add(&core.TextField{Name: "html", Required: true, Max: 1_000_000})
		sendouts.Fields.Add(&core.TextField{Name: "text", Required: true, Max: 256_000})
		sendouts.Fields.Add(&core.JSONField{Name: "content", MaxSize: 1 << 20})
		sendouts.Fields.Add(&core.RelationField{Name: "created_by", MaxSelect: 1, CollectionId: users.Id})
		sendouts.Fields.Add(&core.TextField{Name: "created_by_email", Max: 255})
		sendouts.Fields.Add(&core.DateField{Name: "cancelled_at"})
		sendouts.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		sendouts.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		sendouts.AddIndex("idx_email_sendouts_created", false, "created", "")
		if err := app.Save(sendouts); err != nil {
			return err
		}

		recipients := core.NewBaseCollection("email_sendout_recipients")
		recipients.Fields.Add(&core.RelationField{Name: "sendout", Required: true, MaxSelect: 1, CollectionId: sendouts.Id, CascadeDelete: true})
		recipients.Fields.Add(&core.RelationField{Name: "user", MaxSelect: 1, CollectionId: users.Id})
		recipients.Fields.Add(&core.TextField{Name: "email", Required: true, Max: 255})
		recipients.Fields.Add(&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"pending", "sending", "sent", "failed", "skipped", "cancelled"}})
		recipients.Fields.Add(&core.SelectField{Name: "reason", MaxSelect: 1, Values: []string{"delivery_failed", "smtp_disabled", "interrupted", "user_ineligible", "user_deleted"}})
		recipients.Fields.Add(&core.NumberField{Name: "attempts", OnlyInt: true})
		recipients.Fields.Add(&core.DateField{Name: "queued_at", Required: true})
		recipients.Fields.Add(&core.DateField{Name: "last_attempt_at"})
		recipients.Fields.Add(&core.DateField{Name: "sent_at"})
		recipients.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		recipients.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		recipients.AddIndex("idx_email_sendout_recipients_queue", false, "status, queued_at", "")
		recipients.AddIndex("idx_email_sendout_recipients_sendout", false, "sendout, status", "")
		return app.Save(recipients)
	}, func(app core.App) error {
		for _, name := range []string{"email_sendout_recipients", "email_sendouts"} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		settings, err := app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return err
		}
		settings.Fields.RemoveByName("email_sendout_rate_per_minute")
		return app.Save(settings)
	})
}

// addSendoutRateSetting adds the per-minute delivery rate and seeds the
// existing singleton with the default of 30.
func addSendoutRateSetting(app core.App) error {
	settings, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return err
	}
	settings.Fields.Add(&core.NumberField{Name: "email_sendout_rate_per_minute", OnlyInt: true})
	if err := app.Save(settings); err != nil {
		return err
	}
	records, err := app.FindRecordsByFilter("app_settings", "", "", 0, 0)
	if err != nil {
		return err
	}
	for _, rec := range records {
		rec.Set("email_sendout_rate_per_minute", 30)
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: Write the package constants and helpers**

Create `backend/internal/sendouts/sendouts.go`:

```go
// Package sendouts stores Email Sendouts and delivers their Sendout Recipients
// one at a time through PocketBase's SMTP settings (ADR 0008).
package sendouts

import (
	"log"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	SendoutsCollection   = "email_sendouts"
	RecipientsCollection = "email_sendout_recipients"

	AudienceSingle = "single"
	AudienceAll    = "all"

	StatusPending   = "pending"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
	StatusCancelled = "cancelled"

	ReasonDeliveryFailed = "delivery_failed"
	ReasonSMTPDisabled   = "smtp_disabled"
	ReasonInterrupted    = "interrupted"
	ReasonUserIneligible = "user_ineligible"
	ReasonUserDeleted    = "user_deleted"

	SendoutStatusSending   = "sending"
	SendoutStatusCompleted = "completed"
	SendoutStatusCancelled = "cancelled"

	DefaultRatePerMinute = 30
	MinRatePerMinute     = 1
	MaxRatePerMinute     = 600

	dbDateLayout   = "2006-01-02 15:04:05.000Z"
	eligibleFilter = "status = 'active' && verified = true"
)

var (
	Languages         = []string{"en", "zh-cn"}
	RecipientStatuses = []string{StatusPending, StatusSending, StatusSent, StatusFailed, StatusSkipped, StatusCancelled}
	Reasons           = []string{ReasonDeliveryFailed, ReasonSMTPDisabled, ReasonInterrupted, ReasonUserIneligible, ReasonUserDeleted}
)

// Eligible applies the same active + Verified rule that gates panel login and
// Node Client Auth.
func Eligible(user *core.Record) bool {
	return user.GetString("status") == "active" && user.GetBool("verified")
}

func CountEligible(app core.App) (int64, error) {
	return app.CountRecords("users", dbx.NewExp("status = 'active' AND verified = TRUE"))
}

// FindEligible returns eligible Users ordered by email. An empty search matches
// every eligible User; limit 0 means no limit.
func FindEligible(app core.App, search string, limit int) ([]*core.Record, error) {
	if search == "" {
		return app.FindRecordsByFilter("users", eligibleFilter, "email", limit, 0)
	}
	return app.FindRecordsByFilter("users", eligibleFilter+" && email ~ {:search}", "email", limit, 0, dbx.Params{"search": search})
}

// NormalizeRate maps an unset (zero) rate to the default and caps the rest.
func NormalizeRate(rate int) int {
	if rate < MinRatePerMinute {
		return DefaultRatePerMinute
	}
	if rate > MaxRatePerMinute {
		return MaxRatePerMinute
	}
	return rate
}

func RatePerMinute(app core.App) int {
	records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0)
	if err != nil {
		log.Printf("[sendouts] load rate setting: %v", err)
		return DefaultRatePerMinute
	}
	if len(records) == 0 {
		return DefaultRatePerMinute
	}
	return NormalizeRate(records[0].GetInt("email_sendout_rate_per_minute"))
}

// Interval is the gap the worker leaves between two deliveries.
func Interval(rate int) time.Duration {
	return time.Minute / time.Duration(NormalizeRate(rate))
}
```

- [ ] **Step 3: Write the test helpers**

Create `backend/internal/sendouts/testutil_test.go`:

```go
package sendouts

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

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

func newTestUser(t *testing.T, app core.App, email, status string, verified bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.SetPassword("password12345")
	rec.Set("role", "user")
	rec.Set("status", status)
	rec.Set("verified", verified)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return rec
}

func recipientsOf(t *testing.T, app core.App, sendoutID string) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter(RecipientsCollection, "sendout = {:id}", "queued_at,id", 0, 0, dbx.Params{"id": sendoutID})
	if err != nil {
		t.Fatalf("find recipients: %v", err)
	}
	return rows
}

func setRecipientStatus(t *testing.T, app core.App, row *core.Record, status, reason string) {
	t.Helper()
	row.Set("status", status)
	row.Set("reason", reason)
	if err := app.Save(row); err != nil {
		t.Fatalf("set recipient status: %v", err)
	}
}

func reload(t *testing.T, app core.App, collection, id string) *core.Record {
	t.Helper()
	rec, err := app.FindRecordById(collection, id)
	if err != nil {
		t.Fatalf("reload %s/%s: %v", collection, id, err)
	}
	return rec
}
```

- [ ] **Step 4: Write the failing schema test**

Create `backend/internal/sendouts/migration_test.go`:

```go
package sendouts

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestEmailSendoutMigrationSchema(t *testing.T) {
	app := newTestApp(t)

	sendouts, err := app.FindCollectionByNameOrId(SendoutsCollection)
	if err != nil {
		t.Fatalf("find %s: %v", SendoutsCollection, err)
	}
	for _, name := range []string{"subject", "language", "audience", "html", "text", "content", "created_by", "created_by_email", "cancelled_at", "created"} {
		if sendouts.Fields.GetByName(name) == nil {
			t.Errorf("%s missing %q", SendoutsCollection, name)
		}
	}

	recipients, err := app.FindCollectionByNameOrId(RecipientsCollection)
	if err != nil {
		t.Fatalf("find %s: %v", RecipientsCollection, err)
	}
	status, ok := recipients.Fields.GetByName("status").(*core.SelectField)
	if !ok || strings.Join(status.Values, ",") != strings.Join(RecipientStatuses, ",") {
		t.Fatalf("recipient status values must equal %v", RecipientStatuses)
	}
	reason, ok := recipients.Fields.GetByName("reason").(*core.SelectField)
	if !ok || strings.Join(reason.Values, ",") != strings.Join(Reasons, ",") {
		t.Fatalf("recipient reason values must equal %v", Reasons)
	}
	if recipients.GetIndex("idx_email_sendout_recipients_queue") == "" {
		t.Fatal("recipients must index the queue order")
	}
	if sendouts.ListRule != nil || recipients.ListRule != nil || sendouts.ViewRule != nil || recipients.ViewRule != nil {
		t.Fatal("sendout collections must stay superuser-only")
	}
	if got := RatePerMinute(app); got != DefaultRatePerMinute {
		t.Fatalf("seeded rate = %d, want %d", got, DefaultRatePerMinute)
	}
}

func TestNormalizeRateAndInterval(t *testing.T) {
	for rate, want := range map[int]int{0: 30, -5: 30, 1: 1, 45: 45, 600: 600, 601: 600} {
		if got := NormalizeRate(rate); got != want {
			t.Errorf("NormalizeRate(%d) = %d, want %d", rate, got, want)
		}
	}
	if got := Interval(30); got.Seconds() != 2 {
		t.Errorf("Interval(30) = %v, want 2s", got)
	}
	if got := Interval(600); got.Milliseconds() != 100 {
		t.Errorf("Interval(600) = %v, want 100ms", got)
	}
}

func TestEligibleUsers(t *testing.T) {
	app := newTestApp(t)
	newTestUser(t, app, "ops@example.com", "active", true)
	newTestUser(t, app, "dev@example.com", "active", true)
	newTestUser(t, app, "off@example.com", "disabled", true)
	newTestUser(t, app, "new@example.com", "active", false)

	count, err := CountEligible(app)
	if err != nil || count != 2 {
		t.Fatalf("CountEligible() = %d, %v; want 2", count, err)
	}
	found, err := FindEligible(app, "ops", 20)
	if err != nil || len(found) != 1 || found[0].Email() != "ops@example.com" {
		t.Fatalf("FindEligible(ops) = %v, %v; want only ops@example.com", found, err)
	}
	all, err := FindEligible(app, "", 0)
	if err != nil || len(all) != 2 || all[0].Email() != "dev@example.com" {
		t.Fatalf("FindEligible(\"\") = %v, %v; want dev, ops in email order", all, err)
	}
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/sendouts/ -run 'TestEmailSendoutMigrationSchema|TestNormalizeRateAndInterval|TestEligibleUsers' -v`
Expected: PASS. Write the migration first and run the tests after it: without the migration, the schema test fails with `find email_sendouts: ...`. If `CountEligible` returns 0, SQLite is storing `verified` in a form that `TRUE` does not match. In that case switch to `app.CountRecords("users", dbx.HashExp{"status": "active", "verified": true})` and run again.

- [ ] **Step 6: Build, vet, commit**

```bash
go build ./... && go vet ./...
git add backend/migrations/1730000027_create_email_sendouts.go backend/internal/sendouts/
git commit -m "feat(sendouts): add email sendout collections and rate setting"
```

---

### Task 2: Queue operations

**Files:**
- Create: `backend/internal/sendouts/queue.go`
- Modify: `backend/internal/sendouts/testutil_test.go` (add `testDraft`, `newTestSendout`)
- Test: `backend/internal/sendouts/queue_test.go`

**Interfaces:**
- Consumes: Task 1 constants and helpers.
- Produces:
  - `type Draft struct { Subject, Language, Audience, HTML, Text string; Content any }`
  - `func Create(app core.App, draft Draft, users []*core.Record, createdBy *core.Record, now time.Time) (*core.Record, error)`
  - `type Counts struct { Total, Pending, Sent, Failed, Skipped, Cancelled int64 }` (`Pending` includes `sending`)
  - `func CountRecipients(app core.App, sendoutID string) (map[string]Counts, error)` (`""` counts every Sendout)
  - `func Status(sendout *core.Record, counts Counts) string`
  - `var ErrCancelled, ErrNothingToCancel error`
  - `func Cancel(app core.App, sendout *core.Record, now time.Time) error`
  - `func Requeue(app core.App, sendout *core.Record, recipientIDs []string, now time.Time) (int64, error)`

- [ ] **Step 1: Add the Sendout helpers to `testutil_test.go`**

Append the functions below, and add `"time"` to the import block:

```go
func testDraft() Draft {
	return Draft{
		Subject:  "Scheduled maintenance",
		Language: "en",
		Audience: AudienceAll,
		HTML:     "<!DOCTYPE html><html><body><p>Maintenance tonight.</p></body></html>",
		Text:     "Maintenance tonight.",
		Content:  map[string]any{"type": "doc"},
	}
}

func newTestSendout(t *testing.T, app core.App, queuedAt time.Time, users ...*core.Record) *core.Record {
	t.Helper()
	rec, err := Create(app, testDraft(), users, nil, queuedAt)
	if err != nil {
		t.Fatalf("create sendout: %v", err)
	}
	return rec
}
```

- [ ] **Step 2: Write the failing queue tests**

Create `backend/internal/sendouts/queue_test.go`:

```go
package sendouts

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

var testNow = time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)

func TestCreateQueuesOnePendingRecipientPerUser(t *testing.T) {
	app := newTestApp(t)
	admin := newTestUser(t, app, "admin@example.com", "active", true)
	user := newTestUser(t, app, "user@example.com", "active", true)

	sendout, err := Create(app, testDraft(), []*core.Record{admin, user}, admin, testNow)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sendout.GetString("created_by_email") != "admin@example.com" || sendout.GetString("subject") != testDraft().Subject {
		t.Fatalf("sendout = %v, want creator snapshot and draft subject", sendout)
	}
	rows := recipientsOf(t, app, sendout.Id)
	if len(rows) != 2 {
		t.Fatalf("queued %d recipients, want 2", len(rows))
	}
	for _, row := range rows {
		if row.GetString("status") != StatusPending || row.GetInt("attempts") != 0 || !row.GetDateTime("queued_at").Time().Equal(testNow) {
			t.Fatalf("recipient %s = status %q attempts %d queued_at %v", row.Id, row.GetString("status"), row.GetInt("attempts"), row.GetDateTime("queued_at"))
		}
	}
}

func TestCountRecipientsAndStatus(t *testing.T) {
	app := newTestApp(t)
	users := []*core.Record{
		newTestUser(t, app, "a@example.com", "active", true),
		newTestUser(t, app, "b@example.com", "active", true),
		newTestUser(t, app, "c@example.com", "active", true),
		newTestUser(t, app, "d@example.com", "active", true),
	}
	sendout := newTestSendout(t, app, testNow, users...)
	rows := recipientsOf(t, app, sendout.Id)
	setRecipientStatus(t, app, rows[0], StatusSending, "")
	setRecipientStatus(t, app, rows[1], StatusSent, "")
	setRecipientStatus(t, app, rows[2], StatusFailed, ReasonDeliveryFailed)

	counts, err := CountRecipients(app, sendout.Id)
	if err != nil {
		t.Fatalf("CountRecipients() error = %v", err)
	}
	got := counts[sendout.Id]
	if want := (Counts{Total: 4, Pending: 2, Sent: 1, Failed: 1}); got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
	if Status(sendout, got) != SendoutStatusSending {
		t.Fatalf("status with pending rows = %q, want sending", Status(sendout, got))
	}
	if Status(sendout, Counts{Total: 4, Sent: 4}) != SendoutStatusCompleted {
		t.Fatal("status without pending rows must be completed")
	}
	sendout.Set("cancelled_at", testNow)
	if Status(sendout, got) != SendoutStatusCancelled {
		t.Fatal("status with cancelled_at must be cancelled")
	}
}

func TestCancelOnlyTouchesPendingRecipients(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow,
		newTestUser(t, app, "a@example.com", "active", true),
		newTestUser(t, app, "b@example.com", "active", true),
		newTestUser(t, app, "c@example.com", "active", true),
	)
	rows := recipientsOf(t, app, sendout.Id)
	setRecipientStatus(t, app, rows[1], StatusSending, "")
	setRecipientStatus(t, app, rows[2], StatusSent, "")

	if err := Cancel(app, sendout, testNow); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	want := []string{StatusCancelled, StatusSending, StatusSent}
	for i, row := range rows {
		if got := reload(t, app, RecipientsCollection, row.Id).GetString("status"); got != want[i] {
			t.Errorf("row %d status = %q, want %q", i, got, want[i])
		}
	}
	if reload(t, app, SendoutsCollection, sendout.Id).GetDateTime("cancelled_at").IsZero() {
		t.Fatal("Cancel() did not set cancelled_at")
	}
	if err := Cancel(app, sendout, testNow); !errors.Is(err, ErrCancelled) {
		t.Fatalf("second Cancel() error = %v, want ErrCancelled", err)
	}
}

func TestCancelRejectsFinishedSendout(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow, newTestUser(t, app, "a@example.com", "active", true))
	setRecipientStatus(t, app, recipientsOf(t, app, sendout.Id)[0], StatusSent, "")

	if err := Cancel(app, sendout, testNow); !errors.Is(err, ErrNothingToCancel) {
		t.Fatalf("Cancel() error = %v, want ErrNothingToCancel", err)
	}
}

func TestRequeueMovesFailedRecipientsToQueueTail(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow,
		newTestUser(t, app, "a@example.com", "active", true),
		newTestUser(t, app, "b@example.com", "active", true),
		newTestUser(t, app, "c@example.com", "active", true),
	)
	rows := recipientsOf(t, app, sendout.Id)
	setRecipientStatus(t, app, rows[0], StatusFailed, ReasonDeliveryFailed)
	setRecipientStatus(t, app, rows[1], StatusFailed, ReasonSMTPDisabled)
	setRecipientStatus(t, app, rows[2], StatusSkipped, ReasonUserIneligible)
	later := testNow.Add(time.Hour)

	n, err := Requeue(app, sendout, []string{rows[0].Id}, later)
	if err != nil || n != 1 {
		t.Fatalf("Requeue(one) = %d, %v; want 1", n, err)
	}
	first := reload(t, app, RecipientsCollection, rows[0].Id)
	if first.GetString("status") != StatusPending || first.GetString("reason") != "" || !first.GetDateTime("queued_at").Time().Equal(later) {
		t.Fatalf("requeued row = status %q reason %q queued_at %v", first.GetString("status"), first.GetString("reason"), first.GetDateTime("queued_at"))
	}
	n, err = Requeue(app, sendout, nil, later)
	if err != nil || n != 1 {
		t.Fatalf("Requeue(all failed) = %d, %v; want 1", n, err)
	}
	if got := reload(t, app, RecipientsCollection, rows[2].Id).GetString("status"); got != StatusSkipped {
		t.Fatalf("skipped row status = %q, want skipped", got)
	}
}

func TestRequeueRejectsCancelledSendout(t *testing.T) {
	app := newTestApp(t)
	sendout := newTestSendout(t, app, testNow, newTestUser(t, app, "a@example.com", "active", true))
	if err := Cancel(app, sendout, testNow); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if _, err := Requeue(app, sendout, nil, testNow); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Requeue() error = %v, want ErrCancelled", err)
	}
}

func TestDeletingUsersKeepsSendoutHistory(t *testing.T) {
	app := newTestApp(t)
	admin := newTestUser(t, app, "admin@example.com", "active", true)
	user := newTestUser(t, app, "user@example.com", "active", true)
	sendout, err := Create(app, testDraft(), []*core.Record{user}, admin, testNow)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := app.Delete(user); err != nil {
		t.Fatalf("delete recipient user: %v", err)
	}
	if err := app.Delete(admin); err != nil {
		t.Fatalf("delete creator: %v", err)
	}

	rows := recipientsOf(t, app, sendout.Id)
	if len(rows) != 1 || rows[0].GetString("user") != "" || rows[0].GetString("email") != "user@example.com" {
		t.Fatalf("recipient after user delete = %v, want kept row with cleared user", rows)
	}
	kept := reload(t, app, SendoutsCollection, sendout.Id)
	if kept.GetString("created_by") != "" || kept.GetString("created_by_email") != "admin@example.com" {
		t.Fatalf("sendout after creator delete = %v, want cleared relation and kept email", kept)
	}
}
```

- [ ] **Step 3: Run the tests to confirm they fail**

Run: `go test ./internal/sendouts/ -run 'TestCreate|TestCount|TestCancel|TestRequeue|TestDeleting' -v`
Expected: build failure. `Create`, `Draft`, `Counts`, `CountRecipients`, `Status`, `Cancel`, `Requeue`, `ErrCancelled` and `ErrNothingToCancel` are undefined.

- [ ] **Step 4: Implement `queue.go`**

Create `backend/internal/sendouts/queue.go`:

```go
package sendouts

import (
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var (
	ErrCancelled       = errors.New("email sendout is cancelled")
	ErrNothingToCancel = errors.New("email sendout has no pending recipients")
)

// Draft is a validated Sendout as submitted by the browser. HTML and Text are
// stored and sent unchanged.
type Draft struct {
	Subject  string
	Language string
	Audience string
	HTML     string
	Text     string
	Content  any
}

// Create stores a Sendout and queues one pending Sendout Recipient per User in
// one transaction, so a Sendout never exists with a partial audience.
func Create(app core.App, draft Draft, users []*core.Record, createdBy *core.Record, now time.Time) (*core.Record, error) {
	var sendout *core.Record
	err := app.RunInTransaction(func(tx core.App) error {
		rec, err := saveSendout(tx, draft, createdBy)
		if err != nil {
			return err
		}
		sendout = rec
		return queueRecipients(tx, rec.Id, users, now)
	})
	return sendout, err
}

func saveSendout(app core.App, draft Draft, createdBy *core.Record) (*core.Record, error) {
	coll, err := app.FindCollectionByNameOrId(SendoutsCollection)
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(coll)
	rec.Set("subject", draft.Subject)
	rec.Set("language", draft.Language)
	rec.Set("audience", draft.Audience)
	rec.Set("html", draft.HTML)
	rec.Set("text", draft.Text)
	rec.Set("content", draft.Content)
	if createdBy != nil {
		rec.Set("created_by", createdBy.Id)
		rec.Set("created_by_email", createdBy.Email())
	}
	return rec, app.Save(rec)
}

func queueRecipients(app core.App, sendoutID string, users []*core.Record, now time.Time) error {
	coll, err := app.FindCollectionByNameOrId(RecipientsCollection)
	if err != nil {
		return err
	}
	for _, user := range users {
		rec := core.NewRecord(coll)
		rec.Set("sendout", sendoutID)
		rec.Set("user", user.Id)
		rec.Set("email", user.Email())
		rec.Set("status", StatusPending)
		rec.Set("queued_at", now)
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// Counts summarises a Sendout's recipients. Pending includes rows the worker
// is sending right now.
type Counts struct {
	Total     int64
	Pending   int64
	Sent      int64
	Failed    int64
	Skipped   int64
	Cancelled int64
}

func (c *Counts) add(status string, n int64) {
	c.Total += n
	switch status {
	case StatusPending, StatusSending:
		c.Pending += n
	case StatusSent:
		c.Sent += n
	case StatusFailed:
		c.Failed += n
	case StatusSkipped:
		c.Skipped += n
	case StatusCancelled:
		c.Cancelled += n
	}
}

type statusCount struct {
	Sendout string `db:"sendout"`
	Status  string `db:"status"`
	N       int64  `db:"n"`
}

// CountRecipients returns per-status counts keyed by Sendout ID. An empty
// sendoutID counts every Sendout.
func CountRecipients(app core.App, sendoutID string) (map[string]Counts, error) {
	query := "SELECT sendout, status, COUNT(*) AS n FROM " + RecipientsCollection
	params := dbx.Params{}
	if sendoutID != "" {
		query += " WHERE sendout = {:id}"
		params["id"] = sendoutID
	}
	var rows []statusCount
	if err := app.DB().NewQuery(query + " GROUP BY sendout, status").Bind(params).All(&rows); err != nil {
		return nil, err
	}
	out := map[string]Counts{}
	for _, row := range rows {
		c := out[row.Sendout]
		c.add(row.Status, row.N)
		out[row.Sendout] = c
	}
	return out, nil
}

// Status is derived from the recipients rather than stored, so it always
// matches what the worker, cancel and resend did.
func Status(sendout *core.Record, counts Counts) string {
	if !sendout.GetDateTime("cancelled_at").IsZero() {
		return SendoutStatusCancelled
	}
	if counts.Pending > 0 {
		return SendoutStatusSending
	}
	return SendoutStatusCompleted
}

// Cancel stops the rows the worker has not claimed yet. A row already in
// `sending` finishes normally.
func Cancel(app core.App, sendout *core.Record, now time.Time) error {
	if !sendout.GetDateTime("cancelled_at").IsZero() {
		return ErrCancelled
	}
	counts, err := CountRecipients(app, sendout.Id)
	if err != nil {
		return err
	}
	if counts[sendout.Id].Pending == 0 {
		return ErrNothingToCancel
	}
	return app.RunInTransaction(func(tx core.App) error {
		_, err := tx.DB().Update(RecipientsCollection,
			dbx.Params{"status": StatusCancelled, "updated": now.Format(dbDateLayout)},
			dbx.HashExp{"sendout": sendout.Id, "status": StatusPending},
		).Execute()
		if err != nil {
			return err
		}
		sendout.Set("cancelled_at", now)
		return tx.Save(sendout)
	})
}

// Requeue puts failed rows back at the tail of the queue. With no IDs it
// requeues every failed row of the Sendout; other statuses are never touched.
func Requeue(app core.App, sendout *core.Record, recipientIDs []string, now time.Time) (int64, error) {
	if !sendout.GetDateTime("cancelled_at").IsZero() {
		return 0, ErrCancelled
	}
	var where dbx.Expression = dbx.HashExp{"sendout": sendout.Id, "status": StatusFailed}
	if len(recipientIDs) > 0 {
		ids := make([]any, len(recipientIDs))
		for i, id := range recipientIDs {
			ids[i] = id
		}
		where = dbx.And(where, dbx.In("id", ids...))
	}
	stamp := now.Format(dbDateLayout)
	res, err := app.DB().Update(RecipientsCollection,
		dbx.Params{"status": StatusPending, "reason": "", "queued_at": stamp, "updated": stamp},
		where,
	).Execute()
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/sendouts/ -v`
Expected: PASS. If `TestDeletingUsersKeepsSendoutHistory` fails because PocketBase refuses the delete, one of the relations was accidentally marked `Required`. Both `user` and `created_by` must stay optional, so check the migration.

- [ ] **Step 6: Build, vet, commit**

```bash
go build ./... && go vet ./...
git add backend/internal/sendouts/
git commit -m "feat(sendouts): add create, cancel and requeue queue operations"
```

---

### Task 3: Worker

**Files:**
- Create: `backend/internal/sendouts/worker.go`
- Test: `backend/internal/sendouts/worker_test.go`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces:
  - `type Service struct` (unexported fields `app`, `send`, `now`, `wake`)
  - `func New(app core.App) *Service`
  - `func (s *Service) Start(ctx context.Context)`
  - `func (s *Service) Notify()` (non-blocking)
  - `func (s *Service) ProcessNext() (bool, error)`
  - `func (s *Service) RecoverInterrupted() error`

- [ ] **Step 1: Write the failing worker tests**

Create `backend/internal/sendouts/worker_test.go`:

```go
package sendouts

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tools/mailer"
)

func newTestService(t *testing.T, sendErr error) (*Service, *[]*mailer.Message) {
	t.Helper()
	app := newTestApp(t)
	app.Settings().SMTP.Enabled = true
	sent := []*mailer.Message{}
	s := New(app)
	s.send = func(msg *mailer.Message) error {
		sent = append(sent, msg)
		return sendErr
	}
	return s, &sent
}

func TestProcessNextSendsStoredContentToCurrentEmail(t *testing.T) {
	s, sent := newTestService(t, nil)
	user := newTestUser(t, s.app, "old@example.com", "active", true)
	sendout := newTestSendout(t, s.app, testNow, user)
	user.Set("email", "new@example.com")
	if err := s.app.Save(user); err != nil {
		t.Fatalf("change email: %v", err)
	}

	processed, err := s.ProcessNext()
	if err != nil || !processed {
		t.Fatalf("ProcessNext() = %v, %v; want true, nil", processed, err)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(*sent))
	}
	msg := (*sent)[0]
	if msg.To[0].Address != "new@example.com" || msg.Subject != sendout.GetString("subject") ||
		msg.HTML != sendout.GetString("html") || msg.Text != sendout.GetString("text") {
		t.Fatalf("message = %+v, want stored content sent to the current email", msg)
	}
	row := recipientsOf(t, s.app, sendout.Id)[0]
	if row.GetString("status") != StatusSent || row.GetInt("attempts") != 1 ||
		row.GetDateTime("sent_at").IsZero() || row.GetString("email") != "new@example.com" {
		t.Fatalf("recipient = status %q attempts %d sent_at %v email %q",
			row.GetString("status"), row.GetInt("attempts"), row.GetDateTime("sent_at"), row.GetString("email"))
	}
}

func TestProcessNextSkipsUsersWhoLostEligibility(t *testing.T) {
	s, sent := newTestService(t, nil)
	disabled := newTestUser(t, s.app, "disabled@example.com", "active", true)
	deleted := newTestUser(t, s.app, "deleted@example.com", "active", true)
	sendout := newTestSendout(t, s.app, testNow, disabled, deleted)
	disabled.Set("status", "disabled")
	if err := s.app.Save(disabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if err := s.app.Delete(deleted); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := s.ProcessNext(); err != nil {
			t.Fatalf("ProcessNext() error = %v", err)
		}
	}
	if len(*sent) != 0 {
		t.Fatalf("sent %d messages, want 0", len(*sent))
	}
	got := map[string]string{}
	for _, row := range recipientsOf(t, s.app, sendout.Id) {
		got[row.GetString("email")] = row.GetString("status") + "/" + row.GetString("reason")
	}
	if got["disabled@example.com"] != "skipped/user_ineligible" || got["deleted@example.com"] != "skipped/user_deleted" {
		t.Fatalf("outcomes = %v", got)
	}
}

func TestProcessNextFailsWhileSMTPIsDisabled(t *testing.T) {
	s, sent := newTestService(t, nil)
	s.app.Settings().SMTP.Enabled = false
	sendout := newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "a@example.com", "active", true))

	if _, err := s.ProcessNext(); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	row := recipientsOf(t, s.app, sendout.Id)[0]
	if len(*sent) != 0 || row.GetString("status") != StatusFailed || row.GetString("reason") != ReasonSMTPDisabled {
		t.Fatalf("sent %d, recipient %s/%s; want failed/smtp_disabled", len(*sent), row.GetString("status"), row.GetString("reason"))
	}
}

func TestProcessNextRecordsDeliveryFailure(t *testing.T) {
	s, _ := newTestService(t, errors.New("421 service not available"))
	sendout := newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "a@example.com", "active", true))

	if _, err := s.ProcessNext(); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	row := recipientsOf(t, s.app, sendout.Id)[0]
	if row.GetString("status") != StatusFailed || row.GetString("reason") != ReasonDeliveryFailed || row.GetInt("attempts") != 1 {
		t.Fatalf("recipient = %s/%s attempts %d; want failed/delivery_failed attempts 1",
			row.GetString("status"), row.GetString("reason"), row.GetInt("attempts"))
	}
}

func TestProcessNextFollowsQueueOrder(t *testing.T) {
	s, sent := newTestService(t, nil)
	newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "late@example.com", "active", true))
	newTestSendout(t, s.app, testNow.Add(-time.Minute), newTestUser(t, s.app, "early@example.com", "active", true))

	for i := 0; i < 2; i++ {
		if _, err := s.ProcessNext(); err != nil {
			t.Fatalf("ProcessNext() error = %v", err)
		}
	}
	if len(*sent) != 2 || (*sent)[0].To[0].Address != "early@example.com" {
		t.Fatalf("first delivery went to %v, want early@example.com", (*sent)[0].To)
	}
}

func TestProcessNextReportsEmptyQueue(t *testing.T) {
	s, _ := newTestService(t, nil)
	processed, err := s.ProcessNext()
	if err != nil || processed {
		t.Fatalf("ProcessNext() on empty queue = %v, %v; want false, nil", processed, err)
	}
}

func TestRecoverInterruptedFailsSendingRows(t *testing.T) {
	s, _ := newTestService(t, nil)
	sendout := newTestSendout(t, s.app, testNow, newTestUser(t, s.app, "a@example.com", "active", true))
	row := recipientsOf(t, s.app, sendout.Id)[0]
	setRecipientStatus(t, s.app, row, StatusSending, "")

	if err := s.RecoverInterrupted(); err != nil {
		t.Fatalf("RecoverInterrupted() error = %v", err)
	}
	row = reload(t, s.app, RecipientsCollection, row.Id)
	if row.GetString("status") != StatusFailed || row.GetString("reason") != ReasonInterrupted {
		t.Fatalf("recovered row = %s/%s; want failed/interrupted", row.GetString("status"), row.GetString("reason"))
	}
}

func TestNotifyDoesNotBlock(t *testing.T) {
	s, _ := newTestService(t, nil)
	s.Notify()
	s.Notify()
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/sendouts/ -run 'TestProcessNext|TestRecoverInterrupted|TestNotify' -v`
Expected: build failure because `New` and `Service` are undefined.

- [ ] **Step 3: Implement `worker.go`**

Create `backend/internal/sendouts/worker.go`:

```go
package sendouts

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/mail"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

const idlePoll = 5 * time.Second

// Service delivers Sendout Recipients one at a time in queue order.
type Service struct {
	app  core.App
	send func(*mailer.Message) error
	now  func() time.Time
	wake chan struct{}
}

func New(app core.App) *Service {
	return &Service{
		app:  app,
		send: func(msg *mailer.Message) error { return app.NewMailClient().Send(msg) },
		now:  func() time.Time { return time.Now().UTC() },
		wake: make(chan struct{}, 1),
	}
}

// Notify wakes an idle worker after rows are queued, so a new Sendout starts
// without waiting for the next idle poll.
func (s *Service) Notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *Service) run(ctx context.Context) {
	if err := s.RecoverInterrupted(); err != nil {
		log.Printf("[sendouts] recover interrupted recipients: %v", err)
	}
	for {
		processed, err := s.ProcessNext()
		if err != nil {
			log.Printf("[sendouts] process recipient: %v", err)
		}
		if !s.wait(ctx, processed) {
			return
		}
	}
}

// wait paces deliveries at the configured rate. Only an idle worker listens
// to Notify, so a wake-up cannot shorten the gap between two emails.
func (s *Service) wait(ctx context.Context, processed bool) bool {
	delay, wake := idlePoll, s.wake
	if processed {
		delay, wake = Interval(RatePerMinute(s.app)), nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-wake:
		return true
	case <-timer.C:
		return true
	}
}

// RecoverInterrupted fails rows a previous process claimed but never
// finished. The email may already have gone out, so they are not resent
// automatically.
func (s *Service) RecoverInterrupted() error {
	_, err := s.app.DB().Update(RecipientsCollection,
		dbx.Params{"status": StatusFailed, "reason": ReasonInterrupted, "updated": s.now().Format(dbDateLayout)},
		dbx.HashExp{"status": StatusSending},
	).Execute()
	return err
}

// ProcessNext delivers the oldest pending row. It reports false when the
// queue is empty or the row was cancelled before the worker claimed it.
func (s *Service) ProcessNext() (bool, error) {
	row, err := s.claimNext()
	if err != nil || row == nil {
		return false, err
	}
	status, reason := s.deliver(row)
	return true, s.finish(row, status, reason)
}

// claimNext moves the oldest pending row to `sending` with a guarded UPDATE,
// so a concurrent Cancel either wins before the claim or leaves the row alone.
func (s *Service) claimNext() (*core.Record, error) {
	rows, err := s.app.FindRecordsByFilter(RecipientsCollection, "status = 'pending'", "queued_at,id", 1, 0)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	stamp := s.now().Format(dbDateLayout)
	res, err := s.app.DB().NewQuery(
		"UPDATE " + RecipientsCollection + " SET status = 'sending', attempts = attempts + 1, last_attempt_at = {:now}, updated = {:now} WHERE id = {:id} AND status = 'pending'",
	).Bind(dbx.Params{"now": stamp, "id": rows[0].Id}).Execute()
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return nil, err
	}
	return s.app.FindRecordById(RecipientsCollection, rows[0].Id)
}

// deliver returns the row's final status and reason.
func (s *Service) deliver(row *core.Record) (string, string) {
	user, status, reason := s.recipientUser(row)
	if user == nil {
		return status, reason
	}
	if !s.app.Settings().SMTP.Enabled {
		return StatusFailed, ReasonSMTPDisabled
	}
	sendout, err := s.app.FindRecordById(SendoutsCollection, row.GetString("sendout"))
	if err != nil {
		log.Printf("[sendouts] load sendout for recipient %s: %v", row.Id, err)
		return StatusFailed, ReasonDeliveryFailed
	}
	row.Set("email", user.Email())
	if err := s.send(s.message(sendout, user.Email())); err != nil {
		log.Printf("[sendouts] deliver recipient %s of sendout %s: %v", row.Id, sendout.Id, err)
		return StatusFailed, ReasonDeliveryFailed
	}
	return StatusSent, ""
}

// recipientUser rechecks eligibility at send time: a User disabled, unverified
// or deleted since the Sendout was created is skipped rather than emailed.
func (s *Service) recipientUser(row *core.Record) (*core.Record, string, string) {
	userID := row.GetString("user")
	if userID == "" {
		return nil, StatusSkipped, ReasonUserDeleted
	}
	user, err := s.app.FindRecordById("users", userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, StatusSkipped, ReasonUserDeleted
	}
	if err != nil {
		log.Printf("[sendouts] load user for recipient %s: %v", row.Id, err)
		return nil, StatusFailed, ReasonDeliveryFailed
	}
	if !Eligible(user) {
		return nil, StatusSkipped, ReasonUserIneligible
	}
	return user, "", ""
}

func (s *Service) message(sendout *core.Record, to string) *mailer.Message {
	meta := s.app.Settings().Meta
	return &mailer.Message{
		From:    mail.Address{Name: meta.SenderName, Address: meta.SenderAddress},
		To:      []mail.Address{{Address: to}},
		Subject: sendout.GetString("subject"),
		HTML:    sendout.GetString("html"),
		Text:    sendout.GetString("text"),
	}
}

func (s *Service) finish(row *core.Record, status, reason string) error {
	row.Set("status", status)
	row.Set("reason", reason)
	if status == StatusSent {
		row.Set("sent_at", s.now())
	}
	return s.app.Save(row)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/sendouts/ -v`
Expected: PASS for every test in the package.

- [ ] **Step 5: Build, vet, commit**

```bash
go build ./... && go vet ./...
git add backend/internal/sendouts/
git commit -m "feat(sendouts): deliver queued recipients one at a time"
```

---

### Task 4: Rate setting in the settings API

**Files:**
- Modify: `backend/internal/api/settings.go`
- Modify: `backend/internal/api/dto.go` (`SettingsResponse`, `SettingsUpdateRequest`)
- Test: `backend/internal/api/email_sendouts_test.go` (create the file here; Tasks 5 and 6 append to it)

**Interfaces:**
- Consumes: `sendouts.NormalizeRate`, `sendouts.DefaultRatePerMinute`, `sendouts.MinRatePerMinute`, `sendouts.MaxRatePerMinute`.
- Produces: `SettingsResponse.EmailSendoutRatePerMinute int` (`json:"email_sendout_rate_per_minute"`), `SettingsUpdateRequest.EmailSendoutRatePerMinute *int` (`json:"email_sendout_rate_per_minute,omitempty"`). Test helpers `sendoutEvent` and `sendoutAPIStatus` for Tasks 5–6.

- [ ] **Step 1: Write the failing test and shared helpers**

Create `backend/internal/api/email_sendouts_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func sendoutEvent(
	t *testing.T,
	app core.App,
	method, target, id string,
	body any,
	auth *core.Record,
) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	raw := []byte("{}")
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	e := &core.RequestEvent{App: app, Event: router.Event{Request: req, Response: response}}
	e.Auth = auth
	return e, response
}

func sendoutAPIStatus(t *testing.T, err error) int {
	t.Helper()
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an ApiError", err)
	}
	return apiErr.Status
}

func TestSettingsEmailSendoutRate(t *testing.T) {
	app := newMigratedTestApp(t)
	h := &Handlers{app: app}

	e, response := sendoutEvent(t, app, http.MethodGet, "/api/panel/settings", "", nil, nil)
	if err := h.getSettings(e); err != nil {
		t.Fatalf("getSettings() error = %v", err)
	}
	var current SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil || current.EmailSendoutRatePerMinute != 30 {
		t.Fatalf("default rate = %d (%v), want 30", current.EmailSendoutRatePerMinute, err)
	}

	for _, rate := range []int{0, 601} {
		e, _ := sendoutEvent(t, app, http.MethodPatch, "/api/panel/settings", "", SettingsUpdateRequest{EmailSendoutRatePerMinute: &rate}, nil)
		if err := h.updateSettings(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
			t.Fatalf("updateSettings(rate=%d) error = %v, want 400", rate, err)
		}
	}

	rate := 45
	e, response = sendoutEvent(t, app, http.MethodPatch, "/api/panel/settings", "", SettingsUpdateRequest{EmailSendoutRatePerMinute: &rate}, nil)
	if err := h.updateSettings(e); err != nil {
		t.Fatalf("updateSettings(rate=45) error = %v", err)
	}
	var updated SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil || updated.EmailSendoutRatePerMinute != 45 {
		t.Fatalf("updated rate = %d (%v), want 45", updated.EmailSendoutRatePerMinute, err)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./internal/api/ -run TestSettingsEmailSendoutRate -v`
Expected: build failure because `EmailSendoutRatePerMinute` is an unknown field.

- [ ] **Step 3: Add the DTO fields**

In `backend/internal/api/dto.go`, add this field as the last one in `SettingsResponse`:

```go
	EmailSendoutRatePerMinute int `json:"email_sendout_rate_per_minute"`
```

Add this field as the last one in `SettingsUpdateRequest`:

```go
	EmailSendoutRatePerMinute *int `json:"email_sendout_rate_per_minute,omitempty"`
```

- [ ] **Step 4: Read and write the setting**

In `backend/internal/api/settings.go`:

1. Add `"hysterical-panel/internal/sendouts"` to the imports.
2. Add the field `EmailSendoutRatePerMinute int` as the last field of `type settings struct`.
3. In `settingsFromRecord`, add `EmailSendoutRatePerMinute: rec.GetInt("email_sendout_rate_per_minute"),`.
4. In `settingsRecord`, before `h.app.Save(rec)` on the lazily created record, add `rec.Set("email_sendout_rate_per_minute", sendouts.DefaultRatePerMinute)`.
5. In `settingsResponse`, add `EmailSendoutRatePerMinute: sendouts.NormalizeRate(s.EmailSendoutRatePerMinute),`.
6. In `updateSettings`, insert this block right before the `// Management API: enabling auto-generates ...` comment:

```go
	if in.EmailSendoutRatePerMinute != nil {
		rate := *in.EmailSendoutRatePerMinute
		if rate < sendouts.MinRatePerMinute || rate > sendouts.MaxRatePerMinute {
			return apis.NewBadRequestError("email_sendout_rate_per_minute must be between 1 and 600", nil)
		}
		rec.Set("email_sendout_rate_per_minute", rate)
	}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/api/ -run TestSettingsEmailSendoutRate -v`
Expected: PASS.

- [ ] **Step 6: Build, vet, full suite, commit**

```bash
go build ./... && go vet ./... && go test ./...
git add backend/internal/api/settings.go backend/internal/api/dto.go backend/internal/api/email_sendouts_test.go
git commit -m "feat(settings): add email sendout rate per minute"
```

---

### Task 5: Composer endpoints (context, candidates, create) and worker wiring

**Files:**
- Create: `backend/internal/api/email_sendouts.go`
- Modify: `backend/internal/api/dto.go`
- Modify: `backend/internal/api/api.go` (`Handlers`, `Register`, routes)
- Modify: `backend/main.go`
- Test: `backend/internal/api/email_sendouts_test.go`

**Interfaces:**
- Consumes: `sendouts.Create`, `sendouts.FindEligible`, `sendouts.CountEligible`, `sendouts.Eligible`, `sendouts.RatePerMinute`, `sendouts.CountRecipients`, `sendouts.Status`, `sendouts.New`, `(*sendouts.Service).Start`, `(*sendouts.Service).Notify`.
- Produces:
  - DTOs `EmailSendoutContextResponse`, `EmailSendoutRecipientCandidate`, `EmailSendoutCreateRequest`, `EmailSendoutCounts`, `EmailSendout`
  - `type sendoutNotifier interface{ Notify() }`; field `Handlers.sendoutQueue sendoutNotifier`
  - `func validateEmailSendout(in EmailSendoutCreateRequest) (sendouts.Draft, error)`
  - `func publicEmailSendout(rec *core.Record, counts sendouts.Counts) EmailSendout`
  - `func (h *Handlers) respondEmailSendout(e *core.RequestEvent, rec *core.Record) error`
  - `func errSMTPUnavailable() error`
  - New `Register` signature: `Register(se *core.ServeEvent, app core.App, box *cryptobox.Box, ipLookup ipMetadataLookup, passkeys *webauthn.WebAuthn, monitoring monitorLifecycle, sendoutQueue sendoutNotifier, public PanelConfigResponse) *Handlers`
  - Routes `GET /email-sendouts/context`, `GET /email-sendouts/eligible-recipients`, `POST /email-sendouts`

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/api/email_sendouts_test.go`, and add `"strings"` and `"hysterical-panel/internal/sendouts"` to its imports:

```go
type fakeSendoutNotifier struct{ calls int }

func (f *fakeSendoutNotifier) Notify() { f.calls++ }

func newSendoutTestHandlers(t *testing.T) (*Handlers, *fakeSendoutNotifier, *core.Record) {
	t.Helper()
	app := newMigratedTestApp(t)
	app.Settings().SMTP.Enabled = true
	admin := newUsersTestRecord(t, app, "admin@example.com", "AdminSecret")
	admin.Set("role", "admin")
	if err := app.Save(admin); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	notifier := &fakeSendoutNotifier{}
	return &Handlers{app: app, sendoutQueue: notifier}, notifier, admin
}

func newSendoutTestUser(t *testing.T, app core.App, email, status string, verified bool) *core.Record {
	t.Helper()
	user := newUsersTestRecord(t, app, email, strings.ReplaceAll(email, "@", "-at-")+"-secret")
	user.Set("status", status)
	user.Set("verified", verified)
	if err := app.Save(user); err != nil {
		t.Fatalf("update test user: %v", err)
	}
	return user
}

func validSendoutRequest() EmailSendoutCreateRequest {
	return EmailSendoutCreateRequest{
		Subject:  "  Scheduled maintenance  ",
		Language: "en",
		Audience: "all",
		HTML:     "<!DOCTYPE html><html><body><p>Maintenance tonight.</p></body></html>",
		Text:     "Maintenance tonight.",
		Content:  map[string]any{"type": "doc"},
	}
}

func TestValidateEmailSendout(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*EmailSendoutCreateRequest)
		ok     bool
	}{
		{"valid broadcast", func(*EmailSendoutCreateRequest) {}, true},
		{"valid single", func(in *EmailSendoutCreateRequest) { in.Audience, in.UserID = "single", "user1" }, true},
		{"blank subject", func(in *EmailSendoutCreateRequest) { in.Subject = "   " }, false},
		{"header injection", func(in *EmailSendoutCreateRequest) { in.Subject = "Hi\r\nBcc: x@example.com" }, false},
		{"long subject", func(in *EmailSendoutCreateRequest) { in.Subject = strings.Repeat("a", 201) }, false},
		{"unknown language", func(in *EmailSendoutCreateRequest) { in.Language = "fr" }, false},
		{"single without user", func(in *EmailSendoutCreateRequest) { in.Audience = "single" }, false},
		{"broadcast with user", func(in *EmailSendoutCreateRequest) { in.UserID = "user1" }, false},
		{"unknown audience", func(in *EmailSendoutCreateRequest) { in.Audience = "admins" }, false},
		{"blank html", func(in *EmailSendoutCreateRequest) { in.HTML = " " }, false},
		{"blank text", func(in *EmailSendoutCreateRequest) { in.Text = "" }, false},
		{"oversized html", func(in *EmailSendoutCreateRequest) { in.HTML = strings.Repeat("a", 1_000_001) }, false},
		{"missing content", func(in *EmailSendoutCreateRequest) { in.Content = nil }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validSendoutRequest()
			tc.mutate(&in)
			draft, err := validateEmailSendout(in)
			if tc.ok != (err == nil) {
				t.Fatalf("validateEmailSendout() error = %v, want ok=%v", err, tc.ok)
			}
			if tc.ok && draft.Subject != "Scheduled maintenance" {
				t.Fatalf("subject = %q, want trimmed", draft.Subject)
			}
		})
	}
}

func TestCreateEmailSendoutQueuesEligibleUsersOnly(t *testing.T) {
	h, notifier, admin := newSendoutTestHandlers(t)
	newSendoutTestUser(t, h.app, "active@example.com", "active", true)
	newSendoutTestUser(t, h.app, "disabled@example.com", "disabled", true)
	newSendoutTestUser(t, h.app, "unverified@example.com", "active", false)

	e, response := sendoutEvent(t, h.app, http.MethodPost, "/api/panel/email-sendouts", "", validSendoutRequest(), admin)
	if err := h.createEmailSendout(e); err != nil {
		t.Fatalf("createEmailSendout() error = %v", err)
	}
	var created EmailSendout
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Counts.Total != 2 || created.Counts.Pending != 2 || created.Status != "sending" || created.CreatedByEmail != "admin@example.com" {
		t.Fatalf("created = %+v, want 2 pending recipients created by admin", created)
	}
	if notifier.calls != 1 {
		t.Fatalf("worker notified %d times, want 1", notifier.calls)
	}
	rows, err := h.app.FindRecordsByFilter(sendouts.RecipientsCollection, "", "email", 0, 0)
	if err != nil {
		t.Fatalf("find recipients: %v", err)
	}
	var emails []string
	for _, row := range rows {
		emails = append(emails, row.GetString("email"))
	}
	if got := strings.Join(emails, ","); got != "active@example.com,admin@example.com" {
		t.Fatalf("recipients = %s, want only active and verified users", got)
	}
}

func TestCreateEmailSendoutRequiresSMTP(t *testing.T) {
	h, notifier, admin := newSendoutTestHandlers(t)
	h.app.Settings().SMTP.Enabled = false

	e, _ := sendoutEvent(t, h.app, http.MethodPost, "/api/panel/email-sendouts", "", validSendoutRequest(), admin)
	err := h.createEmailSendout(e)
	if err == nil || sendoutAPIStatus(t, err) != http.StatusServiceUnavailable {
		t.Fatalf("createEmailSendout() error = %v, want 503", err)
	}
	if n, _ := h.app.CountRecords(sendouts.SendoutsCollection); n != 0 || notifier.calls != 0 {
		t.Fatalf("stored %d sendouts and notified %d times, want none", n, notifier.calls)
	}
}

func TestCreateEmailSendoutRejectsIneligibleSingleRecipient(t *testing.T) {
	h, _, admin := newSendoutTestHandlers(t)
	disabled := newSendoutTestUser(t, h.app, "disabled@example.com", "disabled", true)
	in := validSendoutRequest()
	in.Audience, in.UserID = "single", disabled.Id

	e, _ := sendoutEvent(t, h.app, http.MethodPost, "/api/panel/email-sendouts", "", in, admin)
	if err := h.createEmailSendout(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
		t.Fatalf("createEmailSendout() error = %v, want 400", err)
	}
}

func TestEmailSendoutComposerEndpoints(t *testing.T) {
	h, _, admin := newSendoutTestHandlers(t)
	newSendoutTestUser(t, h.app, "ops@example.com", "active", true)
	newSendoutTestUser(t, h.app, "off@example.com", "disabled", true)

	e, response := sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts/context", "", nil, admin)
	if err := h.emailSendoutContext(e); err != nil {
		t.Fatalf("emailSendoutContext() error = %v", err)
	}
	var context EmailSendoutContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &context); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if context.EligibleRecipientCount != 2 || !context.SMTPEnabled || context.RatePerMinute != 30 || context.AppName != h.app.Settings().Meta.AppName {
		t.Fatalf("context = %+v", context)
	}

	e, response = sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts/eligible-recipients?search=ops", "", nil, admin)
	if err := h.listEmailSendoutCandidates(e); err != nil {
		t.Fatalf("listEmailSendoutCandidates() error = %v", err)
	}
	var candidates []EmailSendoutRecipientCandidate
	if err := json.Unmarshal(response.Body.Bytes(), &candidates); err != nil || len(candidates) != 1 || candidates[0].Email != "ops@example.com" {
		t.Fatalf("candidates = %+v (%v), want only ops@example.com", candidates, err)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/api/ -run 'TestValidateEmailSendout|TestCreateEmailSendout|TestEmailSendoutComposer' -v`
Expected: build failure: `sendoutQueue`, `EmailSendoutCreateRequest` and the handlers are undefined.

- [ ] **Step 3: Add the DTOs**

Append to `backend/internal/api/dto.go`:

```go
// ── Email Sendouts ────────────────────────────────────────────────────────────

// EmailSendoutContextResponse is what the composer needs before a Sendout is
// written: the template frame's inputs, SMTP availability and audience size.
type EmailSendoutContextResponse struct {
	AppName                string `json:"app_name"`
	FrontendURL            string `json:"frontend_url"`
	SMTPEnabled            bool   `json:"smtp_enabled"`
	EligibleRecipientCount int64  `json:"eligible_recipient_count"`
	RatePerMinute          int    `json:"rate_per_minute"`
}

// EmailSendoutRecipientCandidate is one active and Verified User offered by
// the single-recipient picker.
type EmailSendoutRecipientCandidate struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// EmailSendoutCreateRequest carries the browser-composed email. HTML and Text
// are sent unchanged; Content is the editor JSON kept for the record.
type EmailSendoutCreateRequest struct {
	Subject  string         `json:"subject"`
	Language string         `json:"language"`
	Audience string         `json:"audience"`
	UserID   string         `json:"user_id,omitempty"`
	HTML     string         `json:"html"`
	Text     string         `json:"text"`
	Content  map[string]any `json:"content"`
}

// EmailSendoutCounts groups Sendout Recipients by status; Pending includes the
// row being sent right now.
type EmailSendoutCounts struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Sent      int64 `json:"sent"`
	Failed    int64 `json:"failed"`
	Skipped   int64 `json:"skipped"`
	Cancelled int64 `json:"cancelled"`
}

// EmailSendout is a Sendout without its content. Status is derived from the
// recipients: cancelled, sending, or completed.
type EmailSendout struct {
	ID             string             `json:"id"`
	Subject        string             `json:"subject"`
	Language       string             `json:"language"`
	Audience       string             `json:"audience"`
	Status         string             `json:"status"`
	Counts         EmailSendoutCounts `json:"counts"`
	CreatedByEmail string             `json:"created_by_email"`
	Created        string             `json:"created"`
	CancelledAt    *string            `json:"cancelled_at,omitempty"`
}
```

- [ ] **Step 4: Implement the composer handlers**

Create `backend/internal/api/email_sendouts.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/sendouts"
)

const (
	maxSendoutSubjectRunes = 200
	maxSendoutHTMLBytes    = 1_000_000
	maxSendoutTextBytes    = 256_000
	maxSendoutContentBytes = 1 << 20
	sendoutCandidateLimit  = 20
)

// sendoutNotifier wakes the Sendout worker after rows are queued.
type sendoutNotifier interface {
	Notify()
}

func errSMTPUnavailable() error {
	return apis.NewApiError(http.StatusServiceUnavailable, "email sendouts are unavailable: SMTP is not configured", nil)
}

func (h *Handlers) emailSendoutContext(e *core.RequestEvent) error {
	count, err := sendouts.CountEligible(h.app)
	if err != nil {
		return apis.NewBadRequestError("failed to count eligible recipients", err)
	}
	return ok(e, EmailSendoutContextResponse{
		AppName:                h.app.Settings().Meta.AppName,
		FrontendURL:            h.publicConfig.FrontendURL,
		SMTPEnabled:            h.smtpEnabled(),
		EligibleRecipientCount: count,
		RatePerMinute:          sendouts.RatePerMinute(h.app),
	})
}

func (h *Handlers) listEmailSendoutCandidates(e *core.RequestEvent) error {
	search := strings.TrimSpace(e.Request.URL.Query().Get("search"))
	users, err := sendouts.FindEligible(h.app, search, sendoutCandidateLimit)
	if err != nil {
		return apis.NewBadRequestError("failed to search recipients", err)
	}
	out := make([]EmailSendoutRecipientCandidate, 0, len(users))
	for _, user := range users {
		out = append(out, EmailSendoutRecipientCandidate{ID: user.Id, Email: user.Email()})
	}
	return ok(e, out)
}

func (h *Handlers) createEmailSendout(e *core.RequestEvent) error {
	var in EmailSendoutCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if !h.smtpEnabled() {
		return errSMTPUnavailable()
	}
	draft, err := validateEmailSendout(in)
	if err != nil {
		return err
	}
	users, err := h.sendoutAudience(draft.Audience, in.UserID)
	if err != nil {
		return err
	}
	rec, err := sendouts.Create(h.app, draft, users, e.Auth, time.Now().UTC())
	if err != nil {
		return apis.NewBadRequestError("failed to save email sendout", err)
	}
	h.sendoutQueue.Notify()
	return h.respondEmailSendout(e, rec)
}

// sendoutAudience expands "all" at creation time; Users who become eligible
// later do not receive this Sendout.
func (h *Handlers) sendoutAudience(audience, userID string) ([]*core.Record, error) {
	if audience == sendouts.AudienceAll {
		users, err := sendouts.FindEligible(h.app, "", 0)
		if err != nil {
			return nil, apis.NewBadRequestError("failed to load recipients", err)
		}
		if len(users) == 0 {
			return nil, apis.NewBadRequestError("no users are active and verified", nil)
		}
		return users, nil
	}
	user, err := h.app.FindRecordById("users", userID)
	if err != nil {
		return nil, apis.NewBadRequestError("recipient not found", nil)
	}
	if !sendouts.Eligible(user) {
		return nil, apis.NewBadRequestError("recipient must be active and verified", nil)
	}
	return []*core.Record{user}, nil
}

func validateEmailSendout(in EmailSendoutCreateRequest) (sendouts.Draft, error) {
	subject, err := normalizeSendoutSubject(in.Subject)
	if err != nil {
		return sendouts.Draft{}, err
	}
	if !slices.Contains(sendouts.Languages, in.Language) {
		return sendouts.Draft{}, apis.NewBadRequestError("language must be en or zh-cn", nil)
	}
	if err := validateSendoutAudience(in.Audience, in.UserID); err != nil {
		return sendouts.Draft{}, err
	}
	if err := validateSendoutBody(in); err != nil {
		return sendouts.Draft{}, err
	}
	return sendouts.Draft{Subject: subject, Language: in.Language, Audience: in.Audience, HTML: in.HTML, Text: in.Text, Content: in.Content}, nil
}

func normalizeSendoutSubject(input string) (string, error) {
	subject := strings.TrimSpace(input)
	if subject == "" || utf8.RuneCountInString(subject) > maxSendoutSubjectRunes {
		return "", apis.NewBadRequestError("subject must be between 1 and 200 characters", nil)
	}
	// Control characters include CR and LF, which would let a subject inject mail headers.
	if strings.IndexFunc(subject, unicode.IsControl) >= 0 {
		return "", apis.NewBadRequestError("subject cannot contain control characters", nil)
	}
	return subject, nil
}

func validateSendoutAudience(audience, userID string) error {
	switch {
	case audience == sendouts.AudienceAll && userID == "", audience == sendouts.AudienceSingle && userID != "":
		return nil
	case audience == sendouts.AudienceAll:
		return apis.NewBadRequestError("user_id is only allowed for a single recipient", nil)
	case audience == sendouts.AudienceSingle:
		return apis.NewBadRequestError("user_id is required for a single recipient", nil)
	}
	return apis.NewBadRequestError("audience must be single or all", nil)
}

func validateSendoutBody(in EmailSendoutCreateRequest) error {
	if strings.TrimSpace(in.HTML) == "" || strings.TrimSpace(in.Text) == "" {
		return apis.NewBadRequestError("html and text are required", nil)
	}
	if len(in.HTML) > maxSendoutHTMLBytes || len(in.Text) > maxSendoutTextBytes {
		return apis.NewBadRequestError("email content is too large", nil)
	}
	if in.Content == nil {
		return apis.NewBadRequestError("content is required", nil)
	}
	raw, err := json.Marshal(in.Content)
	if err != nil || len(raw) > maxSendoutContentBytes {
		return apis.NewBadRequestError("content is too large", nil)
	}
	return nil
}

func (h *Handlers) respondEmailSendout(e *core.RequestEvent, rec *core.Record) error {
	counts, err := sendouts.CountRecipients(h.app, rec.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to count recipients", err)
	}
	return ok(e, publicEmailSendout(rec, counts[rec.Id]))
}

func publicEmailSendout(rec *core.Record, counts sendouts.Counts) EmailSendout {
	var cancelledAt *string
	if value := rec.GetString("cancelled_at"); value != "" {
		cancelledAt = &value
	}
	return EmailSendout{
		ID:       rec.Id,
		Subject:  rec.GetString("subject"),
		Language: rec.GetString("language"),
		Audience: rec.GetString("audience"),
		Status:   sendouts.Status(rec, counts),
		Counts: EmailSendoutCounts{
			Total: counts.Total, Pending: counts.Pending, Sent: counts.Sent,
			Failed: counts.Failed, Skipped: counts.Skipped, Cancelled: counts.Cancelled,
		},
		CreatedByEmail: rec.GetString("created_by_email"),
		Created:        rec.GetString("created"),
		CancelledAt:    cancelledAt,
	}
}
```

- [ ] **Step 5: Wire the handlers, routes, and `Register`**

In `backend/internal/api/api.go`:

1. Add `sendoutQueue sendoutNotifier` as the field after `monitoring monitorLifecycle` in `type Handlers struct`.
2. Change the signature to `func Register(se *core.ServeEvent, app core.App, box *cryptobox.Box, ipLookup ipMetadataLookup, passkeys *webauthn.WebAuthn, monitoring monitorLifecycle, sendoutQueue sendoutNotifier, public PanelConfigResponse) *Handlers`, and add `sendoutQueue: sendoutQueue,` to the `&Handlers{...}` literal after `monitoring: monitoring,`.
3. After the `// monitoring` route block, add:

```go
	// email sendouts (transactional mail through PocketBase SMTP; ADR 0008)
	g.GET("/email-sendouts/context", h.emailSendoutContext).Bind(adminOnly)
	g.GET("/email-sendouts/eligible-recipients", h.listEmailSendoutCandidates).Bind(adminOnly)
	g.POST("/email-sendouts", h.createEmailSendout).Bind(adminOnly)
```

In `backend/main.go`:

1. Add `"hysterical-panel/internal/sendouts"` to the imports.
2. After `monitorService := monitoring.New(...)`, add `sendoutService := sendouts.New(app)`.
3. Change the call to `handlers := api.Register(se, app, box, ipLookup, passkeys, monitorService, sendoutService, api.PanelConfigResponse{`.
4. After `monitorService.Start(ctx)`, add `sendoutService.Start(ctx)`.

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/api/ -run 'TestValidateEmailSendout|TestCreateEmailSendout|TestEmailSendoutComposer' -v`
Expected: PASS.

- [ ] **Step 7: Build, vet, full suite, commit**

```bash
go build ./... && go vet ./... && go test ./...
git add backend/internal/api/ backend/main.go
git commit -m "feat(api): add email sendout composer endpoints and start the worker"
```

---

### Task 6: History, recipients, cancel, and resend endpoints

**Files:**
- Modify: `backend/internal/api/email_sendouts.go`
- Modify: `backend/internal/api/dto.go`
- Modify: `backend/internal/api/api.go` (routes)
- Test: `backend/internal/api/email_sendouts_test.go`

**Interfaces:**
- Consumes: Task 5 helpers; `sendouts.Cancel`, `sendouts.Requeue`, `sendouts.ErrCancelled`, `sendouts.ErrNothingToCancel`, `sendouts.RecipientStatuses`.
- Produces: DTOs `EmailSendoutDetail`, `EmailSendoutRecipient`, `EmailSendoutResendRequest`, `EmailSendoutResendResponse`. Routes `GET /email-sendouts`, `GET /email-sendouts/{id}`, `GET /email-sendouts/{id}/recipients`, `POST /email-sendouts/{id}/cancel`, `POST /email-sendouts/{id}/resend`.

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/api/email_sendouts_test.go`, and add `"time"` to its imports:

```go
func TestEmailSendoutHistoryCancelAndResend(t *testing.T) {
	h, notifier, admin := newSendoutTestHandlers(t)
	user := newSendoutTestUser(t, h.app, "user@example.com", "active", true)
	draft, err := validateEmailSendout(validSendoutRequest())
	if err != nil {
		t.Fatalf("validateEmailSendout() error = %v", err)
	}
	rec, err := sendouts.Create(h.app, draft, []*core.Record{admin, user}, admin, time.Now().UTC())
	if err != nil {
		t.Fatalf("sendouts.Create() error = %v", err)
	}
	failed, err := h.app.FindFirstRecordByData(sendouts.RecipientsCollection, "email", "user@example.com")
	if err != nil {
		t.Fatalf("find recipient: %v", err)
	}
	failed.Set("status", sendouts.StatusFailed)
	failed.Set("reason", sendouts.ReasonDeliveryFailed)
	if err := h.app.Save(failed); err != nil {
		t.Fatalf("fail recipient: %v", err)
	}
	base := "/api/panel/email-sendouts/" + rec.Id

	e, response := sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts", "", nil, admin)
	if err := h.listEmailSendouts(e); err != nil {
		t.Fatalf("listEmailSendouts() error = %v", err)
	}
	var list []EmailSendout
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Counts.Failed != 1 || list[0].Counts.Pending != 1 {
		t.Fatalf("list = %+v (%v)", list, err)
	}

	e, response = sendoutEvent(t, h.app, http.MethodGet, base, rec.Id, nil, admin)
	if err := h.getEmailSendout(e); err != nil {
		t.Fatalf("getEmailSendout() error = %v", err)
	}
	var detail EmailSendoutDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil || detail.HTML != draft.HTML || detail.Text != draft.Text {
		t.Fatalf("detail = %+v (%v), want stored content", detail, err)
	}

	e, response = sendoutEvent(t, h.app, http.MethodGet, base+"/recipients?status=failed", rec.Id, nil, admin)
	if err := h.listEmailSendoutRecipients(e); err != nil {
		t.Fatalf("listEmailSendoutRecipients() error = %v", err)
	}
	var rows []EmailSendoutRecipient
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].Reason == nil || *rows[0].Reason != sendouts.ReasonDeliveryFailed {
		t.Fatalf("failed recipients = %+v (%v)", rows, err)
	}
	e, _ = sendoutEvent(t, h.app, http.MethodGet, base+"/recipients?status=bogus", rec.Id, nil, admin)
	if err := h.listEmailSendoutRecipients(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
		t.Fatalf("unknown status filter error = %v, want 400", err)
	}

	e, response = sendoutEvent(t, h.app, http.MethodPost, base+"/resend", rec.Id, EmailSendoutResendRequest{}, admin)
	if err := h.resendEmailSendout(e); err != nil {
		t.Fatalf("resendEmailSendout() error = %v", err)
	}
	var resent EmailSendoutResendResponse
	if err := json.Unmarshal(response.Body.Bytes(), &resent); err != nil || resent.Requeued != 1 || notifier.calls != 1 {
		t.Fatalf("resend = %+v (%v), notified %d; want 1 requeued and 1 wake-up", resent, err, notifier.calls)
	}

	e, response = sendoutEvent(t, h.app, http.MethodPost, base+"/cancel", rec.Id, nil, admin)
	if err := h.cancelEmailSendout(e); err != nil {
		t.Fatalf("cancelEmailSendout() error = %v", err)
	}
	var cancelled EmailSendout
	if err := json.Unmarshal(response.Body.Bytes(), &cancelled); err != nil || cancelled.Status != "cancelled" || cancelled.Counts.Cancelled != 2 || cancelled.CancelledAt == nil {
		t.Fatalf("cancelled = %+v (%v)", cancelled, err)
	}
	for _, call := range []func(*core.RequestEvent) error{h.cancelEmailSendout, h.resendEmailSendout} {
		e, _ = sendoutEvent(t, h.app, http.MethodPost, base, rec.Id, nil, admin)
		if err := call(e); err == nil || sendoutAPIStatus(t, err) != http.StatusBadRequest {
			t.Fatalf("action on cancelled sendout error = %v, want 400", err)
		}
	}

	e, _ = sendoutEvent(t, h.app, http.MethodGet, "/api/panel/email-sendouts/missing", "missing", nil, admin)
	if err := h.getEmailSendout(e); err == nil || sendoutAPIStatus(t, err) != http.StatusNotFound {
		t.Fatalf("missing sendout error = %v, want 404", err)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./internal/api/ -run TestEmailSendoutHistoryCancelAndResend -v`
Expected: build failure because the handlers and DTOs are undefined.

- [ ] **Step 3: Add the DTOs**

Append to `backend/internal/api/dto.go`:

```go
// EmailSendoutDetail adds the stored content to a Sendout.
type EmailSendoutDetail struct {
	EmailSendout
	HTML string `json:"html"`
	Text string `json:"text"`
}

// EmailSendoutRecipient is one Sendout Recipient. Email is the address of the
// latest delivery attempt (or of creation, if none yet). UserID is empty once
// the User is deleted.
type EmailSendoutRecipient struct {
	ID            string  `json:"id"`
	UserID        string  `json:"user_id"`
	Email         string  `json:"email"`
	Status        string  `json:"status"`
	Reason        *string `json:"reason,omitempty"`
	Attempts      int     `json:"attempts"`
	QueuedAt      string  `json:"queued_at"`
	LastAttemptAt string  `json:"last_attempt_at"`
	SentAt        string  `json:"sent_at"`
}

// EmailSendoutResendRequest selects failed recipients to requeue; an empty
// list means every failed recipient of the Sendout.
type EmailSendoutResendRequest struct {
	RecipientIDs []string `json:"recipient_ids,omitempty"`
}

type EmailSendoutResendResponse struct {
	Requeued int64 `json:"requeued"`
}
```

- [ ] **Step 4: Implement the handlers**

Append to `backend/internal/api/email_sendouts.go`, and add `"errors"` and `"github.com/pocketbase/dbx"` to its imports:

```go
func (h *Handlers) findEmailSendout(id string) (*core.Record, error) {
	rec, err := h.app.FindRecordById(sendouts.SendoutsCollection, id)
	if err != nil {
		return nil, apis.NewNotFoundError("email sendout not found", err)
	}
	return rec, nil
}

func (h *Handlers) listEmailSendouts(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter(sendouts.SendoutsCollection, "", "-created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list email sendouts", err)
	}
	counts, err := sendouts.CountRecipients(h.app, "")
	if err != nil {
		return apis.NewBadRequestError("failed to count recipients", err)
	}
	out := make([]EmailSendout, 0, len(records))
	for _, rec := range records {
		out = append(out, publicEmailSendout(rec, counts[rec.Id]))
	}
	return ok(e, out)
}

func (h *Handlers) getEmailSendout(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	counts, err := sendouts.CountRecipients(h.app, rec.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to count recipients", err)
	}
	return ok(e, EmailSendoutDetail{
		EmailSendout: publicEmailSendout(rec, counts[rec.Id]),
		HTML:         rec.GetString("html"),
		Text:         rec.GetString("text"),
	})
}

func (h *Handlers) listEmailSendoutRecipients(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	filter, params := "sendout = {:id}", dbx.Params{"id": rec.Id}
	if status := e.Request.URL.Query().Get("status"); status != "" {
		if !slices.Contains(sendouts.RecipientStatuses, status) {
			return apis.NewBadRequestError("unknown recipient status", nil)
		}
		filter += " && status = {:status}"
		params["status"] = status
	}
	rows, err := h.app.FindRecordsByFilter(sendouts.RecipientsCollection, filter, "email", 0, 0, params)
	if err != nil {
		return apis.NewBadRequestError("failed to list recipients", err)
	}
	out := make([]EmailSendoutRecipient, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicSendoutRecipient(row))
	}
	return ok(e, out)
}

func publicSendoutRecipient(row *core.Record) EmailSendoutRecipient {
	var reason *string
	if value := row.GetString("reason"); value != "" {
		reason = &value
	}
	return EmailSendoutRecipient{
		ID:            row.Id,
		UserID:        row.GetString("user"),
		Email:         row.GetString("email"),
		Status:        row.GetString("status"),
		Reason:        reason,
		Attempts:      row.GetInt("attempts"),
		QueuedAt:      row.GetString("queued_at"),
		LastAttemptAt: row.GetString("last_attempt_at"),
		SentAt:        row.GetString("sent_at"),
	}
}

func (h *Handlers) cancelEmailSendout(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := sendouts.Cancel(h.app, rec, time.Now().UTC()); err != nil {
		return sendoutStateError(err, "cancel")
	}
	return h.respondEmailSendout(e, rec)
}

func (h *Handlers) resendEmailSendout(e *core.RequestEvent) error {
	rec, err := h.findEmailSendout(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in EmailSendoutResendRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if !h.smtpEnabled() {
		return errSMTPUnavailable()
	}
	requeued, err := sendouts.Requeue(h.app, rec, in.RecipientIDs, time.Now().UTC())
	if err != nil {
		return sendoutStateError(err, "resend")
	}
	if requeued > 0 {
		h.sendoutQueue.Notify()
	}
	return ok(e, EmailSendoutResendResponse{Requeued: requeued})
}

func sendoutStateError(err error, action string) error {
	switch {
	case errors.Is(err, sendouts.ErrCancelled):
		return apis.NewBadRequestError("email sendout is cancelled", nil)
	case errors.Is(err, sendouts.ErrNothingToCancel):
		return apis.NewBadRequestError("email sendout has no pending recipients", nil)
	}
	return apis.NewBadRequestError("failed to "+action+" email sendout", err)
}
```

- [ ] **Step 5: Register the routes**

In `backend/internal/api/api.go`, add these lines below the three email sendout routes from Task 5:

```go
	g.GET("/email-sendouts", h.listEmailSendouts).Bind(adminOnly)
	g.GET("/email-sendouts/{id}", h.getEmailSendout).Bind(adminOnly)
	g.GET("/email-sendouts/{id}/recipients", h.listEmailSendoutRecipients).Bind(adminOnly)
	g.POST("/email-sendouts/{id}/cancel", h.cancelEmailSendout).Bind(adminOnly)
	g.POST("/email-sendouts/{id}/resend", h.resendEmailSendout).Bind(adminOnly)
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/api/ -run 'EmailSendout' -v`
Expected: PASS.

- [ ] **Step 7: Build, vet, full suite, commit**

```bash
go build ./... && go vet ./... && go test ./...
git add backend/internal/api/
git commit -m "feat(api): add email sendout history, cancel and resend endpoints"
```

---

### Task 7: OpenAPI contract and documentation

**Files:**
- Modify: `backend/internal/api/openapi.go`
- Test: `backend/internal/api/email_sendouts_test.go`
- Modify: `AGENTS.md`, `backend/README.md`

**Interfaces:**
- Consumes: every DTO from Tasks 4–6.
- Produces: OpenAPI operation IDs `listEmailSendouts`, `createEmailSendout`, `getEmailSendoutContext`, `listEmailSendoutCandidates`, `getEmailSendout`, `listEmailSendoutRecipients`, `cancelEmailSendout`, `resendEmailSendout`. The frontend plan uses them through `schema.d.ts`.

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/api/email_sendouts_test.go`:

```go
func TestOpenAPIDescribesEmailSendouts(t *testing.T) {
	spec, err := BuildOpenAPISpec()
	if err != nil {
		t.Fatalf("BuildOpenAPISpec: %v", err)
	}
	for _, path := range []string{
		"/api/panel/email-sendouts",
		"/api/panel/email-sendouts/context",
		"/api/panel/email-sendouts/eligible-recipients",
		"/api/panel/email-sendouts/{id}",
		"/api/panel/email-sendouts/{id}/recipients",
		"/api/panel/email-sendouts/{id}/cancel",
		"/api/panel/email-sendouts/{id}/resend",
	} {
		if spec.Paths.Find(path) == nil {
			t.Errorf("OpenAPI is missing %s", path)
		}
	}
	if spec.Paths.Find("/api/panel/email-sendouts").Post.Responses.Value("503") == nil {
		t.Error("createEmailSendout must document 503")
	}
	status := spec.Components.Schemas["EmailSendoutRecipient"].Value.Properties["status"].Value.Enum
	if len(status) != len(sendouts.RecipientStatuses) {
		t.Errorf("recipient status enum = %v, want %v", status, sendouts.RecipientStatuses)
	}
	rate := spec.Components.Schemas["SettingsResponse"].Value.Properties["email_sendout_rate_per_minute"]
	if rate == nil {
		t.Error("SettingsResponse is missing email_sendout_rate_per_minute")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./internal/api/ -run TestOpenAPIDescribesEmailSendouts -v`
Expected: FAIL with `OpenAPI is missing /api/panel/email-sendouts` (plus the other paths). The `SettingsResponse` check already passes because of Task 4.

- [ ] **Step 3: Register the schemas**

In `backend/internal/api/openapi.go`, add these entries to `schemaDefs`, after `"ManagementAPITokenResponse": ManagementAPITokenResponse{},`:

```go
		"EmailSendoutContextResponse":    EmailSendoutContextResponse{},
		"EmailSendoutRecipientCandidate": EmailSendoutRecipientCandidate{},
		"EmailSendoutCreateRequest":      EmailSendoutCreateRequest{},
		"EmailSendoutCounts":             EmailSendoutCounts{},
		"EmailSendout":                   EmailSendout{},
		"EmailSendoutDetail":             EmailSendoutDetail{},
		"EmailSendoutRecipient":          EmailSendoutRecipient{},
		"EmailSendoutResendRequest":      EmailSendoutResendRequest{},
		"EmailSendoutResendResponse":     EmailSendoutResendResponse{},
```

Add these entries to the `for name, fields := range map[string][]string{ ... }` block that appends `Required`:

```go
		"EmailSendoutContextResponse":    {"app_name", "frontend_url", "smtp_enabled", "eligible_recipient_count", "rate_per_minute"},
		"EmailSendoutRecipientCandidate": {"id", "email"},
		"EmailSendoutCreateRequest":      {"subject", "language", "audience", "html", "text", "content"},
		"EmailSendoutCounts":             {"total", "pending", "sent", "failed", "skipped", "cancelled"},
		"EmailSendout":                   {"id", "subject", "language", "audience", "status", "counts", "created_by_email", "created"},
		"EmailSendoutDetail":             {"id", "subject", "language", "audience", "status", "counts", "created_by_email", "created", "html", "text"},
		"EmailSendoutRecipient":          {"id", "user_id", "email", "status", "attempts", "queued_at", "last_attempt_at", "sent_at"},
		"EmailSendoutResendResponse":     {"requeued"},
		"SettingsResponse":               {"email_sendout_rate_per_minute"},
```

If `SettingsResponse` already has an entry in that map, append `"email_sendout_rate_per_minute"` to it instead of adding a duplicate key, because Go rejects duplicate keys in a map literal.

After the `if s, ok := schemas["Alert"]; ...` enum block, add:

```go
	for _, name := range []string{"EmailSendout", "EmailSendoutDetail", "EmailSendoutCreateRequest"} {
		if s, ok := schemas[name]; ok && s.Value != nil {
			setEnum(s.Value.Properties, "language", []any{"en", "zh-cn"})
			setEnum(s.Value.Properties, "audience", []any{"single", "all"})
		}
	}
	for _, name := range []string{"EmailSendout", "EmailSendoutDetail"} {
		if s, ok := schemas[name]; ok && s.Value != nil {
			setEnum(s.Value.Properties, "status", []any{"sending", "completed", "cancelled"})
			s.Value.Properties["counts"] = &openapi3.SchemaRef{Ref: "#/components/schemas/EmailSendoutCounts"}
		}
	}
	if s, ok := schemas["EmailSendoutRecipient"]; ok && s.Value != nil {
		setEnum(s.Value.Properties, "status", []any{"pending", "sending", "sent", "failed", "skipped", "cancelled"})
		setEnum(s.Value.Properties, "reason", []any{"delivery_failed", "smtp_disabled", "interrupted", "user_ineligible", "user_deleted"})
	}
```

- [ ] **Step 4: Add the paths**

In `backend/internal/api/openapi.go`, insert the following immediately before the final `return t, nil` of `BuildOpenAPISpec`:

```go
	// ── /email-sendouts ────────────────────────────────────────────────────
	smtpUnavailable := errRef(503, "SMTP is not configured")
	sendoutOp := func(id, summary string, response *openapi3.SchemaRef, body string) *openapi3.Operation {
		op := &openapi3.Operation{
			OperationID: id, Summary: summary, Tags: []string{"email sendouts"},
			Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: &openapi3.Response{
				Description: ptr(summary), Content: content(response),
			}})),
		}
		if body != "" {
			op.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithRequired(true).WithJSONSchemaRef(ref(body))}
			op.Responses.Set("400", badRequest)
		}
		withAuth(op)
		return op
	}
	sendoutQueryParam := func(name, desc string) *openapi3.ParameterRef {
		return &openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name: name, In: "query", Description: desc,
			Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
		}}
	}
	sendoutIDParams := openapi3.Parameters{idParam("Email Sendout ID")}

	createSendout := sendoutOp("createEmailSendout", "Queue an Email Sendout", ref("EmailSendout"), "EmailSendoutCreateRequest")
	createSendout.Responses.Set("503", smtpUnavailable)
	t.Paths.Set("/api/panel/email-sendouts", &openapi3.PathItem{
		Get:  sendoutOp("listEmailSendouts", "List Email Sendouts, newest first", arrayRef("EmailSendout"), ""),
		Post: createSendout,
	})
	t.Paths.Set("/api/panel/email-sendouts/context", &openapi3.PathItem{
		Get: sendoutOp("getEmailSendoutContext", "Get the template inputs, SMTP availability and audience size", ref("EmailSendoutContextResponse"), ""),
	})
	candidates := sendoutOp("listEmailSendoutCandidates", "Search active and verified Users by email", arrayRef("EmailSendoutRecipientCandidate"), "")
	candidates.Parameters = openapi3.Parameters{sendoutQueryParam("search", "Email substring")}
	t.Paths.Set("/api/panel/email-sendouts/eligible-recipients", &openapi3.PathItem{Get: candidates})

	getSendout := sendoutOp("getEmailSendout", "Get an Email Sendout with its content", ref("EmailSendoutDetail"), "")
	getSendout.Responses.Set("404", notFound)
	t.Paths.Set("/api/panel/email-sendouts/{id}", &openapi3.PathItem{Parameters: sendoutIDParams, Get: getSendout})

	recipients := sendoutOp("listEmailSendoutRecipients", "List a Sendout's recipients by email", arrayRef("EmailSendoutRecipient"), "")
	recipients.Parameters = openapi3.Parameters{sendoutQueryParam("status", "Only recipients in this status")}
	recipients.Responses.Set("400", badRequest)
	recipients.Responses.Set("404", notFound)
	t.Paths.Set("/api/panel/email-sendouts/{id}/recipients", &openapi3.PathItem{Parameters: sendoutIDParams, Get: recipients})

	cancelSendout := sendoutOp("cancelEmailSendout", "Cancel a Sendout's pending recipients", ref("EmailSendout"), "")
	cancelSendout.Responses.Set("400", badRequest)
	cancelSendout.Responses.Set("404", notFound)
	t.Paths.Set("/api/panel/email-sendouts/{id}/cancel", &openapi3.PathItem{Parameters: sendoutIDParams, Post: cancelSendout})

	resendSendout := sendoutOp("resendEmailSendout", "Requeue failed recipients at the tail of the queue", ref("EmailSendoutResendResponse"), "EmailSendoutResendRequest")
	resendSendout.Responses.Set("404", notFound)
	resendSendout.Responses.Set("503", smtpUnavailable)
	t.Paths.Set("/api/panel/email-sendouts/{id}/resend", &openapi3.PathItem{Parameters: sendoutIDParams, Post: resendSendout})
```

- [ ] **Step 5: Run the tests and regenerate the schema**

```bash
go test ./internal/api/ -run TestOpenAPIDescribesEmailSendouts -v
make openapi
```
Expected: the test passes, and `make openapi` prints `wrote openapi.json`. `backend/openapi.json` is a generated file, so check `git status` for whether it is tracked. Commit it only if it is.

- [ ] **Step 6: Update `backend/README.md`**

1. In the opening paragraph, add this sentence after the sentence about Notification Channels: `管理员可经 PocketBase SMTP 发送 Email Sendout（事务邮件，不可退订），由持久化队列按速率逐封投递、只发一次、失败可手动重发。`
2. In the `## 接口` table, add these rows after the `/ignored-connection-ips/{id}` row:

```markdown
| GET | `/email-sendouts/context` | 写信页所需：`app_name`、`frontend_url`、`smtp_enabled`、可用 User 数、发送速率 |
| GET | `/email-sendouts/eligible-recipients?search=` | 按邮箱子串查可用 User（`status=active && verified=true`），最多 20 条 |
| GET | `/email-sendouts` | Email Sendout 列表（新到旧，含各状态计数与推导出的 `status`） |
| POST | `/email-sendouts` | 创建 Sendout 并入队（SMTP 未启用 503） |
| GET | `/email-sendouts/{id}` | Sendout 详情，含原样存储的 `html` / `text` |
| GET | `/email-sendouts/{id}/recipients?status=` | Sendout Recipient 列表 |
| POST | `/email-sendouts/{id}/cancel` | 取消尚未发出的收件人 |
| POST | `/email-sendouts/{id}/resend` | 把失败的收件人重新排到队尾（body `{ "recipient_ids": [] }`，空 = 全部失败项；SMTP 未启用 503） |
```

3. Add this section after `### 通知 Channel`:

```markdown
### Email Sendout

邮件 HTML 与纯文本由前端 `@react-email/editor` 生成（含固定模板外框），后端只校验大小并原样存储、原样发送，没有后端模板（ADR 0008）。创建时在同一事务里为每个可用 User 写一行 `email_sendout_recipients`；「全部」按创建瞬间展开。进程内单 worker 按 `queued_at` FIFO 逐封发送，间隔 `60s / app_settings.email_sendout_rate_per_minute`（1–600，默认 30）。每行只尝试一次：发送前重查资格（不可用 → `skipped`），用 User 当时的邮箱；SMTP 失败或未启用 → `failed`。进程重启时遗留的 `sending` 行转 `failed`（`interrupted`），不会自动重发。取消只影响 `pending` 行；重发只影响 `failed` 行。历史永久保留。
```

- [ ] **Step 7: Update `AGENTS.md`**

1. In the backend-responsibilities list, add this bullet after the Management API bullet: `- 可选的 Email Sendout：admin 在面板里写事务邮件，经 PocketBase SMTP 发给一个或全部可用 User；持久化队列、只发一次、失败手动重发，见 ADR-0008。`
2. In the directory tree:
   - change `migrations/             代码式迁移，启动自动应用（1730000001..26）` to `..27`;
   - after the `onlinedevices/` line, add `│       ├── sendouts/           Email Sendout 队列（创建/取消/重发/计数）+ 单 worker 限速投递`;
   - after the `notification_channels.go` line, add `│           ├── email_sendouts.go Email Sendout 接口（校验、收件人展开、历史、取消/重发）`.
3. In the data model section, append after the `passkey_sessions` paragraph:

```markdown
`email_sendouts` / `email_sendout_recipients`：Email Sendout 与每个收件人一行。Sendout 保存浏览器生成、原样发送的 `html` / `text` 与 editor JSON `content`、`language`、`audience`（`single`/`all`）、`created_by` + `created_by_email` 快照、`cancelled_at`；状态不落库，由 Recipient 计数推导。Recipient 的 `status` 为 `pending`/`sending`/`sent`/`failed`/`skipped`/`cancelled`，`reason` 为安全错误码，`email` 记录最近一次投递的地址；User 删除后 `user` 置空而行保留。历史永久保留。
```

4. In the `app_settings` paragraph, add `email_sendout_rate_per_minute`（1–600，默认 30）to the field list.
5. In the panel-interface bullet list, add this bullet after the Monitoring bullet: `- Email Sendout 接口 \`GET /email-sendouts/context\`、\`GET /email-sendouts/eligible-recipients\`、\`GET|POST /email-sendouts\`、\`GET /email-sendouts/{id}\`、\`GET /email-sendouts/{id}/recipients\`、\`POST /email-sendouts/{id}/cancel|resend\` 均仅 admin、进 OpenAPI。创建与重发在 SMTP 未启用时 503；worker 语义见 backend/README.md 与 ADR-0008。`

- [ ] **Step 8: Final verification and commit**

```bash
go build ./... && go vet ./... && go test ./...
git add backend/internal/api/ backend/README.md AGENTS.md
git commit -m "feat(api): document email sendouts in OpenAPI and project docs"
```

Then check the running server:

```bash
PANEL_MASTER_KEY=dev go run . serve --dir "$(mktemp -d)" &
sleep 3
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8090/api/panel/email-sendouts
kill %1
```
Expected: `401` (the route exists and requires login).
