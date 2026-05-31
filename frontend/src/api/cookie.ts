export const AUTH_COOKIE = "pb_auth";

// Keep this <= the PocketBase token lifetime, otherwise the cookie advertises
// a session the API will reject with 401. See the plan's risk notes.
const MAX_AGE_SECONDS = 60 * 60 * 24 * 7; // 7 days

function secureAttr(): string {
  return typeof window !== "undefined" && window.location.protocol === "https:"
    ? "; Secure"
    : "";
}

/** Build the document.cookie / Set-Cookie string for the given value. */
export function buildAuthCookie(value: string): string {
  return (
    `${AUTH_COOKIE}=${encodeURIComponent(value)}` +
    `; Path=/` +
    `; Max-Age=${MAX_AGE_SECONDS}` +
    `; SameSite=Lax` +
    secureAttr()
  );
}

/** Expire the auth cookie immediately. */
export function buildAuthCookieExpired(): string {
  return `${AUTH_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax${secureAttr()}`;
}

/**
 * Read the auth cookie from document.cookie (client only).
 * Returns the decoded JSON string, or null when absent / not in a browser.
 */
export function readAuthCookieValueClient(): string | null {
  if (typeof document === "undefined") return null;
  const prefix = `${AUTH_COOKIE}=`;
  for (const part of document.cookie.split(/;\s*/)) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length));
    }
  }
  return null;
}
