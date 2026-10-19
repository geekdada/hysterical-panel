# Management API Node URL changes require a new secret

The Management API manages Nodes through the same handlers as the panel, with one extra rule. A `PATCH /api/mgmt/nodes/{id}` that changes `api_url` must also supply a new `api_secret`, or the API rejects it with 400. The Collector sends the stored secret to whatever `api_url` the Node has. Without this rule, a holder of the Management API token could point a Node at their own server and capture its Stats API secret on the next poll. The panel skips the rule because its callers are admins, inside the trust boundary. The Management API token goes to external systems and gets less trust.

## Consequences

- The API detects a URL change by comparing the value with the stored one, so a PATCH that echoes the unchanged `api_url` needs no secret.
- The check lives in a thin Management API wrapper. The shared node handlers do not know whether the panel or the Management API called them.
- `POST /api/mgmt/nodes/{id}/test` uses only the stored URL and secret, so a test cannot send the secret to a new address.
