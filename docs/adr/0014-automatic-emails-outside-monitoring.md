---
status: accepted
---

# Send Automatic Emails outside Monitoring, from administrator-edited templates

Automatic Emails are a feature of their own, not a switch on User Monitors. Administrators must be able to edit what Users receive, and a Monitor only has a severity, a threshold and Channels. A switch would also tie user-facing mail to the admin Alert lifecycle, so a Monitor edit that cancels Alerts would change what Users get. Each Automatic Email Kind therefore has its own switch, its own Email Template and its own per-User state. The Low Allowance Monitor stays as it is and only notifies admins. This replaces the plan in ADR-0009 to add a Monitor switch that emails the User and to render the email from a template the server owns.

An administrator writes each Email Template in the same browser editor as Email Sendouts. The panel stores the subject, HTML and text, and at send time the backend only fills in `{{placeholders}}` with HTML-escaped values. It never composes layout. This is the one exception to ADR-0008, where the backend holds no template: it still holds no frame of its own, only the administrator's saved output with placeholders. Default templates and the fixed frame show Chinese and then English in one email, so no User language preference is needed.

## Considered options

- A Monitor switch, as ADR-0009 planned. We rejected it because the template could not be edited and because Alert lifecycle rules (cancellation on Monitor edits, firing and recovery pairs) do not match what Users should receive.
- Server-side Go templates. We rejected them because administrators could not edit them, which is the reason the feature exists.
- Fire-and-forget delivery like Notifications. We rejected it. Every Automatic Email becomes a single-recipient Email Sendout marked as automatic, so it shares the persisted outbox, rate limit, eligibility recheck, `interrupted` handling and manual resend from ADR-0008.

## Consequences

- Evaluation follows the User Monitor pattern: every 60 seconds over active, Verified Users with a current User Subscription. It sends on state transitions. Low Allowance Email fires on each drop below its threshold, Allowance Exhausted Email on each drop to zero or below, and Allowance Restored Email when an exhausted User has a positive remaining amount again. Rare cases such as Subscription Reschedule get no special handling.
- An Automatic Email is created even while SMTP is disabled. The worker marks it `failed` with `smtp_disabled`, it still counts as sent, and an administrator can resend it from Sendout history.
- Users cannot unsubscribe. The worker sends to the User's current email after rechecking active and Verified, like any Sendout Recipient.
- The frontend owns the template frame for Automatic Emails as it does for Email Sendouts. That is acceptable because the templates are admin-only.
