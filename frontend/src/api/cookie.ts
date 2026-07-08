import { createIsomorphicFn } from "@tanstack/react-start";
import { getCookie } from "@tanstack/react-start/server";

export const AUTH_COOKIE = "hp_auth";
export const LEGACY_AUTH_COOKIE = "pb_auth";

export const MAX_AGE_CAP_SECONDS = 60 * 60 * 24 * 7; // 7 days

function secureAttr(): string {
  return typeof window !== "undefined" && window.location.protocol === "https:" ? "; Secure" : "";
}

/** Decode JWT payload `exp` (seconds since epoch). Scheduling only — not verified. */
export function parseJwtExp(token: string): number | null {
  try {
    const parts = token.split(".");
    if (parts.length < 2) return null;
    const payloadPart = parts[1];
    if (!payloadPart) return null;
    const payload = JSON.parse(atob(payloadPart.replace(/-/g, "+").replace(/_/g, "/"))) as {
      exp?: unknown;
    };
    return typeof payload.exp === "number" ? payload.exp : null;
  } catch {
    return null;
  }
}

/** Cookie Max-Age aligned with JWT expiry, capped at 7 days. */
export function cookieMaxAgeFromToken(token: string): number {
  const exp = parseJwtExp(token);
  if (!exp) return MAX_AGE_CAP_SECONDS;
  const remaining = Math.floor(exp - Date.now() / 1000);
  if (remaining <= 0) return 0;
  return Math.min(remaining, MAX_AGE_CAP_SECONDS);
}

const AUTH_RECORD_FIELDS = ["id", "email", "role", "status"] as const;

/** Keep only the fields the frontend consumes; the raw auth record carries
 * heavy, sensitive data (auth_string, recent_connections, usage counters) that
 * has no business living in a non-HttpOnly cookie sent on every request. */
export function sanitizeAuthRecord(record: unknown): Record<string, unknown> | null {
  if (!record || typeof record !== "object") return null;
  const source = record as Record<string, unknown>;
  const slim: Record<string, unknown> = {};
  for (const field of AUTH_RECORD_FIELDS) {
    if (field in source) slim[field] = source[field];
  }
  return slim;
}

/** True when a record carries keys beyond the sanitized whitelist. */
function recordHasExtraFields(record: unknown): boolean {
  if (!record || typeof record !== "object") return false;
  return Object.keys(record as Record<string, unknown>).some(
    (key) => !(AUTH_RECORD_FIELDS as readonly string[]).includes(key)
  );
}

function readRawCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const prefix = `${name}=`;
  for (const part of document.cookie.split(/;\s*/)) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length));
    }
  }
  return null;
}

/** Build the document.cookie string for the panel auth cookie. */
export function buildAuthCookie(value: string, maxAgeSeconds?: number): string {
  const maxAge = maxAgeSeconds ?? MAX_AGE_CAP_SECONDS;
  return (
    `${AUTH_COOKIE}=${encodeURIComponent(value)}` +
    `; Path=/` +
    `; Max-Age=${maxAge}` +
    `; SameSite=Lax` +
    secureAttr()
  );
}

/** Expire the panel auth cookie immediately. */
export function buildAuthCookieExpired(): string {
  return `${AUTH_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax${secureAttr()}`;
}

/** Expire the legacy PocketBase cookie name. */
export function buildLegacyAuthCookieExpired(): string {
  return `${LEGACY_AUTH_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax${secureAttr()}`;
}

/** Persist auth JSON to hp_auth with Max-Age derived from the JWT. */
export function writeAuthCookie(token: string, record: unknown): void {
  if (typeof document === "undefined") return;
  document.cookie = buildAuthCookie(
    JSON.stringify({ token, record: sanitizeAuthRecord(record) }),
    cookieMaxAgeFromToken(token)
  );
}

/** Clear panel and legacy auth cookies. */
export function clearAuthCookies(): void {
  if (typeof document === "undefined") return;
  document.cookie = buildAuthCookieExpired();
  document.cookie = buildLegacyAuthCookieExpired();
}

/**
 * Read auth cookie from document.cookie (client only).
 * Prefers hp_auth, falls back to legacy pb_auth and migrates when found.
 */
export function readAuthCookieValueClient(): string | null {
  const current = readRawCookie(AUTH_COOKIE);
  if (current) {
    try {
      const data = JSON.parse(current) as { token?: string; record?: unknown };
      if (data.token && recordHasExtraFields(data.record)) {
        writeAuthCookie(data.token, data.record);
        return readRawCookie(AUTH_COOKIE) ?? current;
      }
    } catch {
      // Fall through and serve the raw value.
    }
    return current;
  }

  const legacy = readRawCookie(LEGACY_AUTH_COOKIE);
  if (!legacy) return null;

  try {
    const data = JSON.parse(legacy) as { token?: string; record?: unknown };
    if (data.token) {
      writeAuthCookie(data.token, data.record);
    }
  } catch {
    // Keep serving legacy value even if migration write fails.
  }

  return legacy;
}

/** Server-side read: hp_auth first, then legacy pb_auth. */
export function readAuthCookieValueServer(
  getCookieFn: (name: string) => string | undefined
): string | null {
  return getCookieFn(AUTH_COOKIE) ?? getCookieFn(LEGACY_AUTH_COOKIE) ?? null;
}

export const readAuthCookieValue = createIsomorphicFn()
  .server((): string | null => readAuthCookieValueServer((name) => getCookie(name)))
  .client((): string | null => readAuthCookieValueClient());
