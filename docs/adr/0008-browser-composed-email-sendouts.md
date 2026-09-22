# Compose Email Sendouts in the browser and deliver them from a persisted outbox

## Decision

An administrator writes an Email Sendout in `@react-email/editor`, embedded in the panel with its built-in bubble menu and slash commands. A custom `BaseTemplate` wraps the content in the fixed service template for the Sendout's language (en or zh-cn). The browser then produces the final HTML and plain text with `composeReactEmail`. The backend validates the subject, HTML, text and editor JSON for size and emptiness, stores them, and sends the stored HTML and text unchanged through PocketBase's configured SMTP. The backend holds no email template.

Creating a Sendout writes one Sendout Recipient row per addressed User in the same transaction. For "all Users", that means every User who is active and Verified at creation time. A single in-process worker drains pending rows in FIFO order at the per-minute rate stored in `app_settings`. Before each send it rechecks that the User is active and Verified, and it uses the User's current email. Each row gets one attempt. On failure the row becomes `failed` with a safe error code, and the administrator can resend single rows or every failed row of a Sendout. A row left `sending` by a crash becomes `failed` with `interrupted` rather than being sent again automatically. Sendout history has no retention limit.

## Considered options

Pre-rendering React Email templates at build time into Go `html/template` files would let the backend enforce the template frame. It leaves room only for plain-text slots, though, and the editor's rich content needs the browser because `composeReactEmail` requires a live TipTap editor instance. A hybrid that sanitizes a browser-made fragment into a Go-held frame would need an inline-style allowlist kept in step with the editor, plus a backend preview so that what the administrator sees matches what goes out.

The Monitoring model of fire-and-forget goroutines (ADR 0002) was rejected for Sendouts. Transactional mail that recipients cannot unsubscribe from needs restart safety, visible progress and a record of who was sent what.

## Consequences

The frontend guarantees the template frame, not the backend. That is acceptable because Sendout endpoints are admin-only, and admins are already trusted with Node API URLs and Channel URLs. Stored HTML is displayed only in a sandboxed iframe. No per-recipient personalization is possible. Creating a Sendout is refused while PocketBase SMTP is disabled. If SMTP is disabled while a Sendout is still pending, the worker keeps draining and marks those rows `failed` with `smtp_disabled`. It does not pause, so the worker needs no extra state for SMTP having just been turned off. Recipients cannot see Sendouts in the panel, and the editor's Inspector panel is not used, so administrators control content but not layout or colors.
