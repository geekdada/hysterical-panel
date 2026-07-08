"use client";

import { useSyncExternalStore } from "react";
import { getLocalTimeZone } from "@internationalized/date";
import { rootRouteId, useRouteContext } from "@tanstack/react-router";
import {
  FALLBACK_TIME_ZONE,
  getTimezoneServerSnapshot,
  getTimezoneSnapshot,
  setTimezonePreference,
  subscribeTimezone,
} from "~/lib/timezone";
import { useMounted } from "~/lib/use-mounted";

/** Resolved IANA timezone (override, cookie, or browser), reactive to preference changes. */
export function useActiveTimeZone(): string {
  const cookieTz = useRouteContext({ from: rootRouteId, select: (c) => c.timeZone });
  const preference = useSyncExternalStore(
    subscribeTimezone,
    getTimezoneSnapshot,
    getTimezoneServerSnapshot
  );
  const mounted = useMounted();
  return preference ?? cookieTz ?? (mounted ? getLocalTimeZone() : FALLBACK_TIME_ZONE);
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
