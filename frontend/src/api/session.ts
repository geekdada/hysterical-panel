import { parseJwtExp, readAuthCookieValueClient, writeAuthCookie } from "./cookie";
import { fetchPanelConfig, resolveApiBaseUrl } from "./panel-config";
import type { Auth, AuthUser } from "./auth";

const CLOCK_SKEW_SECONDS = 30;
const REFRESH_SOON_SECONDS = 60 * 60 * 24; // 24h
const REFRESH_SOON_FRACTION = 0.2;

interface AuthResponse {
  token: string;
  record: AuthUser;
}

export let sessionRecovering = false;
let sessionRecoveryFailed = false;
const recoveryListeners = new Set<() => void>();

function notifyRecoveryListeners(): void {
  for (const listener of recoveryListeners) {
    listener();
  }
}

export function getSessionRecoveryFailed(): boolean {
  return sessionRecoveryFailed;
}

export function subscribeSessionRecovery(listener: () => void): () => void {
  recoveryListeners.add(listener);
  return () => recoveryListeners.delete(listener);
}

export function isTokenUsable(token: string): boolean {
  const exp = parseJwtExp(token);
  if (!exp) return true;
  return exp > Date.now() / 1000 + CLOCK_SKEW_SECONDS;
}

const DEFAULT_TOKEN_LIFETIME_SECONDS = 432000; // PocketBase default (5 days)

function parseJwtIat(token: string): number | null {
  try {
    const parts = token.split(".");
    if (parts.length < 2) return null;
    const payloadPart = parts[1];
    if (!payloadPart) return null;
    const payload = JSON.parse(atob(payloadPart.replace(/-/g, "+").replace(/_/g, "/"))) as {
      iat?: unknown;
    };
    return typeof payload.iat === "number" ? payload.iat : null;
  } catch {
    return null;
  }
}

export function shouldRefreshSoon(token: string): boolean {
  const exp = parseJwtExp(token);
  if (!exp) return true;
  const now = Date.now() / 1000;
  const remaining = exp - now;
  if (remaining <= CLOCK_SKEW_SECONDS) return false;
  const iat = parseJwtIat(token);
  const lifetime = iat ? Math.max(exp - iat, 1) : DEFAULT_TOKEN_LIFETIME_SECONDS;
  return remaining <= REFRESH_SOON_SECONDS || remaining <= lifetime * REFRESH_SOON_FRACTION;
}

function parseAuthFromRaw(raw: string | null | undefined): Auth | null {
  if (!raw) return null;
  try {
    const data = JSON.parse(raw) as { token?: string; record?: AuthUser };
    if (!data.token || !data.record) return null;
    return { token: data.token, user: data.record };
  } catch {
    return null;
  }
}

async function resolveAuthBase(): Promise<string> {
  const config = await fetchPanelConfig();
  let base = resolveApiBaseUrl(config);
  if (!base && typeof window !== "undefined") {
    base = window.location.origin;
  }
  return base.replace(/\/$/, "");
}

export async function refreshAuthToken(): Promise<Auth | null> {
  const auth = parseAuthFromRaw(readAuthCookieValueClient());
  if (!auth?.token) return null;

  const base = await resolveAuthBase();
  const res = await fetch(`${base}/api/collections/users/auth-refresh`, {
    method: "POST",
    headers: { Authorization: auth.token },
  });

  const body = (await res.json().catch(() => ({}))) as AuthResponse & {
    message?: string;
  };
  if (!res.ok || !body.token || !body.record) {
    return null;
  }

  writeAuthCookie(body.token, body.record);
  sessionRecoveryFailed = false;
  notifyRecoveryListeners();
  return { token: body.token, user: body.record };
}

let refreshFlight: Promise<Auth | null> | null = null;

export function refreshAuthTokenSingleFlight(): Promise<Auth | null> {
  if (!refreshFlight) {
    refreshFlight = refreshAuthToken().finally(() => {
      refreshFlight = null;
    });
  }
  return refreshFlight;
}

export async function recoverSession(): Promise<Auth | null> {
  try {
    const { loginWithPasskey } = await import("./auth");
    await loginWithPasskey(true);
    sessionRecoveryFailed = false;
    notifyRecoveryListeners();
    return parseAuthFromRaw(readAuthCookieValueClient());
  } catch (error) {
    const { isPasskeySoftError } = await import("./auth");
    if (!isPasskeySoftError(error)) {
      sessionRecoveryFailed = true;
      notifyRecoveryListeners();
    }
    return null;
  }
}

export async function ensureSessionFresh(options?: { force?: boolean }): Promise<Auth | null> {
  const auth = parseAuthFromRaw(readAuthCookieValueClient());
  if (!auth?.token || !isTokenUsable(auth.token)) return null;
  if (!options?.force && !shouldRefreshSoon(auth.token)) return auth;
  return refreshAuthTokenSingleFlight();
}

export async function handleUnauthorized(): Promise<string | null> {
  sessionRecovering = true;
  try {
    let auth = await refreshAuthTokenSingleFlight();
    if (!auth) {
      auth = await recoverSession();
    }
    if (auth?.token) {
      sessionRecoveryFailed = false;
      notifyRecoveryListeners();
      return auth.token;
    }
    sessionRecoveryFailed = true;
    notifyRecoveryListeners();
    return null;
  } finally {
    sessionRecovering = false;
  }
}

const SESSION_RETRY_HEADER = "x-hp-session-retry";

function requestUrl(input: RequestInfo | URL): string {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.href;
  return input.url;
}

export function isNoSessionRetryUrl(input: RequestInfo | URL): boolean {
  const url = requestUrl(input);
  return (
    url.includes("/auth-refresh") ||
    url.includes("/auth-with-password") ||
    url.includes("/passkeys/login/")
  );
}

function isRetriedRequest(init?: RequestInit): boolean {
  if (!init?.headers) return false;
  const headers = new Headers(init.headers);
  return headers.get(SESSION_RETRY_HEADER) === "1";
}

function withRetryInit(init: RequestInit | undefined, token: string): RequestInit {
  const headers = new Headers(init?.headers);
  headers.set("Authorization", token);
  headers.set(SESSION_RETRY_HEADER, "1");
  return { ...init, headers };
}

/** Wrap fetch to retry once after silent session refresh on 401. */
export function wrapFetchWithSession(baseFetch: typeof fetch): typeof fetch {
  return async (input, init) => {
    const response = await baseFetch(input, init);
    if (response.status !== 401) return response;
    if (isNoSessionRetryUrl(input) || isRetriedRequest(init)) return response;

    const token = await handleUnauthorized();
    if (!token) return response;

    if (input instanceof Request) {
      const retryRequest = new Request(input, withRetryInit(init, token));
      return baseFetch(retryRequest);
    }

    return baseFetch(input, withRetryInit(init, token));
  };
}

export function isSessionAuthError(error: unknown): boolean {
  if (
    error &&
    typeof error === "object" &&
    "status" in error &&
    (error as { status: unknown }).status === 401 &&
    "message" in error &&
    typeof (error as { message: unknown }).message === "string"
  ) {
    return (error as { message: string }).message.toLowerCase().includes("authorization token");
  }
  return false;
}

export function shouldSuppressSessionError(): boolean {
  return sessionRecovering;
}

/** Exported for tests — reset module state. */
export function resetSessionStateForTests(): void {
  sessionRecovering = false;
  sessionRecoveryFailed = false;
  refreshFlight = null;
}
