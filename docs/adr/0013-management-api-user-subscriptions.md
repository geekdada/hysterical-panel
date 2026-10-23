---
status: accepted
---

# Let the Management API manage User Subscriptions

External provisioning and support systems can use the existing Management API Bearer token to read available Subscription Types and to list, grant, top up, terminate or cancel User Subscriptions. These operations follow the panel's subscription lifecycle. The Management API cannot change Subscription Types or reschedule grants, and creating a User remains a separate call.

Grant and Top-up have no idempotency mechanism in this version. We deferred persistent idempotency to keep this change scoped to the existing subscription model. A repeated Top-up adds its amount again, and a repeated Grant can queue a renewal. After an unknown request outcome, callers must read the User Subscriptions before deciding whether to retry.

## Consequences

- The same global token already trusted with Current Auth Strings and Node management also has these subscription permissions. There is no read-only scope or separate subscription switch.
- Management API state conflicts return 409, while malformed requests return 400 and missing or cross-User resources return 404. Panel responses keep their existing status codes.
- Terminating a current grant preserves its history and starts the same best-effort Kick as the panel. Cancelling a queued grant does not Kick.
- The Management API OpenAPI document warns that Grant and Top-up are not idempotent.
