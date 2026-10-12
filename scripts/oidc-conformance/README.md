# OIDC conformance harness

Runs the OpenID Foundation conformance suite's `oidcc-basic-certification-test-plan` against a local panel. Results from the last run are in [`docs/oidc-conformance.md`](../../docs/oidc-conformance.md).

## Pieces

- `pkce-proxy.mjs` sits in front of the backend and is the issuer the suite sees. The provider requires S256 PKCE from every client, and the plan's modules do not send it, so the proxy adds a fixed challenge to `/authorize` and the matching verifier to `/token`. Requests that already carry PKCE pass unchanged.
- `driver.mjs` creates the plan through the suite's API and runs each module. When a module waits for a browser, it opens the URL in headless Chromium (as a form POST when the module asks for one), signs in on the panel, clicks Allow or Sign in again, and uploads a screenshot when the module asks for one. It writes a JSON summary after every module and exits non-zero if any module failed or did not finish.

## Run

1. Build and start the suite from a clone of `https://gitlab.com/openid/conformance-suite`. The recorded results used commit `95a1ed4`:

   ```bash
   git checkout 95a1ed4
   MAVEN_CACHE=~/.m2 docker compose -f builder-compose.yml run --rm builder
   docker compose -f docker-compose-dev.yml -f compose.override.yml up -d
   ```

   `compose.override.yml` adds a sidecar so the suite reaches the proxy on the host at `127.0.0.1:8092`. This works on Docker Desktop and OrbStack. Linux Docker Engine needs `extra_hosts: ["host.docker.internal:host-gateway"]` on the sidecar and the proxy bound to an address the bridge can reach.

   ```yaml
   services:
     hostbridge:
       image: alpine/socat
       network_mode: "service:server"
       command: TCP-LISTEN:8092,fork,reuseaddr,bind=127.0.0.1 TCP:host.docker.internal:8092
       depends_on:
        - server
   ```

2. Start the panel on spare ports, with the proxy as the public backend URL. Run each long-lived command in its own terminal from the repository root:

   ```bash
   pnpm install
   (cd backend && go build -o /tmp/hp-oidc-bin .)
   export PANEL_MASTER_KEY=conformance PANEL_BACKEND_URL_BASE=http://127.0.0.1:8092 \
     PANEL_FRONTEND_URL_BASE=http://127.0.0.1:3001 MMDB_DIR=$PWD/backend/mmdb
   /tmp/hp-oidc-bin superuser upsert root@example.test Password12345 --dir /tmp/hp-oidc-conformance

   # terminal 1, with the same exports
   /tmp/hp-oidc-bin serve --http 127.0.0.1:8091 --dir /tmp/hp-oidc-conformance
   # terminal 2
   node scripts/oidc-conformance/pkce-proxy.mjs 8092 http://127.0.0.1:8091
   # terminal 3
   (cd frontend && VITE_API_BASE_URL=http://127.0.0.1:8091 pnpm exec vite dev --host 127.0.0.1 --port 3001)
   ```

3. Sign in to `http://127.0.0.1:8091/_/` as the superuser and create a User `admin@example.test` with password `Password12345`, `role=admin`, `status=active` and `verified=true`. Sign in to the panel at `http://127.0.0.1:3001` as that User, turn on the OIDC provider in Settings, and register two Connected Apps. Keep the default `openid` and `email` scopes (`oidcc-scope-email` needs `email`), enable refresh, and use the redirect URI `https://localhost.emobix.co.uk:8443/test/a/hysterical-panel/callback`.

4. Write `config.json` from `config.template.json` with both apps' client IDs and secrets. `config.json` and `results.json` are gitignored. Then run:

   ```bash
   cd scripts/oidc-conformance
   pnpm install --ignore-workspace --no-lockfile
   pnpm exec playwright install chromium-headless-shell
   NODE_TLS_REJECT_UNAUTHORIZED=0 node driver.mjs config.json results.json
   ```

   Pass module names after `results.json` to run only those modules.

The suite's certificate is self-signed, which is why the driver runs with `NODE_TLS_REJECT_UNAUTHORIZED=0`. Use it only against the local suite.
