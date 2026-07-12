# Separate Node credentials from stable client identity

Node Client Auth returns the User ID as the stable Node Client ID instead of returning the Auth String. Auth Strings live in a separate `user_auth_strings` history: exactly one is Current and may authenticate, while Retired values can never authenticate or be reused but remain associated with the User so `/traffic`, `/online`, and `/dump/streams` data from connections established before the change stays attributable.

## Consequences

Hysteria and AnyTLS Nodes no longer retain current credentials as accounting identifiers for new connections. The Collector and Live diagnostics canonicalize both User IDs and every historical Auth String to one User, and aggregate mixed counters before advancing the existing User+Node cursor. Auth Strings and User IDs are enforced as disjoint identifier namespaces so a reported ID can never resolve ambiguously. Product and Management APIs expose and search only the Current Auth String. Kick targets only the stable User ID; short-lived legacy-ID connections are allowed to drain naturally.
