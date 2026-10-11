import { useQuery } from "@tanstack/react-query";
import { panelConfigQueryOptions } from "~/api/queries";
import * as m from "~/paraglide/messages.js";

// The brand follows PocketBase's Application Name. The bundled title covers
// the window before the config loads or when the request fails.
export function resolveAppName(appName: string | undefined): string {
  return appName?.trim() || m.app_title();
}

export function appNameInitial(appName: string): string {
  return (Array.from(appName)[0] ?? "").toUpperCase();
}

export function useAppName(): string {
  const { data } = useQuery(panelConfigQueryOptions());
  return resolveAppName(data?.app_name);
}
