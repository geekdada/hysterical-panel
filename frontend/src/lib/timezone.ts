import { getLocalTimeZone } from "@internationalized/date";

// User-overridable display/query timezone. Mirrors the theme-preference pattern
// (localStorage, module-level state) since the codebase keeps cross-page prefs as
// plain helpers rather than a React context — and a non-hook accessor is needed by
// callers outside React (e.g. setInterval bodies). Backed by an external store so
// React components can subscribe via useSyncExternalStore (see use-timezone.ts).

export const TIMEZONE_STORAGE_KEY = "hp:tz";

// null = "follow system" (no override, current behavior); otherwise an IANA id —
// a fixed-offset "Etc/GMT±N" zone or "UTC".
type TimezonePreference = string | null;

/** Sentinel Select item id for "follow system"; the stored preference is null. */
export const SYSTEM_TIMEZONE_ID = "system";

export type TimezoneOption = {
  /** Select item id: the SYSTEM_TIMEZONE_ID sentinel, or the stored IANA id. */
  id: string;
  /** Whole-hour UTC offset, or null for "follow system". */
  offset: number | null;
};

let preference: TimezonePreference = null;
let hydrated = false;
const listeners = new Set<() => void>();

/** Whether the runtime can resolve an IANA id; guards stale/garbage stored values. */
export function isValidTimeZone(id: string): boolean {
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: id });
    return true;
  } catch {
    return false;
  }
}

function readStored(): TimezonePreference {
  if (typeof localStorage === "undefined") return null;
  try {
    const value = localStorage.getItem(TIMEZONE_STORAGE_KEY);
    return value && isValidTimeZone(value) ? value : null;
  } catch {
    return null;
  }
}

// Read storage lazily on the first client snapshot so SSR stays "follow system"
// and useSyncExternalStore's server/client snapshots reconcile in a single pass.
function ensureHydrated(): void {
  if (hydrated || typeof window === "undefined") return;
  preference = readStored();
  hydrated = true;
}

export function getTimezonePreference(): TimezonePreference {
  ensureHydrated();
  return preference;
}

/** Resolved IANA timezone used for every query bound + datetime display. */
export function getActiveTimeZone(): string {
  return getTimezonePreference() ?? getLocalTimeZone();
}

export function setTimezonePreference(next: TimezonePreference): void {
  preference = next;
  hydrated = true;
  try {
    if (next) localStorage.setItem(TIMEZONE_STORAGE_KEY, next);
    else localStorage.removeItem(TIMEZONE_STORAGE_KEY);
  } catch {
    // Storage unavailable (private mode / disabled) — still apply for this session.
  }
  for (const listener of listeners) listener();
}

// --- external store plumbing (consumed by useSyncExternalStore) ---

export function subscribeTimezone(callback: () => void): () => void {
  listeners.add(callback);
  return () => listeners.delete(callback);
}

export function getTimezoneSnapshot(): TimezonePreference {
  return getTimezonePreference();
}

export function getTimezoneServerSnapshot(): TimezonePreference {
  return null;
}

// --- offset option table + IANA mapping ---

/** UTC+8 → "Etc/GMT-8" (the POSIX sign is inverted); UTC±0 → "UTC". */
export function offsetToIana(offsetHours: number): string {
  if (offsetHours === 0) return "UTC";
  const sign = offsetHours > 0 ? "-" : "+";
  return `Etc/GMT${sign}${Math.abs(offsetHours)}`;
}

/** "UTC+08:00" / "UTC-05:00" — generated, never translated. */
export function offsetLabel(offsetHours: number): string {
  const sign = offsetHours >= 0 ? "+" : "-";
  return `UTC${sign}${String(Math.abs(offsetHours)).padStart(2, "0")}:00`;
}

// "Follow system" plus whole-hour offsets from UTC+14 down to UTC-12. Half/quarter
// hour zones (e.g. UTC+05:30) aren't representable as Etc/GMT±N and are out of scope.
export const TIMEZONE_OPTIONS: TimezoneOption[] = [
  { id: SYSTEM_TIMEZONE_ID, offset: null },
  ...Array.from({ length: 27 }, (_, i) => {
    const offset = 14 - i;
    return { id: offsetToIana(offset), offset };
  }),
];
