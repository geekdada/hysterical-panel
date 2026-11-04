# Automatic Emails Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send Low Allowance, Allowance Exhausted and Allowance Restored emails to Users automatically, from administrator-edited bilingual templates, through the existing Email Sendout outbox.

**Architecture:** A new `internal/automaticemails` package scans usable metered Users every 60s, compares each User's current low / exhausted condition with a persisted per-User state, and for each transition renders the Kind's Email Template (placeholder substitution only) into a single-recipient Email Sendout marked `source=automatic`. The existing Sendout worker delivers it. Admin endpoints under `/api/panel/automatic-emails` manage switches, the Low threshold and templates. The frontend reuses the Sendout editor with a bilingual frame.

**Tech Stack:** Go + PocketBase v0.40 (framework), TanStack Start + HeroUI v3 + TanStack Query/Form, `@react-email/editor`, Paraglide.

**Spec:** `docs/adr/0014-automatic-emails-outside-monitoring.md` and the Automatic Email entries in `CONTEXT.md`. Read ADR-0008 for the Sendout outbox it reuses.

## Global Constraints

- Kind identifiers: `low_allowance`, `allowance_exhausted`, `allowance_restored`.
- Scope: Users with `status = 'active' && verified = true && subscription_required = true` that have a current User Subscription (`subscriptions.Current` non-nil).
- Scan interval: 60 seconds.
- Low threshold: integer percent 1–99, default 10. Low means `remaining*100 < allowance*percent` (existing `IsLowAllowance` semantics); exhausted means `remaining <= 0`. Allowance includes top-ups (`State.Allowance`).
- Placeholders: `{{email}}`, `{{remaining}}`, `{{allowance}}`, `{{used}}`, `{{remaining_percent}}`, `{{window_starts_at}}`, `{{window_ends_at}}`, `{{subscription_ends_at}}`, `{{panel_url}}`. Any other `{{...}}` is a 400.
- Byte values use binary units (`B`, `KiB` … `PiB`, one decimal above bytes), negatives display as `0 B`. Times display as `2006-01-02 15:04 UTC`. Percent rounds down and clamps at 0.
- HTML substitution escapes values with `html.EscapeString`; subject and text get raw values.
- A Kind cannot be enabled until its template is saved. SMTP state never blocks saving or enabling.
- All datetimes stored in UTC. Bytes are int64.
- Every new `/api/panel/*` endpoint is admin-only and in OpenAPI; run `make openapi` + `pnpm api:sync` after DTO changes.
- Every new UI string goes into both `messages/en.json` and `messages/zh-cn.json`. HeroUI Alert titles/messages end without `.` or `。`.
- Default templates show Chinese first, then English, separated by a divider. The automatic frame footer shows both languages.
- `go build ./... && go vet ./...` clean after every backend task; `pnpm typecheck && pnpm i18n:check` after every frontend task.

## Review Focus

- A placeholder split by editor formatting (`{{<strong>remaining</strong>}}`) would silently go out literally. Saving must reject a template whose HTML placeholders differ from its text placeholders (Task 3 test `TestValidateTemplateRejectsBrokenHTMLPlaceholder`).
- A placeholder inside a link `href` is percent-encoded to `%7B%7B…%7D%7D` and never substituted. Saving must reject HTML containing `%7B%7B` (case-insensitive) (Task 3 test `TestValidateTemplateRejectsEncodedPlaceholder`).
- A User's email containing `<` or `&` (or any value) must not inject HTML (Task 3 test `TestRenderEscapesHTMLValues`).
- A scan crash between creating the Sendout and saving state must not send twice or lose the transition: state upsert and Sendout creation share one transaction (Task 4 test `TestScanRollsBackStateWhenSendoutFails`).
- A disabled Kind still advances state, so turning it on later does not mail every User already past the threshold (Task 4 test `TestScanTracksStateWhileKindDisabled`).

---

### Task 1: Move shared helpers out of `monitoring`

Automatic Emails must not import `monitoring` (ADR-0014 keeps them independent).

**Files:**
- Create: `backend/internal/bytesize/bytesize.go`, `backend/internal/bytesize/bytesize_test.go`
- Modify: `backend/internal/subscriptions/subscriptions.go` (add `IsLowAllowance`), `backend/internal/monitoring/evaluator.go:67-78` (remove it), `backend/internal/monitoring/user_monitors.go:129`, `backend/internal/monitoring/message.go:213-215,283-297`, `backend/internal/monitoring/evaluator_test.go:85-86` (move the table test to `subscriptions_test.go`)

**Interfaces:**
- Produces: `bytesize.Format(bytes int64) string` (body of `formatNotificationBytes`, unchanged); `subscriptions.IsLowAllowance(remaining, allowance, percent int64) bool` (unchanged semantics).

- [ ] **Step 1:** Write `TestFormat` in `bytesize_test.go`: `Format(-5) == "0 B"`, `Format(1023) == "1023 B"`, `Format(1536) == "1.5 KiB"`, `Format(10*1<<30) == "10.0 GiB"`. Move the `IsLowAllowance` table test into `subscriptions_test.go` as `TestIsLowAllowance`.
- [ ] **Step 2:** Run `go test ./internal/bytesize ./internal/subscriptions`; expect compile failure.
- [ ] **Step 3:** Move the two functions, update monitoring call sites, delete `formatNotificationBytes`.
- [ ] **Step 4:** Run `go test ./internal/... && go build ./... && go vet ./...`; expect PASS.
- [ ] **Step 5:** Commit `refactor: share byte formatting and low-allowance check outside monitoring`.

---

### Task 2: Collections and Sendout fields

**Files:**
- Create: `backend/migrations/1730000039_create_automatic_emails.go`, `backend/migrations/automatic_emails_test.go`
- Modify: `backend/internal/sendouts/sendouts.go` (constants), `backend/internal/sendouts/queue.go` (`Draft`, `saveSendout`)

**Interfaces:**
- Produces collections (all five API rules nil):
  - `automatic_email_templates`: `kind` (select, the three Kinds, required, unique index), `enabled` (bool), `threshold_percent` (number, int; 10 on the `low_allowance` row, 0 elsewhere), `subject` (text, max 200), `html` (text, max 1_000_000), `text` (text, max 256_000), `content` (json, 1 MiB), `created`/`updated` autodate. Migration seeds the three rows disabled with empty templates.
  - `automatic_email_states`: `user` (relation→users, required, cascade delete, unique index), `low` (bool), `exhausted` (bool), `updated` autodate.
  - `email_sendouts`: add `source` (select `manual`/`automatic`; existing rows set to `manual`), `automatic_kind` (select, the three Kinds, optional); `language` gains value `bilingual`.
- Produces in `sendouts`: `SourceManual = "manual"`, `SourceAutomatic = "automatic"`, `LanguageBilingual = "bilingual"`; `Draft` gains `Source string` and `AutomaticKind string`. `saveSendout` writes both and defaults an empty `Source` to `SourceManual`. `Languages` stays `en`/`zh-cn` (manual Sendouts cannot pick bilingual).

- [ ] **Step 1:** Write `TestAutomaticEmailsMigration`: after migrations, three template rows exist with `enabled=false`, `low_allowance` has `threshold_percent=10`; a pre-existing Sendout row reads `source == "manual"`; saving a second state row for the same user fails. Add `TestCreateStoresSource` in `internal/sendouts/queue_test.go`: `Create` with `Draft{Source: SourceAutomatic, AutomaticKind: "low_allowance", Language: LanguageBilingual, ...}` persists all three; an empty Source persists `manual`.
- [ ] **Step 2:** Run `go test ./migrations ./internal/sendouts`; expect FAIL.
- [ ] **Step 3:** Write the migration (with a down migration that removes both collections and the added fields/value) and the `sendouts` changes.
- [ ] **Step 4:** Run the tests, `go build ./... && go vet ./...`; expect PASS.
- [ ] **Step 5:** Commit `feat(automatic-emails): add template, state and sendout source fields`.

---

### Task 3: Kinds, transition decision, placeholders

Pure functions, no database.

**Files:**
- Create: `backend/internal/automaticemails/kinds.go`, `decide.go`, `template.go`, and `decide_test.go`, `template_test.go`

**Interfaces:**
- Consumes: `bytesize.Format`, `subscriptions.State`, `subscriptions.WindowEnd`.
- Produces:
  - `type Kind string`; constants `KindLowAllowance`, `KindAllowanceExhausted`, `KindAllowanceRestored`; `var Kinds = []Kind{...}` in that order; `func ParseKind(s string) (Kind, bool)`.
  - `type Condition struct{ Low, Exhausted bool }`.
  - `func Decide(prev, cur Condition) []Kind`:
    - `cur.Exhausted && !prev.Exhausted` → `KindAllowanceExhausted`
    - else `cur.Low && !prev.Low` → `KindLowAllowance`
    - `!cur.Exhausted && prev.Exhausted` → `KindAllowanceRestored`
  - `type Template struct{ Subject, HTML, Text string }`.
  - `var ErrUnknownPlaceholder, ErrBrokenPlaceholder, ErrEncodedPlaceholder error`.
  - `func ValidateTemplate(t Template) error`: unknown name in any part → `ErrUnknownPlaceholder`; HTML containing `%7b%7b` case-insensitively → `ErrEncodedPlaceholder`; set of names in HTML ≠ set in Text → `ErrBrokenPlaceholder`.
  - `func Variables(email string, state *subscriptions.State, panelURL string) map[string]string` (all nine keys; `window_starts_at` is the grant start for window 0, otherwise `WindowEnd(..., index-1, ...)`).
  - `func Render(t Template, vars map[string]string) Template`.
  - Placeholder regex: `\{\{([a-z_]+)\}\}`.

- [ ] **Step 1:** Write tests:
  - `TestDecide` table: `{false,false}→{true,false}` = `[low]`; `{false,false}→{true,true}` = `[exhausted]`; `{true,false}→{true,true}` = `[exhausted]`; `{true,true}→{true,false}` = `[restored]`; `{true,true}→{false,false}` = `[restored]`; `{true,false}→{false,false}` = `[]`; `{true,false}→{true,false}` = `[]`.
  - `TestValidateTemplateRejectsUnknownPlaceholder` (`{{name}}` in subject).
  - `TestValidateTemplateRejectsBrokenHTMLPlaceholder` (HTML `{{<strong>remaining</strong>}}`, text `{{remaining}}`).
  - `TestValidateTemplateRejectsEncodedPlaceholder` (HTML `href="%7B%7Bpanel_url%7D%7D"`).
  - `TestRenderEscapesHTMLValues`: an `email` value containing `<`, `>` and `&` appears HTML-escaped (`&lt;`, `&gt;`, `&amp;`) in HTML and unchanged in Text and Subject.
  - `TestVariables`: allowance 100 GiB, remaining −1 → `remaining == "0 B"`, `remaining_percent == "0%"`; window end formatted `2026-11-04 08:00 UTC`.
- [ ] **Step 2:** Run `go test ./internal/automaticemails`; expect FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run; expect PASS.
- [ ] **Step 5:** Commit `feat(automatic-emails): decide transitions and render templates`.

---

### Task 4: Scanner service and wiring

**Files:**
- Create: `backend/internal/automaticemails/service.go`, `service_test.go` (helpers copied from `internal/monitoring/user_monitors_test.go:205-270` for users, types, grants and window usage)
- Modify: `backend/main.go:114-132`

**Interfaces:**
- Consumes: Task 2 collections and `sendouts.Create`/`Draft`; Task 3 functions; `subscriptions.Current`, `subscriptions.IsLowAllowance`.
- Produces:
  - `type Notifier interface{ Notify() }` (satisfied by `*sendouts.Service`).
  - `func New(app core.App, notifier Notifier, frontendURL string) *Service`.
  - `func (s *Service) Start(ctx context.Context)`: ticks every 60s; first scan after the first tick.
  - `func (s *Service) Scan(now time.Time) error`: loads the three template rows once per scan; for each User in scope it reads `subscriptions.Current`, skips Users without one (state untouched), computes `Condition`, calls `Decide`, and in **one transaction** upserts the state row and creates one Sendout per decided Kind whose row is `enabled`. Sendout draft: `Subject/HTML/Text` from `Render`, `Content` from the template, `Language = LanguageBilingual`, `Audience = AudienceSingle`, `Source = SourceAutomatic`, `AutomaticKind = kind`, no creator. A per-User error is logged as `[automatic-emails] user=%s: %v` and the scan continues. Calls `notifier.Notify()` once if anything was queued.

- [ ] **Step 1:** Write tests (all call `Scan` directly with explicit times):
  - `TestScanSendsLowOnceUntilRecovered`: allowance 100, used 95 → one Low Sendout; second scan → still one; top-up to 20 remaining → none; used again to <10% → second Low Sendout.
  - `TestScanSendsOnlyExhaustedWhenBothCross`: from 50% straight to 0 → one Sendout, kind `allowance_exhausted`.
  - `TestScanSendsRestoredAfterTopUp` and `TestScanSendsRestoredAtNewWindow` (30-day Type, scan at day 31).
  - `TestScanSkipsDisabledKindAndUnusableUsers`: disabled User, unverified User and Legacy Unmetered User get nothing.
  - `TestScanTracksStateWhileKindDisabled`: Low disabled, User drops to 5%, scan; enable Low, scan → no Sendout.
  - `TestScanRollsBackStateWhenSendoutFails`: make the template html exceed the collection max so `Create` fails → state row unchanged, next valid scan sends.
  - `TestScanRendersRecipientAndSource`: Sendout has `source=automatic`, `language=bilingual`, one recipient with the User's email, subject placeholders filled.
- [ ] **Step 2:** Run `go test ./internal/automaticemails`; expect FAIL.
- [ ] **Step 3:** Implement `service.go`; in `main.go` construct `automaticemails.New(app, sendoutService, frontendURL)` after `sendoutService` and `Start(ctx)` it next to `sendoutService.Start(ctx)`.
- [ ] **Step 4:** Run `go test ./... && go build ./... && go vet ./...`; expect PASS.
- [ ] **Step 5:** Commit `feat(automatic-emails): scan users and queue automatic sendouts`.

---

### Task 5: Admin API, Sendout source filter, docs

**Files:**
- Create: `backend/internal/api/automatic_emails.go`, `backend/internal/api/automatic_emails_test.go`
- Modify: `backend/internal/api/api.go:152-160` (routes), `backend/internal/api/dto.go` (types below + `EmailSendout`), `backend/internal/api/email_sendouts.go:177-226` (source fields, filter), `backend/internal/api/openapi.go` (schemas + paths near line 2195), `backend/README.md`, `AGENTS.md`

**Interfaces:**
- Consumes: Task 2 collections, Task 3 `ParseKind` / `ValidateTemplate` and its errors.
- Produces (all `adminOnly`):
  - `GET /automatic-emails` → `AutomaticEmailsResponse{ SMTPEnabled bool; AppName string; FrontendURL string; Kinds []AutomaticEmailKindSummary }`; summary `{ Kind, Enabled bool, ThresholdPercent *int (only low), Subject string, HasTemplate bool, Updated string }` in `Kinds` order.
  - `GET /automatic-emails/{kind}` → `AutomaticEmailKindDetail` = summary + `HTML`, `Text`, `Content map[string]any` (nil when none).
  - `PUT /automatic-emails/{kind}/template` body `AutomaticEmailTemplateRequest{ Subject, HTML, Text, Content }` → detail. Reuses `normalizeSendoutSubject` and the Sendout size limits; `ValidateTemplate` errors map to 400 with messages `unknown placeholder`, `a placeholder was split by formatting; retype it as plain text`, `placeholders cannot be used inside links`.
  - `PATCH /automatic-emails/{kind}` body `AutomaticEmailKindUpdateRequest{ Enabled *bool; ThresholdPercent *int }` → detail. 400 `save a template before enabling` when enabling without a template; 400 when threshold is outside 1–99 or sent for a non-low Kind.
  - Unknown `{kind}` → 404.
  - `GET /email-sendouts?source=manual|automatic` filters; any other non-empty value → 400. `EmailSendout` gains `Source string` and `AutomaticKind *string`.
- Docs: add the endpoints to `backend/README.md`; in `AGENTS.md` add one line to the 职责边界 list, the two collections and the new Sendout fields to 数据模型, the endpoints to 面板接口, and `automaticemails/` to the directory tree.

- [ ] **Step 1:** Write tests: `TestAutomaticEmailsList` (three kinds, low threshold 10), `TestSaveAutomaticEmailTemplateValidates` (each error maps to 400), `TestEnableRequiresTemplate`, `TestThresholdValidation` (0, 100, threshold on exhausted → 400; 25 on low → saved), `TestUnknownKind404`, `TestListEmailSendoutsFiltersBySource`, and extend the existing OpenAPI coverage test if it enumerates paths.
- [ ] **Step 2:** Run `go test ./internal/api -run 'AutomaticEmail|Threshold|UnknownKind|FiltersBySource'`; expect FAIL.
- [ ] **Step 3:** Implement handlers, DTOs, routes and OpenAPI; update docs.
- [ ] **Step 4:** Run `go test ./... && go build ./... && go vet ./... && make openapi`; expect PASS and an updated `openapi.json`.
- [ ] **Step 5:** Commit `feat(automatic-emails): admin endpoints and sendout source filter`.

---

### Task 6: Frontend building blocks

**Files:**
- Create: `frontend/src/emails/automatic-email-defaults.ts`
- Modify: `frontend/src/emails/service-email-frame.tsx`, `frontend/src/emails/sendout-editor.tsx`, `frontend/src/api/queries.ts`, `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes: Task 5 endpoints via regenerated `schema.d.ts`.
- Produces:
  - `type FrameLanguage = SendoutLanguage | "bilingual"`; `ServiceEmailFrameProps.language: FrameLanguage`. Bilingual renders `lang="zh-CN"`, the notice and panel link first with `{ locale: "zh-cn" }` then `{ locale: "en" }`.
  - `SendoutEditor` gains `initialContent?: Record<string, unknown>` passed to `EditorProvider`'s `content`.
  - `automaticEmailDefaults: Record<AutomaticEmailKind, { subject: string; content: Record<string, unknown> }>`: TipTap JSON documents, Chinese paragraphs, a divider node, then English paragraphs, using placeholders in plain text only (no links). These are template content, not UI copy, so they live here and not in `messages/`; add a one-line comment saying so.
  - Query hooks: `useAutomaticEmails()`, `useAutomaticEmail(kind)`, `useSaveAutomaticEmailTemplate(kind)`, `useUpdateAutomaticEmailKind(kind)`; `useEmailSendouts` gains an optional `source` filter. Mutations invalidate the list and detail keys.

- [ ] **Step 1:** Run `pnpm api:sync`; confirm `schema.d.ts` contains `/api/panel/automatic-emails`.
- [ ] **Step 2:** Implement the frame, editor prop, defaults and hooks.
- [ ] **Step 3:** Run `pnpm typecheck && pnpm i18n:check`; expect PASS.
- [ ] **Step 4:** Commit `feat(automatic-emails): bilingual frame, editor content and query hooks`.

---

### Task 7: Settings pages and history filter

**Files:**
- Create: `frontend/src/routes/settings/automatic-emails/route.tsx`, `index.tsx`, `$kind.tsx`
- Modify: `frontend/src/routes/settings/index.tsx:296-315` (link next to emails), `frontend/src/routes/settings/emails/index.tsx` (manual / automatic filter and a Kind label on automatic rows), `frontend/messages/en.json`, `frontend/messages/zh-cn.json`, `frontend/src/routeTree.gen.ts` (regenerated)

**Interfaces:**
- Consumes: Task 6 hooks, `SendoutEditor`, `ServiceEmailFrame`, `automaticEmailDefaults`.
- Produces:
  - `/settings/automatic-emails` (`requireAdmin`): one row per Kind with a Switch (disabled with a hint until `has_template`), the Low row's threshold number field (1–99, saved on blur through `useUpdateAutomaticEmailKind`), a link to edit the template, and an Alert when `smtp_enabled` is false.
  - `/settings/automatic-emails/$kind` (`requireAdmin`, 404 on unknown kind): TanStack Form with a subject field, the `SendoutEditor` loaded with saved `content` or the Kind default, `frame.language = "bilingual"`, a list of the nine placeholders with one-line descriptions, and Save (`compose()` then `useSaveAutomaticEmailTemplate`). Server 400 messages show inline.
  - Follow `routes/settings/emails/new.tsx` for editor lazy-loading (`lazy` + `useMounted`) and form layout.

- [ ] **Step 1:** Implement routes and the history filter, adding every string to both locales.
- [ ] **Step 2:** Run `pnpm dev` once so the route tree and Paraglide output regenerate, then `pnpm typecheck && pnpm i18n:check`; expect PASS.
- [ ] **Step 3:** Manual check with `pnpm dev`: save the default Low template, enable it, set threshold 25, set a test User's window usage to 80% in `/_/`, wait one scan, see an automatic Sendout in history with filled values in both languages.
- [ ] **Step 4:** Commit `feat(automatic-emails): settings pages and sendout history filter`.
