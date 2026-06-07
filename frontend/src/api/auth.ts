import { createIsomorphicFn } from "@tanstack/react-start";
import { getCookie } from "@tanstack/react-start/server";
import {
  AUTH_COOKIE,
  buildAuthCookie,
  buildAuthCookieExpired,
  readAuthCookieValueClient,
} from "./cookie";
import { fetchPanelConfig, resolveApiBaseUrl } from "./panel-config";
import type {
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
  RegistrationResponseJSON,
} from "@simplewebauthn/browser";

export interface AuthUser {
  id: string;
  email: string;
  role: "admin" | "user";
  auth_string: string;
  status: "active" | "disabled";
}

export interface Auth {
  token: string;
  user: AuthUser;
}

interface AuthResponse {
  token: string;
  record: AuthUser;
}

export interface Passkey {
  id: string;
  name: string;
  transports: string[];
  sign_count: number;
  backup_eligible: boolean;
  backup_state: boolean;
  clone_warning: boolean;
  created: string;
  updated: string;
  last_used_at: string;
}

interface PasskeyOptionsResponse<T> {
  challenge_id: string;
  options: T;
}

const FRESH_PASSWORD_LOGIN_KEY = "hp:fresh-password-login";

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

async function resolveAuthBase(): Promise<string> {
  const config = await fetchPanelConfig();
  let base = resolveApiBaseUrl(config);
  if (!base && typeof window !== "undefined") {
    base = window.location.origin;
  }
  return base.replace(/\/$/, "");
}

function storeAuthResponse(data: AuthResponse): AuthUser {
  document.cookie = buildAuthCookie(
    JSON.stringify({ token: data.token, record: data.record }),
  );
  return data.record;
}

function markFreshPasswordLogin(userId: string): void {
  if (typeof sessionStorage === "undefined") return;
  sessionStorage.setItem(FRESH_PASSWORD_LOGIN_KEY, userId);
}

export function consumeFreshPasswordLogin(userId: string): boolean {
  if (typeof sessionStorage === "undefined") return false;
  const stored = sessionStorage.getItem(FRESH_PASSWORD_LOGIN_KEY);
  if (stored !== userId) return false;
  sessionStorage.removeItem(FRESH_PASSWORD_LOGIN_KEY);
  return true;
}

export async function login(email: string, password: string): Promise<AuthUser> {
  const base = await resolveAuthBase();
  const res = await fetch(`${base}/api/collections/users/auth-with-password`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ identity: email, password }),
  });

  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body.message || `Authentication failed (${res.status})`);
  }

  const data = body as AuthResponse;
  const user = storeAuthResponse(data);
  markFreshPasswordLogin(user.id);
  return user;
}

export function clearAuth(): void {
  document.cookie = buildAuthCookieExpired();
}

export async function loginWithPasskey(
  useBrowserAutofill = false,
): Promise<AuthUser> {
  const { startAuthentication } = await import("@simplewebauthn/browser");
  const challenge =
    await apiFetch<PasskeyOptionsResponse<PublicKeyCredentialRequestOptionsJSON>>(
      "/api/panel/passkeys/login/options",
      { method: "POST" },
    );
  const credential = await startAuthentication({
    optionsJSON: challenge.options,
    useBrowserAutofill,
    verifyBrowserAutofillInput: useBrowserAutofill,
  });
  const auth = await apiFetch<AuthResponse>("/api/panel/passkeys/login/finish", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      challenge_id: challenge.challenge_id,
      credential,
    }),
  });
  return storeAuthResponse(auth);
}

export async function listPasskeys(userId: string): Promise<Passkey[]> {
  return apiFetch<Passkey[]>(`/api/panel/users/${encodeURIComponent(userId)}/passkeys`, {
    headers: authHeaders(),
  });
}

export async function registerPasskey(
  userId: string,
  name = "Passkey",
  useAutoRegister = false,
): Promise<Passkey> {
  const { startRegistration } = await import("@simplewebauthn/browser");
  const challenge =
    await apiFetch<PasskeyOptionsResponse<PublicKeyCredentialCreationOptionsJSON>>(
      `/api/panel/users/${encodeURIComponent(userId)}/passkeys/registration/options`,
      {
        method: "POST",
        headers: authHeaders(),
      },
    );
  const credential: RegistrationResponseJSON = await startRegistration({
    optionsJSON: challenge.options,
    useAutoRegister,
  });
  return apiFetch<Passkey>(
    `/api/panel/users/${encodeURIComponent(userId)}/passkeys/registration/finish`,
    {
      method: "POST",
      headers: { ...authHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({
        challenge_id: challenge.challenge_id,
        credential,
        name,
      }),
    },
  );
}

export async function deletePasskey(
  userId: string,
  passkeyId: string,
): Promise<void> {
  await apiFetch(
    `/api/panel/users/${encodeURIComponent(userId)}/passkeys/${encodeURIComponent(passkeyId)}`,
    {
      method: "DELETE",
      headers: authHeaders(),
    },
  );
}

export function isPasskeySoftError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return (
    error.name === "AbortError" ||
    error.name === "NotAllowedError" ||
    error.message.toLowerCase().includes("not allowed") ||
    error.message.toLowerCase().includes("cancel")
  );
}

function authHeaders(): Record<string, string> {
  const raw = readAuthCookieValueClient();
  if (!raw) return {};
  try {
    const { token } = JSON.parse(raw) as { token?: string };
    return token ? { Authorization: token } : {};
  } catch {
    return {};
  }
}

async function apiFetch<T = unknown>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const base = await resolveAuthBase();
  const res = await fetch(`${base}${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message || `Request failed (${res.status})`);
  }
  return (await res.json()) as T;
}
