import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { readAuthCookieValue } from "./cookie";
import { fetchPanelConfig, resolveApiBaseUrl, resolveServerApiBaseUrl } from "./panel-config";
import { wrapFetchWithSession } from "./session";

const BOOTSTRAP_BASE = import.meta.env.VITE_API_BASE_URL || "";

async function resolveFetchOrigin(): Promise<string> {
  if (typeof window === "undefined") return resolveServerApiBaseUrl();

  const config = await fetchPanelConfig();
  const base = resolveApiBaseUrl(config);
  if (base) return base;
  return window.location.origin;
}

const panelFetchBase: typeof fetch = async (input, init) => {
  const origin = (await resolveFetchOrigin()).replace(/\/$/, "");

  if (typeof input === "string") {
    if (input.startsWith("http://") || input.startsWith("https://")) {
      return fetch(input, init);
    }
    const url = origin ? `${origin}${input}` : input;
    return fetch(url, init);
  }

  if (input instanceof Request) {
    const reqUrl = input.url;
    if (reqUrl.startsWith("http://") || reqUrl.startsWith("https://")) {
      return fetch(input, init);
    }
    const parsed = new URL(reqUrl, origin || "http://localhost");
    const path = `${parsed.pathname}${parsed.search}`;
    const url = origin ? `${origin}${path}` : path;
    return fetch(new Request(url, input), init);
  }

  return fetch(input, init);
};

const panelFetch = wrapFetchWithSession(panelFetchBase);

/**
 * openapi-fetch constructs `new Request(baseUrl + path)` before the custom
 * fetch runs. With a same-origin (empty) base that relative URL throws in
 * Node during SSR, so resolve the server API base at construction time.
 */
class PanelRequest extends Request {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    if (typeof window === "undefined" && typeof input === "string" && input.startsWith("/")) {
      super(`${resolveServerApiBaseUrl()}${input}`, init);
      return;
    }
    super(input, init);
  }
}

export const apiClient = createClient<paths>({
  baseUrl: BOOTSTRAP_BASE,
  fetch: panelFetch,
  Request: PanelRequest,
});

const authMiddleware = {
  onRequest({ request }: { request: Request }) {
    // Read the token per request: document.cookie in the browser, request
    // AsyncLocalStorage on the server, with no shared auth state.
    const raw = readAuthCookieValue();
    if (raw) {
      try {
        const { token } = JSON.parse(raw) as { token?: string };
        if (token) request.headers.set("Authorization", token);
      } catch {
        // Malformed cookie → send the request unauthenticated.
      }
    }
    return undefined;
  },
};

apiClient.use(authMiddleware);

export type { paths, components } from "./schema";
