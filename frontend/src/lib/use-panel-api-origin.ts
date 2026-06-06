import { useEffect, useState } from "react";
import { fetchPanelConfig, resolvePanelOrigin } from "~/api/panel-config";

const BOOTSTRAP_ORIGIN = (import.meta.env.VITE_API_BASE_URL || "").replace(
  /\/$/,
  "",
);

export function usePanelApiOrigin(): string {
  const [origin, setOrigin] = useState(BOOTSTRAP_ORIGIN);

  useEffect(() => {
    fetchPanelConfig().then((config) => {
      setOrigin(resolvePanelOrigin(config));
    });
  }, []);

  return origin;
}
