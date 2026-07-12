# Hysterical Panel

A lightweight management panel for Hysteria 2 / AnyTLS nodes: user and node registry, traffic aggregation, and HTTP auth callbacks. Not a subscription, billing, or node-deployment product.

## Language

**User**:
The person who can log into the panel and whose credential authenticates client connections to nodes. One record covers both; there is no separate account layer.
_Avoid_: Account, Client (for this meaning), Member, Operator (as the entity)

**Auth String**:
One of the User's node credentials, presented raw by Hysteria or hashed by AnyTLS when a client connects. Independent of panel login email and password; its AnyTLS hash is a derived lookup key, not a second credential.
_Avoid_: password (for node auth), API key, token, username, account name

**Current Auth String**:
The User's sole Auth String accepted for new Node Client Auth. Product APIs that expose or search `auth_string` always mean this value.
_Avoid_: active credential, latest key, primary Auth String

**Retired Auth String**:
An Auth String that no longer authenticates new connections but remains associated with its User so legacy Node Client IDs in Node statistics stay attributable. It can never become Current again.
_Avoid_: expired key, disabled credential, alias

**Node Client ID**:
The stable User ID returned by successful Node Client Auth and subsequently used by Nodes for Traffic, Online Device Count, Live, and Kick. It identifies the User without retaining their Auth String as the Node's accounting key.
_Avoid_: Auth String, credential, username, traffic key

**Status**:
Whether the User is `active` or `disabled`. The single on/off switch for panel login, new node connections, and whether traffic is counted.
_Avoid_: enabled, banned, suspended (as synonyms for this switch)

**Verified**:
Whether the User has completed email (or equivalent) confirmation. Required in addition to Status=`active` for the User to be usable.
_Avoid_: enabled, activated (as synonyms for Verified)

**Role**:
Whether the User is `admin` (manages the panel) or `user` (self-service only). Orthogonal to Status and Verified.
_Avoid_: permission level, tier, plan, operator, staff, member (as Role synonyms)

**Node**:
A Hysteria 2 or AnyTLS instance the panel knows how to call (traffic stats, kick) and that may call the panel for client auth. Soft-deleted Nodes remain Nodes for historical traffic attribution. The panel stores interface info only; it does not deploy or own the machine.
_Avoid_: Server, Instance, Endpoint, Proxy, Host (as the domain noun)

**Hysteria**:
The Hysteria 2 protocol / node software. In prose and UI copy always **Hysteria** (or **Hysteria 2** when the version matters); in code identifiers use `hysteria`.
_Avoid_: hysteria (in copy), Hy2 (as the product name)

**AnyTLS**:
The AnyTLS protocol / node software (the stats-and-http-auth fork the panel integrates with). In prose and UI copy always **AnyTLS**; in code identifiers use `anytls`.
_Avoid_: anytls (in copy), AnyTLS-Go (as the domain noun)

**Enabled** (Node):
Whether the Node participates in polling, User visibility, and kick fan-out. Orthogonal to Soft-deleted: a Soft-deleted Node is never treated as Enabled for those purposes.
_Avoid_: online, active (as synonyms for Enabled)

**Soft-deleted**:
A Node removed from the panel's active set but retained so historical traffic still attributes to it. Distinct from Enabled=false.
_Avoid_: destroyed, archived, offline, disabled (as the name for this state)

**Traffic**:
Aggregated tx/rx bytes attributed to a User on a Node over time (hourly/daily buckets and User totals). Not a billable quantity; reserved quota fields are not a product concept yet.
_Avoid_: usage (as the noun), bandwidth, transfer, quota, consumption, metering

**Online Device Count**:
The latest number of client instances reported by a Node for a Node Client ID. A User's count merges stable and legacy IDs and sums Enabled Nodes without deduplicating physical devices; a Node's total also includes unknown IDs.
_Avoid_: unique devices, physical devices, active streams, connections

**Collector**:
The background process that periodically reads each Enabled Node's traffic counters and online client-instance counts, recording Traffic deltas and the latest Online Device Count projection. Distinct from Live diagnostics, which are on-demand and not stored.
_Avoid_: scraper, syncer, meter, poller, importer

**Live**:
An on-demand, uncached diagnostic snapshot of current streams for a User or a Node, pulled from Nodes at request time. Not Traffic or Online Device Count; not persisted.
_Avoid_: realtime monitor, session log, online status (as the feature name), telemetry dump

**Kick**:
A best-effort request to Nodes to drop a User's already-established sessions, typically when Status becomes `disabled`. Clears existing sessions only; Status and Verified continue to block reconnects.
_Avoid_: ban, terminate, force logout, disconnect (as the feature name)

**Invitation**:
A reusable (or limited-use) code that may be required to register, depending on settings. Optional email on an Invitation is metadata or a mail target only, not an entitlement binding.
_Avoid_: invite link (delivery vehicle), coupon, referral, access code, voucher

**Registration Policy**:
The hierarchical rules that decide whether self-serve registration is allowed and whether an Invitation is required. The individual setting knobs are not separate domain nouns.
_Avoid_: signup mode, access mode, onboarding settings (as the domain noun)

**Node Client Auth**:
The panel's HTTP auth callback for Nodes deciding whether a client may connect. Hysteria and AnyTLS validate the Current Auth String through different lookup forms, then return the same stable Node Client ID. Distinct from panel login and from the Management API.
_Avoid_: webhook (as the feature name), AAA, login API, auth server

**Management API**:
An optional, default-absent external channel for systems to query or create Users with a Bearer token. Distinct from panel login and Node Client Auth; when disabled it must not advertise itself.
_Avoid_: admin API, public API, provisioning API, service account API (as the product name)

**Passkey**:
A WebAuthn credential for panel login, offered alongside password rather than replacing it. Distinct from Auth String and from Node Client Auth.
_Avoid_: security key (as the only name), biometric login, 2FA, MFA, WebAuthn (as the user-facing noun)

**Recent Connection**:
A remembered client IP (no port) on a User from successful Node Client Auth. Capped and distinct from a full session log; ASN/country are enrichment at read time, not stored facts.
_Avoid_: connection history, session log, IP log (as the domain noun)

**Ignored Connection IP**:
A globally configured client IP that must not be recorded as a Recent Connection. Admin-managed; does not by itself block Node Client Auth.
_Avoid_: IP blocklist, banlist, deny list (as the name for this)

**Notification Channel**:
An admin-managed outbound destination for panel messages. A Notification Channel has no triggering logic; Monitors may reference zero or more Channels.
_Avoid_: provider, notifier, alert rule, webhook (as the broad feature name)

**Channel URL**:
The complete Shoutrrr configuration string for one Notification Channel, including its destination and any credentials. It is secret on read.
_Avoid_: endpoint, callback URL, token (as the complete configuration name)

**Channel Test**:
An explicit administrator-initiated verification delivery to one Notification Channel. It is neither a Notification Rule nor normal notification delivery.
_Avoid_: alert, rule run, manual message (as the feature name)

**Observation**:
A timestamped interval of Node monitoring data, expressed as observed bytes and elapsed time. Missing Observations mean unknown data rather than zero Traffic.
_Avoid_: sample (as the domain noun), metric row, Traffic bucket

**Monitor**:
An admin-configured condition evaluated over an Observation window for all Enabled Nodes or selected Nodes, with a severity and optional Notification Channels.
_Avoid_: Notification Rule, Alert Rule, health check

**Alert**:
One Monitor's lifecycle for one Node, from `firing` until it is `resolved` by healthy data or `cancelled` by Monitor, Node lifecycle, or scope changes.
_Avoid_: Monitor, Notification, incident (as the domain noun)

**Notification**:
A single best-effort delivery of an Alert firing or recovery transition to one Notification Channel.
_Avoid_: Alert, message queue, reminder

**Notification Language**:
The language a Monitor selects for its automatic Notifications. An Alert snapshots it so firing and recovery Notifications use the same language.
_Avoid_: interface language, Channel language, locale (as the domain noun)
