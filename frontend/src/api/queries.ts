import { apiClient } from "./client";
import { fetchPanelConfig } from "./panel-config";
import type { components } from "./schema";
import {
  granularityForLocalRange,
  localRangeToUtcQuery,
  trafficPeriodUtcRange,
  type LocalDateRange,
  type TrafficPeriod,
} from "~/lib/traffic-range";

type ApiResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

type Node = components["schemas"]["Node"];
type NodeCreateRequest = components["schemas"]["NodeCreateRequest"];
type NodeLive = components["schemas"]["NodeLiveResponse"];
type NodeTest = components["schemas"]["NodeTestResponse"];
type NodeTrafficSummary =
  components["schemas"]["NodeTrafficSummaryResponse"];
type PanelConfig = components["schemas"]["PanelConfigResponse"];
type PanelNodeTraffic = components["schemas"]["PanelNodeTrafficResponse"];
type PanelTraffic = components["schemas"]["PanelTrafficResponse"];
type PanelUser = components["schemas"]["PanelUser"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type TrafficSummary = components["schemas"]["TrafficSummaryResponse"];
type UserLive = components["schemas"]["LiveResponse"];

export const REFRESH_MS = 20_000;

export class PanelApiError extends Error {
  constructor(
    message: string,
    readonly status?: number,
  ) {
    super(message);
    this.name = "PanelApiError";
  }
}

export type TrafficRangeQuery = {
  from: string;
  granularity: ReturnType<typeof granularityForLocalRange>;
  to: string;
};

export type NodeOverviewData = {
  node: Node | null;
  series: TrafficSeries | null;
  summary: NodeTrafficSummary | null;
};

export type UserOverviewData = {
  series: TrafficSeries | null;
  summary: TrafficSummary | null;
  user: PanelUser | null;
};

export const queryKeys = {
  all: ["panel"] as const,
  config: () => [...queryKeys.all, "config"] as const,
  dashboardBase: () => [...queryKeys.all, "dashboard"] as const,
  dashboardNodes: () => [...queryKeys.dashboardBase(), "nodes"] as const,
  dashboardNodeTraffic: (range: TrafficRangeQuery | null) =>
    [
      ...queryKeys.dashboardBase(),
      "nodes",
      "traffic",
      range?.from ?? "",
      range?.to ?? "",
    ] as const,
  dashboardTraffic: (period: TrafficPeriod) =>
    [...queryKeys.dashboardBase(), "traffic", period] as const,
  dashboardUsers: () => [...queryKeys.dashboardBase(), "users"] as const,
  nodeLive: (nodeId: string) =>
    [...queryKeys.all, "nodes", nodeId, "live"] as const,
  nodeOverview: (nodeId: string, range: TrafficRangeQuery | null) =>
    [
      ...queryKeys.all,
      "nodes",
      nodeId,
      "overview",
      range?.granularity ?? "",
      range?.from ?? "",
      range?.to ?? "",
    ] as const,
  userLive: (userId: string) =>
    [...queryKeys.all, "users", userId, "live"] as const,
  userOverview: (userId: string, range: TrafficRangeQuery | null) =>
    [
      ...queryKeys.all,
      "users",
      userId,
      "overview",
      range?.granularity ?? "",
      range?.from ?? "",
      range?.to ?? "",
    ] as const,
};

export function canQueryPanelApi(): boolean {
  return typeof window !== "undefined";
}

export function toTrafficRangeQuery(
  range: LocalDateRange,
): TrafficRangeQuery {
  const { from, to } = localRangeToUtcQuery(range);
  return {
    from,
    granularity: granularityForLocalRange(range),
    to,
  };
}

export async function fetchPanelConfigQuery(): Promise<PanelConfig> {
  return fetchPanelConfig();
}

export function fetchDashboardNodes(): Promise<Node[]> {
  return apiRequest<Node[]>(apiClient.GET("/api/panel/nodes"));
}

export function fetchDashboardNodeTraffic(
  range: TrafficRangeQuery,
): Promise<PanelNodeTraffic | null> {
  return apiRequest<PanelNodeTraffic | null>(
    apiClient.GET("/api/panel/nodes/traffic/summary", {
      params: { query: { from: range.from, to: range.to } },
    }),
  );
}

export function fetchDashboardUsers(): Promise<PanelUser[]> {
  return apiRequest<PanelUser[]>(apiClient.GET("/api/panel/users"));
}

export function fetchDashboardTraffic(
  period: TrafficPeriod,
): Promise<PanelTraffic | null> {
  const { from, to } = trafficPeriodUtcRange(period);
  return apiRequest<PanelTraffic | null>(
    apiClient.GET("/api/panel/traffic", {
      params: { query: { from, to } },
    }),
  );
}

export async function fetchNodeOverview(
  nodeId: string,
  range: TrafficRangeQuery,
): Promise<NodeOverviewData> {
  const [node, summary, series] = await Promise.all([
    apiRequest<Node | null>(
      apiClient.GET("/api/panel/nodes/{id}", {
        params: { path: { id: nodeId } },
      }),
    ),
    apiRequest<NodeTrafficSummary | null>(
      apiClient.GET("/api/panel/nodes/{id}/traffic/summary", {
        params: { path: { id: nodeId } },
      }),
    ),
    apiRequest<TrafficSeries | null>(
      apiClient.GET("/api/panel/nodes/{id}/traffic/series", {
        params: {
          path: { id: nodeId },
          query: {
            from: range.from,
            granularity: range.granularity,
            to: range.to,
          },
        },
      }),
    ),
  ]);

  return { node, series, summary };
}

export async function fetchUserOverview(
  userId: string,
  range: TrafficRangeQuery,
): Promise<UserOverviewData> {
  const [user, summary, series] = await Promise.all([
    apiRequest<PanelUser | null>(
      apiClient.GET("/api/panel/users/{id}", {
        params: { path: { id: userId } },
      }),
    ),
    apiRequest<TrafficSummary | null>(
      apiClient.GET("/api/panel/users/{id}/traffic/summary", {
        params: { path: { id: userId } },
      }),
    ),
    apiRequest<TrafficSeries | null>(
      apiClient.GET("/api/panel/users/{id}/traffic/series", {
        params: {
          path: { id: userId },
          query: {
            from: range.from,
            granularity: range.granularity,
            to: range.to,
          },
        },
      }),
    ),
  ]);

  return { series, summary, user };
}

export function fetchNodeLive(nodeId: string): Promise<NodeLive | null> {
  return apiRequest<NodeLive | null>(
    apiClient.GET("/api/panel/nodes/{id}/live", {
      params: { path: { id: nodeId } },
    }),
    "Couldn't reach the panel API.",
    "Network error while fetching streams.",
  );
}

export function fetchUserLive(userId: string): Promise<UserLive | null> {
  return apiRequest<UserLive | null>(
    apiClient.GET("/api/panel/users/{id}/live", {
      params: { path: { id: userId } },
    }),
    "Couldn't reach the panel API.",
    "Network error while fetching streams.",
  );
}

export function createNode(body: NodeCreateRequest): Promise<Node> {
  return apiRequest<Node>(
    apiClient.POST("/api/panel/nodes", { body }),
    "Couldn't create the node.",
    "Network error while creating the node.",
  );
}

export function testNode(nodeId: string): Promise<NodeTest> {
  return apiRequest<NodeTest>(
    apiClient.POST("/api/panel/nodes/{id}/test", {
      params: { path: { id: nodeId } },
    }),
    "Couldn't run the connectivity test.",
    "Network error while testing the node.",
  );
}

export function isNotFoundError(error: unknown): boolean {
  return error instanceof PanelApiError && error.status === 404;
}

export function queryErrorMessage(
  error: unknown,
  networkMessage = "Network error. Retrying on the next refresh.",
): string {
  if (error instanceof PanelApiError) {
    return error.message;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return networkMessage;
}

async function apiRequest<T>(
  request: Promise<unknown>,
  apiMessage = "Couldn't reach the panel API.",
  networkMessage = "Network error. Retrying on the next refresh.",
): Promise<T> {
  try {
    const result = (await request) as ApiResult<T>;
    if (result.response.status === 404) {
      throw new PanelApiError("Not found", 404);
    }
    if (result.error) {
      throw new PanelApiError(
        errorMessage(result.error) || apiMessage,
        result.response.status,
      );
    }
    return (result.data ?? null) as T;
  } catch (error) {
    if (error instanceof PanelApiError) throw error;
    throw new PanelApiError(networkMessage);
  }
}

function errorMessage(error: unknown): string {
  if (
    error &&
    typeof error === "object" &&
    "message" in error &&
    typeof (error as { message: unknown }).message === "string"
  ) {
    return (error as { message: string }).message;
  }
  return "";
}
