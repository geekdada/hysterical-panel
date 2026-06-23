"use client";

import { useSyncExternalStore } from "react";
import { getLocalTimeZone } from "@internationalized/date";
import {
  getTimezoneServerSnapshot,
  getTimezoneSnapshot,
  setTimezonePreference,
  subscribeTimezone,
} from "~/lib/timezone";

/** Resolved IANA timezone (override or browser), reactive to preference changes. */
export function useActiveTimeZone(): string {
  const preference = useSyncExternalStore(
    subscribeTimezone,
    getTimezoneSnapshot,
    getTimezoneServerSnapshot
  );
  return preference ?? getLocalTimeZone();
}

/** [preference, setter] for the settings UI; preference is null when following system. */
export function useTimezonePreference(): [string | null, (next: string | null) => void] {
  const preference = useSyncExternalStore(
    subscribeTimezone,
    getTimezoneSnapshot,
    getTimezoneServerSnapshot
  );
  return [preference, setTimezonePreference];
}
