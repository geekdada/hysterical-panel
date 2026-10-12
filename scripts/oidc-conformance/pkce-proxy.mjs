// Forwards every request to the panel backend and adds PKCE to OIDC requests
// that have none. The provider requires S256 PKCE from every client, and the
// conformance suite's Basic OP modules do not send it. A request that already
// carries PKCE passes unchanged, so the suite's own PKCE module still tests
// the provider.
//
// Usage: node pkce-proxy.mjs [listen-port] [backend-origin]
import { createHash } from "node:crypto";
import http from "node:http";

const listenPort = Number(process.argv[2] ?? 8092);
const backend = new URL(process.argv[3] ?? "http://127.0.0.1:8091");
const VERIFIER = "conformance-proxy-verifier-0123456789-abcdefghijklmnopqrstuvwxyz";
const CHALLENGE = createHash("sha256").update(VERIFIER).digest("base64url");
const FORM = "application/x-www-form-urlencoded";

function addChallenge(params) {
  if (params.has("code_challenge")) return false;
  params.set("code_challenge", CHALLENGE);
  params.set("code_challenge_method", "S256");
  return true;
}

function addVerifier(params) {
  if (params.get("grant_type") !== "authorization_code" || params.has("code_verifier")) return false;
  params.set("code_verifier", VERIFIER);
  return true;
}

function rewrite(req, url, body) {
  const isForm = (req.headers["content-type"] ?? "").startsWith(FORM);
  if (url.pathname === "/api/oidc/authorize") {
    if (req.method === "GET" && addChallenge(url.searchParams)) {
      console.log("[pkce-proxy] added code_challenge to GET /authorize");
    }
    if (req.method === "POST" && isForm) {
      const params = new URLSearchParams(body.toString());
      if (addChallenge(params)) {
        console.log("[pkce-proxy] added code_challenge to POST /authorize");
        return Buffer.from(params.toString());
      }
    }
  }
  if (url.pathname === "/api/oidc/token" && req.method === "POST" && isForm) {
    const params = new URLSearchParams(body.toString());
    if (addVerifier(params)) {
      console.log("[pkce-proxy] added code_verifier to POST /token");
      return Buffer.from(params.toString());
    }
  }
  return body;
}

http
  .createServer((req, res) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      const url = new URL(req.url, backend);
      const body = rewrite(req, url, Buffer.concat(chunks));
      const headers = { ...req.headers, "content-length": String(body.length) };
      const upstream = http.request(
        { host: backend.hostname, port: backend.port, method: req.method, path: url.pathname + url.search, headers },
        (upstreamRes) => {
          res.writeHead(upstreamRes.statusCode ?? 502, upstreamRes.headers);
          upstreamRes.pipe(res);
        },
      );
      upstream.on("error", (error) => {
        res.writeHead(502, { "content-type": "text/plain" });
        res.end(`backend unreachable: ${error.message}`);
      });
      upstream.end(body);
    });
  })
  .listen(listenPort, "127.0.0.1", () => {
    console.log(`[pkce-proxy] 127.0.0.1:${listenPort} -> ${backend.origin}`);
  });
