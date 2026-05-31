import createClient from "openapi-fetch";
import type { paths } from "./schema";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "";

export const apiClient = createClient<paths>({ baseUrl: BASE_URL });

let currentToken: string | undefined;

const authMiddleware = {
  onRequest({ request }: { request: Request }) {
    if (currentToken) {
      request.headers.set("Authorization", currentToken);
    }
    return undefined;
  },
};

apiClient.use(authMiddleware);

export function setAuthToken(token: string | undefined) {
  currentToken = token;
}

export type { paths, components } from "./schema";
