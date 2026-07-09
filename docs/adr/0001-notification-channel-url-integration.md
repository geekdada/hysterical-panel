# Use maintained Shoutrrr URLs for Notification Channels

## Decision

Each Notification Channel stores exactly one complete Shoutrrr configuration URL. The panel pins `github.com/nicholas-fedor/shoutrrr v0.16.1` and keeps the dependency isolated in `internal/notifications`. It validates locally and sends only the administrator's explicit fixed test message.

The accepted URL schemes are the Beszel notification-guide allowlist: Generic, Bark, Discord, Gotify, Google Chat, IFTTT, Join, Lark, Mattermost, Matrix, MQTT, ntfy, OpsGenie, Pushbullet, Pushover, Rocket.Chat, Signal, Slack, Teams, Telegram, Twilio, WeCom, and Zulip. Other Shoutrrr services, including SMTP, PagerDuty, Notifiarr, XMPP, and Logger, are intentionally rejected.

The complete URL is AES-GCM encrypted at rest. Normal Channel endpoints return only non-secret metadata. Plaintext reveal requires a current admin's fresh, user-verified passkey assertion using a five-minute, one-time `sensitive_field_reveal` session scoped to that Channel ID.

## Consequences

The panel does not need provider-specific configurations or a notification service of its own. It deliberately has no rules, automatic sends, retries, queue, scheduling, history, audit log, or Management API exposure. Private and self-hosted destinations remain allowed because Channel configuration has the same trusted-admin boundary as Node API configuration.
