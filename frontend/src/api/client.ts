import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { readAuthCookieValueClient } from "./cookie";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "";

export const apiClient = createClient<paths>({ baseUrl: BASE_URL });

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
