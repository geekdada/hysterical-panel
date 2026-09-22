# Subscription design

Status: implemented. This document records the subscription behavior and implementation checks.

## Access and grants

- `status=disabled` or `verified=false` blocks Node Client Auth regardless of subscription. A subscription is required for every new User, including admins. Panel login remains available without a subscription.
- Only Users already in the database at migration time start with `subscription_required=false`. They can use Nodes without a subscription, subject to Status and Verified. Their existing User Traffic totals continue to grow. Every new User starts with `subscription_required=true`, regardless of whether the panel, open registration, Management API, or PocketBase admin creates them.
- The first grant to an exempt User starts immediately and permanently changes `subscription_required` to true in the same transaction. Expiry, exhaustion, or revocation never restores the exemption.
- An admin may give a User one current 360-day grant and queue at most one additional 360-day grant. A new grant to a User with a current grant starts when that grant ends. There is no automatic renewal or early activation when allowance runs out.
- An admin may cancel a queued grant or terminate a current grant. If a current grant ends early, the queued grant retains its scheduled start and there may be a gap. A metered User cannot connect in a gap or after the final grant expires. Preserve past grants, including early termination, as subscription history. No separate admin action log is required.
- At the scheduled boundary, a usable queued grant takes over without disconnecting clients. If no usable grant takes over, Kick existing connections and reject reconnects. A current grant that becomes exhausted, is terminated, or loses allowance after a Type edit also triggers Kick. Use the stable User ID already used by the Node API; the historical credential IDs remain available for Traffic attribution, not Kick.

## Allowance

- A Subscription Type has a positive byte allowance and a reset interval of exactly 30 or 360 days. A grant lasts exactly 360 days. Time ranges are half-open and anchored to the grant's actual UTC start instant. A 30-day Type therefore has 12 windows. The UI uses binary units, for example 1 TiB is `1024^4` bytes.
- The current window's used amount is the sum of tx and rx across all Nodes. The grant stores window tx and rx separately for display; allowance, remaining, over-allowance and exhaustion Kicks use only their sum. Continue incrementing `users.used_tx` and `users.used_rx` as lifetime counters. Do not use those counters as the window's used amount. A 360-day allowance needs its own durable usage state because the existing hourly and daily history can be pruned after 30 days.
- Each grant also stores Grant Usage, the tx and rx counted across all of its windows. Window rollover, top-ups and Type allowance edits do not reset it. See [ADR-0007](adr/0007-subscription-usage-per-direction.md).
- Subscription byte counters are stored as decimal integer text because PocketBase NumberField converts values through `float64`; API amounts remain integer bytes.
- Each new window starts with zero used and the Type's current allowance. No unused balance or manual additions carry over. Disabled Users' grant clocks keep running; the Collector advances their Node cursors but does not count their Traffic.
- A manual top-up adds a positive byte amount, chosen by the admin, to the current window without changing used. It may be repeated. The panel enters that amount in GiB and prefills the Type allowance. Editing the Type allowance replaces the effective window allowance with the new Type value, regardless of previous top-ups; future top-ups use the new value. An edit can therefore immediately exhaust or restore an existing grant. Do not build a top-up operation log in this version.
- A Collector poll may put used beyond the allowance. Store the signed remaining value as allowance minus used. On the User detail page, display zero remaining and mark the window as over allowance rather than showing a negative amount. Once used reaches or exceeds allowance, reject new connections and Kick established ones.
- Assign a counter delta to the window containing its successful poll, even when the prior poll was in a different window. If `/traffic` polling fails, retain the current access decision until a later successful poll and then settle the delta. Do not add a separate stale-data warning to the panel. Kick remains best effort; this is not a strict byte cap.

## Types and interface

- Admins can create, list, edit, and remove Subscription Types. They may change the byte allowance at any time; once a Type has ever been granted, its reset interval is fixed and it cannot be deleted. It can be hidden from new grants without invalidating existing or queued grants. Allowance must be positive; there is no zero-allowance or unlimited Type.
- Remove the unused `users.quota_bytes` field and its User API input and output. Subscription Type allowance is the sole configured quota.
- A metered User's detail page shows the current and queued grants, dates, current window allowance, used amount with its tx and rx split, and remaining amount. A collapsed history lists expired and terminated grants with their dates and Grant Usage tx and rx. The User list shows the current window's used / allowance, tx and rx instead of lifetime User Traffic. Sorting by those columns ranks Users with a current grant by window used, tx or rx; everyone else follows in either direction, ordered by creation. If none is usable, show "no available subscription". For a legacy exempt User, show an exemption marker in the existing User detail attributes and in the User list's subscription column, where the tx and rx columns show that User's lifetime Traffic. Do not add a subscription section to that exempt User's detail page.
- Admins manage Types, grants, cancellation, termination, and top-ups. User detail data is available to the admin and that User; grant changes are admin-only. Panel routes and DTOs follow the existing `/api/panel/*` OpenAPI contract. Node auth callbacks remain outside the panel OpenAPI contract.

## Implementation checks

- Enforce one current and at most one queued grant under concurrent admin requests. A failed first grant must not change `subscription_required`.
- Keep a window's usage and Node counter cursor consistent when a poll is committed. Today the Collector saves cursor, lifetime totals, and hourly/daily aggregates separately; adding an allowance must not turn a partial save into permanently missing subscription usage.
- Check active access at Node Client Auth time, and schedule expiry and exhaustion Kicks without changing User Status. On restart, reconcile any grant whose end passed while the process was down.
- Cover the 30-day and 360-day boundaries, clock-independent UTC arithmetic, a late cross-window poll, a counter reset, concurrent grants, repeated top-ups, Type edits after top-ups, and the migration exemption in tests.
