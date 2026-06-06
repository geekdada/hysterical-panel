import { useQuery } from "@tanstack/react-query";
import { resolvePanelOrigin } from "~/api/panel-config";
import {
  canQueryPanelApi,
  fetchPanelConfigQuery,
  queryKeys,
} from "~/api/queries";

const BOOTSTRAP_ORIGIN = (import.meta.env.VITE_API_BASE_URL || "").replace(
  /\/$/,
  "",
);

export function usePanelApiOrigin(): string {
  const { data } = useQuery({
    queryKey: queryKeys.config(),
    queryFn: fetchPanelConfigQuery,
    enabled: canQueryPanelApi(),
    staleTime: Infinity,
  });

  return data ? resolvePanelOrigin(data) : BOOTSTRAP_ORIGIN;
}
