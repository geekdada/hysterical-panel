import { createRouter } from "@tanstack/react-router";
import { setupRouterSsrQueryIntegration } from "@tanstack/react-router-ssr-query";
import { createPanelQueryClient } from "./api/query-provider";
import { routeTree } from "./routeTree.gen";

export function getRouter() {
  const queryClient = createPanelQueryClient();
  const router = createRouter({
    routeTree,
    scrollRestoration: true,
    context: {
      auth: null, // overwritten by the root beforeLoad on every load
      queryClient,
      timeZone: null,
    },
  });

  setupRouterSsrQueryIntegration({ router, queryClient });

  return router;
}
