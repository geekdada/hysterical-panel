import { apiClient } from "./client";
import { fetchPanelConfig } from "./panel-config";
import {
  getSessionRecoveryFailed,
  isSessionAuthError,
  shouldSuppressSessionError,
} from "./session";
import type { components } from "./schema";
import {
  granularityForLocalRange,
  localRangeToUtcQuery,
  type LocalDateRange,
} from "~/lib/traffic-range";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

type ApiResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

type DatabasePrune = components["schemas"]["DatabasePruneResponse"];
type DatabaseStats = components["schemas"]["DatabaseStatsResponse"];
type Node = components["schemas"]["Node"];
type NodeCreateRequest = components["schemas"]["NodeCreateRequest"];
type NodeLive = components["schemas"]["NodeLiveResponse"];
type NodeTest = components["schemas"]["NodeTestResponse"];
type NodeTrafficSummary = components["schemas"]["NodeTrafficSummaryResponse"];
type PanelConfig = components["schemas"]["PanelConfigResponse"];
type PanelNodeTraffic = components["schemas"]["PanelNodeTrafficResponse"];
type PanelTraffic = components["schemas"]["PanelTrafficResponse"];
type PanelUser = components["schemas"]["PanelUser"];
type UserCreateRequest = components["schemas"]["UserCreateRequest"];
type UserListResponse = components["schemas"]["UserListResponse"];
type UserStatsResponse = components["schemas"]["UserStatsResponse"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type TrafficSummary = components["schemas"]["TrafficSummaryResponse"];
type UserLive = components["schemas"]["LiveResponse"];
type Invitation = components["schemas"]["Invitation"];
type InvitationCreateRequest = components["schemas"]["InvitationCreateRequest"];
type AppSettings = components["schemas"]["SettingsResponse"];
type SettingsUpdateRequest = components["schemas"]["SettingsUpdateRequest"];
type ManagementAPIToken = components["schemas"]["ManagementAPITokenResponse"];

export type {
  Invitation,
  InvitationCreateRequest,
  AppSettings,
  SettingsUpdateRequest,
  ManagementAPIToken,
};

export const REFRESH_MS = 20_000;

export class PanelApiError extends Error {
  constructor(
    message: string,
    readonly status?: number
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

export type UsersListQuery = {
  page: number;
  per_page: number;
  search: string;
  sort: string;
};

export type AnalyticsOverviewData = {
  nodeTraffic: PanelNodeTraffic | null;
  series: TrafficSeries | null;
};

export type { UserListResponse, UserStatsResponse };

export const queryKeys = {
  all: ["panel"] as const,
  analyticsBase: () => [...queryKeys.all, "analytics"] as const,
  analyticsNodeTraffic: (period: string, range: TrafficRangeQuery | null) =>
    [
      ...queryKeys.analyticsBase(),
      "nodes",
      "traffic",
      period,
      range?.from ?? "",
      range?.to ?? "",
    ] as const,
  analyticsOverview: (range: TrafficRangeQuery | null) =>
    [
      ...queryKeys.analyticsBase(),
      "overview",
      range?.granularity ?? "",
      range?.from ?? "",
      range?.to ?? "",
    ] as const,
  config: () => [...queryKeys.all, "config"] as const,
  dashboardBase: () => [...queryKeys.all, "dashboard"] as const,
  dashboardNodes: () => [...queryKeys.dashboardBase(), "nodes"] as const,
  dashboardNodeTraffic: (range: TrafficRangeQuery | null) =>
    [...queryKeys.dashboardBase(), "nodes", "traffic", range?.from ?? "", range?.to ?? ""] as const,
  dashboardTraffic: (range: TrafficRangeQuery | null) =>
    [...queryKeys.dashboardBase(), "traffic", range?.from ?? "", range?.to ?? ""] as const,
  userStats: () => [...queryKeys.dashboardBase(), "users", "stats"] as const,
  usersList: (query: UsersListQuery) =>
    [
      ...queryKeys.all,
      "users",
      "list",
      query.page,
      query.per_page,
      query.search,
      query.sort,
    ] as const,
  databaseStats: () => [...queryKeys.all, "database", "stats"] as const,
  invitations: () => [...queryKeys.all, "invitations"] as const,
  settings: () => [...queryKeys.all, "settings"] as const,
  nodeLive: (nodeId: string) => [...queryKeys.all, "nodes", nodeId, "live"] as const,
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
  userLive: (userId: string) => [...queryKeys.all, "users", userId, "live"] as const,
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

export function toTrafficRangeQuery(range: LocalDateRange): TrafficRangeQuery {
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
  range: TrafficRangeQuery
): Promise<PanelNodeTraffic | null> {
  return apiRequest<PanelNodeTraffic | null>(
    apiClient.GET("/api/panel/nodes/traffic/summary", {
      params: { query: { from: range.from, to: range.to } },
    })
  );
}

export function fetchUserStats(): Promise<UserStatsResponse> {
  return apiRequest<UserStatsResponse>(apiClient.GET("/api/panel/users/stats"));
}

export function fetchUsersList(query: UsersListQuery): Promise<UserListResponse> {
  return apiRequest<UserListResponse>(
    apiClient.GET("/api/panel/users", {
      params: {
        query: {
          page: query.page,
          per_page: query.per_page,
          search: query.search || undefined,
          sort: query.sort || undefined,
        },
      },
    })
  );
}

export function createUser(body: UserCreateRequest): Promise<PanelUser> {
  return apiRequest<PanelUser>(
    apiClient.POST("/api/panel/users", { body }),
    m.error_user_create(),
    m.error_user_create_network()
  );
}

export function updateUserStatus(id: string, status: "active" | "disabled"): Promise<PanelUser> {
  return apiRequest<PanelUser>(
    apiClient.PATCH("/api/panel/users/{id}", {
      params: { path: { id } },
      body: { status },
    }),
    m.error_user_update(),
    m.error_user_update_network()
  );
}

export function resetUserAuthString(id: string): Promise<PanelUser> {
  return apiRequest<PanelUser>(
    apiClient.POST("/api/panel/users/{id}/reset-auth-string", {
      params: { path: { id } },
    }),
    m.error_user_reset_auth(),
    m.error_user_reset_auth_network()
  );
}

export function fetchDashboardTraffic(range: TrafficRangeQuery): Promise<PanelTraffic | null> {
  return apiRequest<PanelTraffic | null>(
    apiClient.GET("/api/panel/traffic", {
      params: { query: { from: range.from, to: range.to } },
    })
  );
}

export function fetchPanelTrafficSeries(range: TrafficRangeQuery): Promise<TrafficSeries | null> {
  return apiRequest<TrafficSeries | null>(
    apiClient.GET("/api/panel/traffic/series", {
      params: {
        query: {
          from: range.from,
          granularity: range.granularity,
          to: range.to,
        },
      },
    })
  );
}

export async function fetchAnalyticsOverview(
  range: TrafficRangeQuery
): Promise<AnalyticsOverviewData> {
  const [nodeTraffic, series] = await Promise.all([
    fetchDashboardNodeTraffic(range),
    fetchPanelTrafficSeries(range),
  ]);

  return { nodeTraffic, series };
}

export function fetchDatabaseStats(): Promise<DatabaseStats | null> {
  return apiRequest<DatabaseStats | null>(apiClient.GET("/api/panel/database/stats"));
}

export function pruneDatabaseTraffic(): Promise<DatabasePrune> {
  return apiRequest<DatabasePrune>(
    apiClient.POST("/api/panel/database/prune"),
    m.error_database_prune(),
    m.error_database_prune_network()
  );
}

export async function fetchNodeOverview(
  nodeId: string,
  range: TrafficRangeQuery
): Promise<NodeOverviewData> {
  const [node, summary, series] = await Promise.all([
    apiRequest<Node | null>(
      apiClient.GET("/api/panel/nodes/{id}", {
        params: { path: { id: nodeId } },
      })
    ),
    apiRequest<NodeTrafficSummary | null>(
      apiClient.GET("/api/panel/nodes/{id}/traffic/summary", {
        params: {
          path: { id: nodeId },
          query: { from: range.from, to: range.to },
        },
      })
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
      })
    ),
  ]);

  return { node, series, summary };
}

export async function fetchUserOverview(
  userId: string,
  range: TrafficRangeQuery
): Promise<UserOverviewData> {
  const [user, summary, series] = await Promise.all([
    apiRequest<PanelUser | null>(
      apiClient.GET("/api/panel/users/{id}", {
        params: { path: { id: userId } },
      })
    ),
    apiRequest<TrafficSummary | null>(
      apiClient.GET("/api/panel/users/{id}/traffic/summary", {
        params: {
          path: { id: userId },
          query: { from: range.from, to: range.to },
        },
      })
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
      })
    ),
  ]);

  return { series, summary, user };
}

export function fetchNodeLive(nodeId: string): Promise<NodeLive | null> {
  return apiRequest<NodeLive | null>(
    apiClient.GET("/api/panel/nodes/{id}/live", {
      params: { path: { id: nodeId } },
    }),
    m.error_api_unreachable(),
    m.error_streams_network()
  );
}

export function fetchUserLive(userId: string): Promise<UserLive | null> {
  return apiRequest<UserLive | null>(
    apiClient.GET("/api/panel/users/{id}/live", {
      params: { path: { id: userId } },
    }),
    m.error_api_unreachable(),
    m.error_streams_network()
  );
}

export function createNode(body: NodeCreateRequest): Promise<Node> {
  return apiRequest<Node>(
    apiClient.POST("/api/panel/nodes", { body }),
    m.error_node_create(),
    m.error_node_create_network()
  );
}

export function testNode(nodeId: string): Promise<NodeTest> {
  return apiRequest<NodeTest>(
    apiClient.POST("/api/panel/nodes/{id}/test", {
      params: { path: { id: nodeId } },
    }),
    m.error_node_test(),
    m.error_node_test_network()
  );
}

export function deleteNode(nodeId: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/nodes/{id}", {
      params: { path: { id: nodeId } },
    }),
    m.error_node_delete(),
    m.error_node_delete_network()
  );
}

export function fetchSettings(): Promise<AppSettings> {
  return apiRequest<AppSettings>(apiClient.GET("/api/panel/settings"), m.error_settings_load());
}

export function updateSettings(body: SettingsUpdateRequest): Promise<AppSettings> {
  return apiRequest<AppSettings>(
    apiClient.PATCH("/api/panel/settings", { body }),
    m.error_settings_update(),
    m.error_settings_update_network()
  );
}

export function rotateManagementApiToken(): Promise<ManagementAPIToken> {
  return apiRequest<ManagementAPIToken>(
    apiClient.POST("/api/panel/management-api/rotate"),
    m.error_mgmt_token_rotate(),
    m.error_mgmt_token_rotate_network()
  );
}

export function fetchInvitations(): Promise<Invitation[]> {
  return apiRequest<Invitation[]>(
    apiClient.GET("/api/panel/invitations"),
    m.error_invitations_load()
  );
}

export function createInvitation(body: InvitationCreateRequest): Promise<Invitation> {
  return apiRequest<Invitation>(
    apiClient.POST("/api/panel/invitations", { body }),
    m.error_invitation_create(),
    m.error_invitation_create_network()
  );
}

export function deleteInvitation(id: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/invitations/{id}", {
      params: { path: { id } },
    }),
    m.error_invitation_delete(),
    m.error_invitation_delete_network()
  );
}

export function isNotFoundError(error: unknown): boolean {
  return error instanceof PanelApiError && error.status === 404;
}

export function queryErrorMessage(
  error: unknown,
  networkMessage = m.error_network_default()
): string {
  if (shouldSuppressSessionError() && isSessionAuthError(error)) {
    return "";
  }
  if (getSessionRecoveryFailed() && isSessionAuthError(error)) {
    return "";
  }
  if (error instanceof PanelApiError) {
    return localizeApiError(error.message);
  }
  if (error instanceof Error && error.message) {
    return localizeApiError(error.message);
  }
  return networkMessage;
}

async function apiRequest<T>(
  request: Promise<unknown>,
  apiMessage = m.error_api_unreachable(),
  networkMessage = m.error_network_default()
): Promise<T> {
  try {
    const result = (await request) as ApiResult<T>;
    if (result.response.status === 404) {
      throw new PanelApiError(m.error_not_found(), 404);
    }
    if (result.error) {
      const raw = errorMessage(result.error) || apiMessage;
      throw new PanelApiError(localizeApiError(raw), result.response.status);
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
