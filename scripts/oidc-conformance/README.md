# OIDC conformance harness

Runs the OpenID Foundation conformance suite's `oidcc-basic-certification-test-plan` against a local panel. Results from the last run are in [`docs/oidc-conformance.md`](../../docs/oidc-conformance.md).

## Pieces

- `pkce-proxy.mjs` sits in front of the backend and is the issuer the suite sees. The provider requires S256 PKCE from every client, and the plan's modules do not send it, so the proxy adds a fixed challenge to `/authorize` and the matching verifier to `/token`. Requests that already carry PKCE pass unchanged.
- `driver.mjs` creates the plan through the suite's API and runs each module. When a module waits for a browser, it opens the URL in headless Chromium, signs in on the panel, clicks Allow or Sign in again, and uploads a screenshot when the module asks for one. It writes a JSON summary.

## Run

1. Build and start the suite from a clone of `https://gitlab.com/openid/conformance-suite`:

   ```bash
   MAVEN_CACHE=~/.m2 docker compose -f builder-compose.yml run --rm builder
   docker compose -f docker-compose-dev.yml -f compose.override.yml up -d
   ```

   `compose.override.yml` adds a sidecar so the suite reaches the proxy on the host at `127.0.0.1:8092`:

   ```yaml
   services:
     hostbridge:
       image: alpine/socat
       network_mode: "service:server"
       command: TCP-LISTEN:8092,fork,reuseaddr,bind=127.0.0.1 TCP:host.docker.internal:8092
       depends_on:
        - server
   ```

2. Start the panel on spare ports, with the proxy as the public backend URL:

   ```bash
   cd backend && go build -o /tmp/hp-oidc-bin .
   PANEL_MASTER_KEY=conformance PANEL_BACKEND_URL_BASE=http://127.0.0.1:8092 \
     PANEL_FRONTEND_URL_BASE=http://127.0.0.1:3001 MMDB_DIR=$PWD/mmdb \
     /tmp/hp-oidc-bin serve --http 127.0.0.1:8091 --dir /tmp/hp-oidc-conformance
   node scripts/oidc-conformance/pkce-proxy.mjs 8092 http://127.0.0.1:8091
   cd frontend && VITE_API_BASE_URL=http://127.0.0.1:8091 pnpm exec vite dev --host 127.0.0.1 --port 3001
   ```

3. Create an admin User `admin@example.test` with password `Password12345`, enable the provider, and register two Connected Apps with refresh enabled and the redirect URI `https://localhost.emobix.co.uk:8443/test/a/hysterical-panel/callback`.

4. Write `config.json` from `config.template.json` with both apps' client IDs and secrets, then run:

   ```bash
   cd scripts/oidc-conformance
   pnpm install --ignore-workspace --no-lockfile
   pnpm exec playwright install chromium-headless-shell
   NODE_TLS_REJECT_UNAUTHORIZED=0 node driver.mjs config.json results.json
   ```

   Pass module names after `results.json` to run only those modules.

The suite's certificate is self-signed, which is why the driver runs with `NODE_TLS_REJECT_UNAUTHORIZED=0`. Use it only against the local suite.
