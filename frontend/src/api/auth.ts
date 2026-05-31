import { createIsomorphicFn } from "@tanstack/react-start";
import { getCookie } from "@tanstack/react-start/server";
import {
  AUTH_COOKIE,
  buildAuthCookie,
  buildAuthCookieExpired,
  readAuthCookieValueClient,
} from "./cookie";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "";

export interface AuthUser {
  id: string;
  email: string;
  role: string;
  auth_string: string;
  status: string;
}

export interface Auth {
  token: string;
  user: AuthUser;
}

interface AuthResponse {
  token: string;
  record: AuthUser;
}

function parseAuth(raw: string | null | undefined): Auth | null {
  if (!raw) return null;
  try {
    const data = JSON.parse(raw) as { token?: string; record?: AuthUser };
    if (!data.token || !data.record) return null;
    return { token: data.token, user: data.record };
  } catch {
    return null;
  }
}

/**
 * Read the stored auth, working on both sides of SSR: the server reads the
 * request cookie, the client reads document.cookie. createIsomorphicFn keeps
 * the server-only import out of the client bundle. Both impls are defined so
 * neither side silently returns undefined.
 */
export const readAuthCookie = createIsomorphicFn()
  .server((): Auth | null => parseAuth(getCookie(AUTH_COOKIE)))
  .client((): Auth | null => parseAuth(readAuthCookieValueClient()));

export async function login(email: string, password: string): Promise<AuthUser> {
  const res = await fetch(`${BASE_URL}/api/collections/users/auth-with-password`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ identity: email, password }),
  });

  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message || `Authentication failed (${res.status})`);
  }

  const data = (await res.json()) as AuthResponse;
  document.cookie = buildAuthCookie(
    JSON.stringify({ token: data.token, record: data.record }),
  );
  return data.record;
}

export function clearAuth(): void {
  document.cookie = buildAuthCookieExpired();
}
