import type { components } from "./schema";

export type PanelConfig = components["schemas"]["PanelConfigResponse"];

const BOOTSTRAP_BASE = (import.meta.env.VITE_API_BASE_URL || "").replace(
  /\/$/,
  "",
);

const EMPTY_CONFIG: PanelConfig = {
  api_url: "",
  frontend_url: "",
  passkeys_enabled: false,
  version: "",
};

let cached: PanelConfig | null = null;
let inflight: Promise<PanelConfig> | null = null;

export function fetchPanelConfig(): Promise<PanelConfig> {
  if (cached) return Promise.resolve(cached);
  if (inflight) return inflight;

  const url = `${BOOTSTRAP_BASE}/api/panel/config`;
  inflight = fetch(url)
    .then(async (res) => {
      if (!res.ok) {
        throw new Error(`panel config failed (${res.status})`);
      }
      return (await res.json()) as PanelConfig;
    })
    .then((data) => {
      cached = data;
      return data;
    })
    .catch(() => {
      cached = EMPTY_CONFIG;
      return EMPTY_CONFIG;
    })
    .finally(() => {
      inflight = null;
    });

  return inflight;
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
