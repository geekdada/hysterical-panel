import type { components } from "./schema";

export type PanelConfig = components["schemas"]["PanelConfigResponse"];

const BOOTSTRAP_BASE = (import.meta.env.VITE_API_BASE_URL || "").replace(/\/$/, "");

const EMPTY_CONFIG: PanelConfig = {
  api_url: "",
  frontend_url: "",
  passkeys_enabled: false,
  version: "",
};

declare const process:
  | {
      env?: {
        PANEL_SSR_API_BASE_URL?: string;
      };
    }
  | undefined;

let cached: PanelConfig | null = null;
let inflight: Promise<PanelConfig> | null = null;

function isServerRuntime(): boolean {
  return typeof window === "undefined";
}

export function resolveServerApiBaseUrl(): string {
  if (!isServerRuntime()) return "";
  const runtimeBase =
    typeof process !== "undefined" ? process.env?.PANEL_SSR_API_BASE_URL : undefined;
  return (runtimeBase || BOOTSTRAP_BASE).replace(/\/$/, "");
}

function resolveConfigFetchBase(): string {
  return isServerRuntime() ? resolveServerApiBaseUrl() : BOOTSTRAP_BASE;
}

export function fetchPanelConfig(): Promise<PanelConfig> {
  const server = isServerRuntime();
  if (!server && cached) return Promise.resolve(cached);
  if (!server && inflight) return inflight;

  const url = `${resolveConfigFetchBase()}/api/panel/config`;
  const request = fetch(url)
    .then(async (res) => {
      if (!res.ok) {
        throw new Error(`panel config failed (${res.status})`);
      }
      return (await res.json()) as PanelConfig;
    })
    .then((data) => {
      if (!server) cached = data;
      return data;
    })
    .catch(() => {
      return EMPTY_CONFIG;
    })
    .finally(() => {
      if (!server) inflight = null;
    });

  if (!server) inflight = request;
  return request;
}

export function resolveApiBaseUrl(config: PanelConfig): string {
  const fromConfig = (config.api_url || "").replace(/\/$/, "");
  return fromConfig || BOOTSTRAP_BASE;
}

export function resolvePanelOrigin(config: PanelConfig): string {
  const api = resolveApiBaseUrl(config);
  if (api) return api;
  if (typeof window !== "undefined") return window.location.origin;
  return "";
}
