import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { readAuthCookieValueClient } from "./cookie";
import { fetchPanelConfig, resolveApiBaseUrl } from "./panel-config";
import { wrapFetchWithSession } from "./session";

const BOOTSTRAP_BASE = import.meta.env.VITE_API_BASE_URL || "";

async function resolveFetchOrigin(): Promise<string> {
  const config = await fetchPanelConfig();
  const base = resolveApiBaseUrl(config);
  if (base) return base;
  if (typeof window !== "undefined") return window.location.origin;
  return BOOTSTRAP_BASE;
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

export const apiClient = createClient<paths>({
  baseUrl: BOOTSTRAP_BASE,
  fetch: panelFetch,
});

const authMiddleware = {
  onRequest({ request }: { request: Request }) {
    // Read the token fresh from the cookie per request — no shared module
    // state, so no cross-request leakage. API calls run client-side.
    const raw = readAuthCookieValueClient();
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
