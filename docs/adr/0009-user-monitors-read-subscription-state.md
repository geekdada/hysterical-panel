---
status: accepted
---

# Evaluate User Monitors from subscription state

Low Allowance and Expiring Subscription Monitors read each User's current User Subscription directly, not Observations. An Alert's subject is now one Node or one User. ADR-0002 still governs Node Monitors.

We considered having the Collector write per-User Observations. The conditions change with Allowance Top-ups, Type allowance edits, window rollover, queued grants and Subscription Reschedules, and no Observation interval records those. We also considered a separate `user_alerts` collection. It would duplicate the Alert lifecycle, Notification delivery, retention and the alert summary.

## Consequences

- An Alert references exactly one Node or one User when it opens. It snapshots the User's email, so the history and both Notifications stay readable after the User changes email or is deleted. Deleting a User cancels that User's firing Alerts and clears the reference. The row remains.
- User Monitors cover every active, Verified User with a current User Subscription. They have no Node scope and no evaluation window. Their only config is a threshold: a percentage from 1 to 99, or a number of days from 1 to 90. They run every 60 seconds instead of every 5.
- Remaining allowance and remaining time only grow back through top-ups, Type edits, rollover, reschedules or a queued grant, so neither condition uses hysteresis. Low Allowance fires below the threshold share of the window's allowance, top-ups included, and resolves at or above it.
- An Alert resolves only when its condition clears. Grant expiry, grant termination, or a User becoming disabled, unverified or deleted cancels it without a Notification.
- Notifications go only to the Monitor's Channels for now. A later change will notify the affected User by email through PocketBase SMTP, not through a Notification Channel. To keep that open, firing and recovery values store every figure a User-facing message would need, and message rendering keeps the recipient separate from the content.
- No browser is involved when an Alert fires, so that email cannot be composed in the browser the way an Email Sendout is (ADR-0008). The server will render it from its own template with a subject, HTML and text. That change still has to decide on a Monitor switch to email the User, a delivery target type on `alert_deliveries`, the source of a User's language, whether to send to the snapshot or the current email, what happens when SMTP is off, and whether delivery reuses the Email Sendout queue.
