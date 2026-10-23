# Management API subscription management design

## Scope

External provisioning and support systems use the existing Management API token. The API exposes available Subscription Types and the existing User Subscription operations. It does not expose Subscription Reschedule, Subscription Type writes, token scopes, idempotency, or combined User creation and granting.

## Routes and responses

| Route | Behavior | Success response |
|---|---|---|
| `GET /api/mgmt/subscription-types` | List non-hidden Types by name | 200, array of `ManagementSubscriptionType` |
| `GET /api/mgmt/users/{id}/subscriptions` | List all grants by descending `starts_at` | 200, array of `UserSubscription` |
| `POST /api/mgmt/users/{id}/subscriptions` | Grant using only `subscription_type` | 200, `UserSubscription` |
| `POST /api/mgmt/users/{id}/subscriptions/{subscriptionId}/top-up` | Add positive `allowance_bytes` to the current grant | 200, `UserSubscription` |
| `DELETE /api/mgmt/users/{id}/subscriptions/{subscriptionId}` | Cancel queued or terminate current | 200, `DeleteResponse` |

`ManagementSubscriptionType` contains only `id`, `name`, `allowance_bytes`, and `reset_days`. The grant list includes current, queued, expired, and terminated history. Existing `UserSubscription` fields expose current Allowance Window usage and Grant Usage.

Grant behavior is exactly the panel behavior: a grant lasts 360 days, starts now without a current grant, otherwise queues after the current grant, and is rejected when a queued grant exists. The first grant permanently enables Subscription Required. Hidden Types cannot be granted. Callers cannot supply dates, allowance, or status.

Top-up accepts an integer byte amount greater than zero and only applies to the current grant. Terminating current starts the existing asynchronous best-effort Kick. Cancelling queued does not Kick. Both preserve history.

## Errors and authentication

- 400 for invalid JSON or fields, unavailable Types, and non-positive Top-up amounts.
- 404 for a missing User, missing Subscription, or Subscription belonging to another User.
- 409 when a queued grant already exists, Top-up targets a non-current grant, or DELETE targets an ended grant.
- The existing Management API gate remains unchanged: disabled feature 404, missing or invalid token 401.
- Successful operations return 200, including Grant.

Grant and Top-up are not idempotent. A repeated Top-up adds again. A repeated Grant can queue a renewal. Callers must GET and reconcile after an unknown outcome instead of blindly retrying.

## Documentation

Add the five operations to the Management API OpenAPI document with typed examples and explicit retry warnings. The existing route/spec consistency tests must cover them. The frontend management API page renders them without frontend changes.

Update `CONTEXT.md`, `docs/subscriptions.md`, `backend/README.md`, and ADR-0013. No database migration or panel OpenAPI change is needed.
