---
status: accepted
---

# Act as a narrow OIDC provider for admin-registered Connected Apps

The panel becomes an OpenID Connect provider so first-party sites can sign Users in with their panel account. The first consumer is the help center, which today signs in through Auth0 and then looks the account up by email through the Management API. Research and sources are in `docs/research/2026-10-12-oauth-provider.md`.

We implement a narrow profile ourselves instead of using a library. fosite's standalone repository stopped in July 2025, zitadel/oidc v3 cannot disable `plain` PKCE or send the RFC 9207 `iss` parameter and brings its own router, and luikyv/go-oidc cannot load clients from the database. We also considered running an external IdP. None of them can read the `users` collection, so the `status` and `verified` gates would have to live in two systems.

## Consequences

- Only admins register Connected Apps. There is no dynamic registration. A Connected App is a confidential client with a SHA-256-hashed secret shown once on creation and rotation. Public clients, device flow, `client_credentials`, PAR, JAR, DPoP and introspection are out of scope.
- The only grant is Authorization Code with PKCE `S256`, required for every client. `redirect_uri` must match a registered value exactly. Codes live 60 seconds and are single-use. Replaying a code revokes the grant.
- Access and refresh tokens are opaque random strings stored as SHA-256. Refresh tokens are issued when the client allows it and the request includes `offline_access`. They rotate on every use, and replaying a rotated one revokes the grant. Lifetimes are fixed in code: authorization request 10 minutes, code 60 seconds, access token and ID token 10 minutes, refresh token 30 days sliding with a 90-day absolute cap.
- ID tokens are RS256. The signing key is encrypted with `PANEL_MASTER_KEY` through `cryptobox`. Keys rotate only through a CLI subcommand, which keeps the retired key in JWKS until tokens signed with it expire.
- Protocol endpoints live under `/api/oidc/*`, stay out of OpenAPI and return 404 while the `app_settings` switch is off, like the Management API. The issuer is `PANEL_BACKEND_URL_BASE + "/api/oidc"` and must not change once published.
- Any User who is active and verified may sign in to any enabled Connected App. The check lives in one function so per-app restrictions can be added there later, as `nodesForUser` does for Nodes. Approve, token, refresh and userinfo all repeat the check.
- Consent is remembered per User, Connected App and scope set, and asked again only when the scopes grow. An admin can mark a Connected App to skip consent. The default is off.
- Claims are `sub` (the User ID), `email` and `email_verified`. There is no `profile` scope because the panel does not maintain `name` or an avatar. `auth_string` and `role` are never exposed. Sites that need the Current Auth String keep calling the Management API server-side, keyed by `sub`.
- Disabling or unverifying a User revokes their grants from a `users` record hook, so changes made in `/_/` also apply. Changing a password or removing a passkey does not revoke grants.
- `prompt=login` and `max_age` send the User back through the panel login page, with any login method. The panel records the time of each real sign-in against the session token and carries it across token refreshes. It sets `auth_time` from that record, so a recent login on another device does not satisfy `max_age`.
- "Client" already means Node Client in `CONTEXT.md`, and `oauth2_*` names the panel acting as an OAuth client. The provider side uses the `oidc` prefix in code and "Connected App" in the glossary.
