import { QueryCache, QueryClient } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { errorMessage, hostFromBaseUrl, serverLog, statusFromError } from "~/lib/server-log";
import { resolveServerApiBaseUrl } from "./panel-config";

export function createPanelQueryClient() {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error, query) => {
        serverLog.error("ssr_query_failed", {
          key: query.queryKey,
          host: hostFromBaseUrl(resolveServerApiBaseUrl()),
          status: statusFromError(error),
          message: errorMessage(error),
        });
      },
    }),
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

export function PanelQueryDevtools() {
  return import.meta.env.DEV ? <ReactQueryDevtools initialIsOpen={false} /> : null;
}
