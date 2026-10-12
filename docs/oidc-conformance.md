# OIDC conformance results

Run on 2026-10-12 with the OpenID Foundation conformance suite at commit `95a1ed4` (master, built locally). The harness and the steps to repeat the run are in [`scripts/oidc-conformance/`](../scripts/oidc-conformance/README.md).

- Plan: `oidcc-basic-certification-test-plan`
- Variant: `server_metadata=discovery`, `client_registration=static_client`, `client_auth_type=client_secret_basic` (the suite runs its `client_secret_post` module with that method)
- Provider: a local panel built from `oidc/5-finish`, two Connected Apps with refresh enabled, issuer `http://127.0.0.1:8092/api/oidc`

The provider requires S256 PKCE from every client (ADR-0010), and only one module in this plan sends PKCE. A test proxy therefore adds a fixed challenge to `/authorize` and the matching verifier to `/token` when a request has none. `oidcc-ensure-request-with-valid-pkce-succeeds` sends its own PKCE, the proxy leaves it alone, and it passes. A real relying party must send PKCE itself.

## Summary

| Result | Count |
|---|---|
| Passed | 21 |
| Warning | 5 |
| Review | 4 |
| Skipped | 5 |
| Failed | 0 |

## Warnings

| Module | Warning | Why |
|---|---|---|
| `oidcc-userinfo-post-body` | The access token in a form body is not accepted | Userinfo reads the token from the `Authorization` header only. RFC 6750 makes the body form optional, and OIDC Core recommends the header. |
| `oidcc-scope-email`, `oidcc-alternate-happy-flow` | `email` appears in the ID token without a `claims` request | Intentional. The help center reads `email` from the ID token, so the provider puts the claims the scope grants in both the ID token and userinfo. OIDC Core allows it. |
| `oidcc-ensure-request-with-acr-values-succeeds` | No `acr` claim | The panel has no authentication context classes. `acr_values` is ignored. |
| `oidcc-claims-essential` | No `name` in userinfo | The panel does not keep a name, so there is no `profile` scope (ADR-0010). The `claims` parameter is not supported and is ignored. |

## Review

These modules finish only after a person checks an uploaded screenshot. The driver uploaded the panel page the browser stopped on.

| Module | What the screenshot shows |
|---|---|
| `oidcc-prompt-login` | The panel sign-in page with "Sign in again to continue". The suite also checked that `auth_time` moved forward. |
| `oidcc-max-age-1` | The same sign-in page, and the same `auth_time` check. |
| `oidcc-ensure-registered-redirect-uri` | The panel error page "Can't continue to the app", which says the app asked to return to an address it has not registered. The browser is not sent to the unregistered URI. |
| `oidcc-ensure-request-object-with-redirect-uri` | The same error page. The module puts `redirect_uri` only inside a request object, which the provider does not read, so the request has no usable `redirect_uri`. The page text says "not registered" although the URI is missing. |

## Skipped

| Module | Why |
|---|---|
| `oidcc-scope-profile`, `oidcc-scope-address`, `oidcc-scope-phone`, `oidcc-scope-all` | Discovery does not list these scopes. |
| `oidcc-unsigned-request-object-supported-correctly-or-rejected-as-unsupported` | Discovery sets `request_parameter_supported` to false, and the provider answered `request_not_supported`. |
