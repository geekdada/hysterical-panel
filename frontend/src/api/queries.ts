import { queryOptions } from "@tanstack/react-query";
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
import type {
  AuthenticationResponseJSON,
  PublicKeyCredentialRequestOptionsJSON,
} from "@simplewebauthn/browser";

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
type NodeAPISecretReset = components["schemas"]["NodeAPISecretResetResponse"];
type NotificationChannel = components["schemas"]["NotificationChannel"];
type NotificationChannelCreateRequest = components["schemas"]["NotificationChannelCreateRequest"];
type NotificationChannelUpdateRequest = components["schemas"]["NotificationChannelUpdateRequest"];
type NotificationChannelTestResponse = components["schemas"]["NotificationChannelTestResponse"];
type NotificationChannelRevealResponse = components["schemas"]["NotificationChannelRevealResponse"];
type PasskeyOptionsResponse = components["schemas"]["PasskeyOptionsResponse"];
type NodeTrafficSummary = components["schemas"]["NodeTrafficSummaryResponse"];
type PanelConfig = components["schemas"]["PanelConfigResponse"];
type PanelNodeTraffic = components["schemas"]["PanelNodeTrafficResponse"];
type PanelTraffic = components["schemas"]["PanelTrafficResponse"];
type PanelUser = components["schemas"]["PanelUser"];
type UserDetail = components["schemas"]["UserDetail"];
type UserCreateRequest = components["schemas"]["UserCreateRequest"];
type UserUpdateRequest = components["schemas"]["UserUpdateRequest"];
type UserListResponse = components["schemas"]["UserListResponse"];
type UserStatsResponse = components["schemas"]["UserStatsResponse"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type TrafficSummary = components["schemas"]["TrafficSummaryResponse"];
type UserLive = components["schemas"]["LiveResponse"];
type Invitation = components["schemas"]["Invitation"];
type InvitationCreateRequest = components["schemas"]["InvitationCreateRequest"];
type IgnoredConnectionIP = components["schemas"]["IgnoredConnectionIP"];
type IgnoredConnectionIPCreateRequest = components["schemas"]["IgnoredConnectionIPCreateRequest"];
type AppSettings = components["schemas"]["SettingsResponse"];
type SettingsUpdateRequest = components["schemas"]["SettingsUpdateRequest"];
type ManagementAPIToken = components["schemas"]["ManagementAPITokenResponse"];
type Monitor = components["schemas"]["Monitor"];
type MonitorCreateRequest = components["schemas"]["MonitorCreateRequest"];
type MonitorUpdateRequest = components["schemas"]["MonitorUpdateRequest"];
type Alert = components["schemas"]["Alert"];
type AlertListResponse = components["schemas"]["AlertListResponse"];
type AlertSummaryResponse = components["schemas"]["AlertSummaryResponse"];
type SubscriptionType = components["schemas"]["SubscriptionType"];
type SubscriptionTypeCreateRequest = components["schemas"]["SubscriptionTypeCreateRequest"];
type SubscriptionTypeUpdateRequest = components["schemas"]["SubscriptionTypeUpdateRequest"];
type UserSubscription = components["schemas"]["UserSubscription"];
type EmailSendout = components["schemas"]["EmailSendout"];
type EmailSendoutDetail = components["schemas"]["EmailSendoutDetail"];
type EmailSendoutRecipient = components["schemas"]["EmailSendoutRecipient"];
type EmailSendoutCandidate = components["schemas"]["EmailSendoutRecipientCandidate"];
type EmailSendoutContext = components["schemas"]["EmailSendoutContextResponse"];
type EmailSendoutCreateRequest = components["schemas"]["EmailSendoutCreateRequest"];
type EmailSendoutResendResponse = components["schemas"]["EmailSendoutResendResponse"];
type RecipientStatus = NonNullable<EmailSendoutRecipient["status"]>;

export type {
  Invitation,
  InvitationCreateRequest,
  IgnoredConnectionIP,
  IgnoredConnectionIPCreateRequest,
  AppSettings,
  SettingsUpdateRequest,
  ManagementAPIToken,
  NotificationChannel,
  NotificationChannelCreateRequest,
  NotificationChannelUpdateRequest,
  NotificationChannelTestResponse,
  NotificationChannelRevealResponse,
  Monitor,
  MonitorCreateRequest,
  MonitorUpdateRequest,
  Alert,
  AlertListResponse,
  AlertSummaryResponse,
  SubscriptionType,
  SubscriptionTypeCreateRequest,
  SubscriptionTypeUpdateRequest,
  UserSubscription,
  EmailSendout,
  EmailSendoutDetail,
  EmailSendoutRecipient,
  EmailSendoutCandidate,
  EmailSendoutContext,
  EmailSendoutCreateRequest,
  EmailSendoutResendResponse,
  RecipientStatus,
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
  user: UserDetail | null;
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
  ignoredConnectionIPs: () => [...queryKeys.all, "ignored-connection-ips"] as const,
  notificationChannels: () => [...queryKeys.all, "notification-channels"] as const,
  emailSendouts: () => [...queryKeys.all, "email-sendouts"] as const,
  emailSendoutContext: () => [...queryKeys.emailSendouts(), "context"] as const,
  emailSendoutCandidates: (search: string) =>
    [...queryKeys.emailSendouts(), "candidates", search] as const,
  emailSendout: (id: string) => [...queryKeys.emailSendouts(), id] as const,
  emailSendoutRecipients: (id: string, status: RecipientStatus | "") =>
    [...queryKeys.emailSendouts(), id, "recipients", status] as const,
  monitors: () => [...queryKeys.all, "monitors"] as const,
  alertsBase: () => [...queryKeys.all, "alerts"] as const,
  alerts: (query?: AlertsQuery) =>
    [
      ...queryKeys.alertsBase(),
      "list",
      query?.page ?? 1,
      query?.per_page ?? 25,
      query?.status ?? "",
      query?.severity ?? "",
    ] as const,
  alertSummary: () => [...queryKeys.alertsBase(), "summary"] as const,
  nodeAlerts: (nodeId: string) => [...queryKeys.all, "nodes", nodeId, "alerts"] as const,
  settings: () => [...queryKeys.all, "settings"] as const,
  subscriptionTypes: () => [...queryKeys.all, "subscription-types"] as const,
  userSubscriptions: (userId: string) =>
    [...queryKeys.all, "users", userId, "subscriptions"] as const,
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
  // Loaders may prefill SSR data; component-only queries stay browser-only until converted.
  return typeof window !== "undefined";
}

export function toTrafficRangeQuery(range: LocalDateRange, tz: string): TrafficRangeQuery {
  const { from, to } = localRangeToUtcQuery(range, tz);
  return {
    from,
    granularity: granularityForLocalRange(range),
    to,
  };
}

export async function fetchPanelConfigQuery(): Promise<PanelConfig> {
  return fetchPanelConfig();
}

export function panelConfigQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.config(),
    queryFn: fetchPanelConfigQuery,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
}

export function fetchDashboardNodes(): Promise<Node[]> {
  return apiRequest<Node[]>(apiClient.GET("/api/panel/nodes"));
}

export function dashboardNodesQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.dashboardNodes(),
    queryFn: fetchDashboardNodes,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
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

export function dashboardNodeTrafficQueryOptions(range: TrafficRangeQuery) {
  return queryOptions({
    queryKey: queryKeys.dashboardNodeTraffic(range),
    queryFn: () => fetchDashboardNodeTraffic(range),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
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

export function userStatsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.userStats(),
    queryFn: fetchUserStats,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function usersListQueryOptions(query: UsersListQuery) {
  return queryOptions({
    queryKey: queryKeys.usersList(query),
    queryFn: () => fetchUsersList(query),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function createUser(body: UserCreateRequest): Promise<PanelUser> {
  return apiRequest<PanelUser>(
    apiClient.POST("/api/panel/users", { body }),
    m.error_user_create(),
    m.error_user_create_network()
  );
}

export function updateUserStatus(id: string, status: "active" | "disabled"): Promise<UserDetail> {
  return apiRequest<UserDetail>(
    apiClient.PATCH("/api/panel/users/{id}", {
      params: { path: { id } },
      body: { status },
    }),
    m.error_user_update(),
    m.error_user_update_network()
  );
}

export function updateUser(id: string, body: Partial<UserUpdateRequest>): Promise<UserDetail> {
  return apiRequest<UserDetail>(
    apiClient.PATCH("/api/panel/users/{id}", {
      params: { path: { id } },
      body,
    }),
    m.error_user_update(),
    m.error_user_update_network()
  );
}

export function fetchSubscriptionTypes(): Promise<SubscriptionType[]> {
  return apiRequest<SubscriptionType[]>(apiClient.GET("/api/panel/subscription-types"));
}

export function subscriptionTypesQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.subscriptionTypes(),
    queryFn: fetchSubscriptionTypes,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
}

export function createSubscriptionType(
  body: SubscriptionTypeCreateRequest
): Promise<SubscriptionType> {
  return apiRequest<SubscriptionType>(apiClient.POST("/api/panel/subscription-types", { body }));
}

export function updateSubscriptionType(
  id: string,
  body: SubscriptionTypeUpdateRequest
): Promise<SubscriptionType> {
  return apiRequest<SubscriptionType>(
    apiClient.PATCH("/api/panel/subscription-types/{id}", { params: { path: { id } }, body })
  );
}

export function deleteSubscriptionType(id: string): Promise<void> {
  return apiRequest<void>(
    apiClient.DELETE("/api/panel/subscription-types/{id}", { params: { path: { id } } })
  );
}

export function fetchUserSubscriptions(id: string): Promise<UserSubscription[]> {
  return apiRequest<UserSubscription[]>(
    apiClient.GET("/api/panel/users/{id}/subscriptions", { params: { path: { id } } })
  );
}

export function userSubscriptionsQueryOptions(id: string) {
  return queryOptions({
    queryKey: queryKeys.userSubscriptions(id),
    queryFn: () => fetchUserSubscriptions(id),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function grantSubscription(id: string, subscriptionType: string): Promise<UserSubscription> {
  return apiRequest<UserSubscription>(
    apiClient.POST("/api/panel/users/{id}/subscriptions", {
      params: { path: { id } },
      body: { subscription_type: subscriptionType },
    })
  );
}

export function topUpSubscription(
  id: string,
  subscriptionId: string,
  allowanceBytes: number
): Promise<UserSubscription> {
  return apiRequest<UserSubscription>(
    apiClient.POST("/api/panel/users/{id}/subscriptions/{subscriptionId}/top-up", {
      params: { path: { id, subscriptionId } },
      body: { allowance_bytes: allowanceBytes },
    })
  );
}

export function rescheduleSubscription(
  id: string,
  subscriptionId: string,
  startsAt: string
): Promise<UserSubscription[]> {
  return apiRequest<UserSubscription[]>(
    apiClient.POST("/api/panel/users/{id}/subscriptions/{subscriptionId}/reschedule", {
      params: { path: { id, subscriptionId } },
      body: { starts_at: startsAt },
    })
  );
}

export function terminateSubscription(id: string, subscriptionId: string): Promise<void> {
  return apiRequest<void>(
    apiClient.DELETE("/api/panel/users/{id}/subscriptions/{subscriptionId}", {
      params: { path: { id, subscriptionId } },
    })
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

export function dashboardTrafficQueryOptions(range: TrafficRangeQuery) {
  return queryOptions({
    queryKey: queryKeys.dashboardTraffic(range),
    queryFn: () => fetchDashboardTraffic(range),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function analyticsOverviewQueryOptions(range: TrafficRangeQuery) {
  return queryOptions({
    queryKey: queryKeys.analyticsOverview(range),
    queryFn: () => fetchAnalyticsOverview(range),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
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

export function databaseStatsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.databaseStats(),
    queryFn: fetchDatabaseStats,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
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

export function nodeOverviewQueryOptions(nodeId: string, range: TrafficRangeQuery) {
  return queryOptions({
    queryKey: queryKeys.nodeOverview(nodeId, range),
    queryFn: () => fetchNodeOverview(nodeId, range),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export async function fetchUserOverview(
  userId: string,
  range: TrafficRangeQuery
): Promise<UserOverviewData> {
  const [user, summary, series] = await Promise.all([
    apiRequest<UserDetail | null>(
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

export function userOverviewQueryOptions(userId: string, range: TrafficRangeQuery) {
  return queryOptions({
    queryKey: queryKeys.userOverview(userId, range),
    queryFn: () => fetchUserOverview(userId, range),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
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

export function resetNodeAPISecret(nodeId: string): Promise<NodeAPISecretReset> {
  return apiRequest<NodeAPISecretReset>(
    apiClient.POST("/api/panel/nodes/{id}/reset-api-secret", {
      params: { path: { id: nodeId } },
    }),
    m.error_node_reset_api_secret(),
    m.error_node_reset_api_secret_network()
  );
}

export function fetchSettings(): Promise<AppSettings> {
  return apiRequest<AppSettings>(apiClient.GET("/api/panel/settings"), m.error_settings_load());
}

export function settingsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
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

export function invitationsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.invitations(),
    queryFn: fetchInvitations,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
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

export function fetchIgnoredConnectionIPs(): Promise<IgnoredConnectionIP[]> {
  return apiRequest<IgnoredConnectionIP[]>(
    apiClient.GET("/api/panel/ignored-connection-ips"),
    m.error_ignored_ips_load()
  );
}

export function createIgnoredConnectionIP(
  body: IgnoredConnectionIPCreateRequest
): Promise<IgnoredConnectionIP> {
  return apiRequest<IgnoredConnectionIP>(
    apiClient.POST("/api/panel/ignored-connection-ips", { body }),
    m.error_ignored_ip_create(),
    m.error_ignored_ip_create_network()
  );
}

export function deleteIgnoredConnectionIP(id: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/ignored-connection-ips/{id}", {
      params: { path: { id } },
    }),
    m.error_ignored_ip_delete(),
    m.error_ignored_ip_delete_network()
  );
}

export function fetchNotificationChannels(): Promise<NotificationChannel[]> {
  return apiRequest<NotificationChannel[]>(
    apiClient.GET("/api/panel/notification-channels"),
    m.error_notification_channels_load()
  );
}

export function createNotificationChannel(
  body: NotificationChannelCreateRequest
): Promise<NotificationChannel> {
  return apiRequest<NotificationChannel>(
    apiClient.POST("/api/panel/notification-channels", { body }),
    m.error_notification_channel_save(),
    m.error_notification_channel_save_network()
  );
}

export function updateNotificationChannel(
  id: string,
  body: NotificationChannelUpdateRequest
): Promise<NotificationChannel> {
  return apiRequest<NotificationChannel>(
    apiClient.PATCH("/api/panel/notification-channels/{id}", {
      params: { path: { id } },
      body,
    }),
    m.error_notification_channel_save(),
    m.error_notification_channel_save_network()
  );
}

export function deleteNotificationChannel(id: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/notification-channels/{id}", {
      params: { path: { id } },
    }),
    m.error_notification_channel_delete(),
    m.error_notification_channel_delete_network()
  );
}

export function testNotificationChannel(id: string): Promise<NotificationChannelTestResponse> {
  return apiRequest<NotificationChannelTestResponse>(
    apiClient.POST("/api/panel/notification-channels/{id}/test", {
      params: { path: { id } },
    }),
    m.error_notification_channel_test(),
    m.error_notification_channel_test_network()
  );
}

export async function revealNotificationChannelURL(
  id: string
): Promise<NotificationChannelRevealResponse> {
  const challenge = await apiRequest<PasskeyOptionsResponse>(
    apiClient.POST("/api/panel/notification-channels/{id}/reveal/options", {
      params: { path: { id } },
    }),
    m.error_notification_channel_reveal()
  );
  const { startAuthentication } = await import("@simplewebauthn/browser");
  const credential = await startAuthentication({
    optionsJSON: challenge.options as unknown as PublicKeyCredentialRequestOptionsJSON,
  });
  return apiRequest<NotificationChannelRevealResponse>(
    apiClient.POST("/api/panel/notification-channels/{id}/reveal/finish", {
      params: { path: { id } },
      body: {
        challenge_id: challenge.challenge_id,
        credential: credential as unknown as AuthenticationResponseJSON & Record<string, unknown>,
      },
    }),
    m.error_notification_channel_reveal()
  );
}

export function fetchEmailSendoutContext(): Promise<EmailSendoutContext> {
  return apiRequest<EmailSendoutContext>(
    apiClient.GET("/api/panel/email-sendouts/context"),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendoutCandidates(search: string): Promise<EmailSendoutCandidate[]> {
  return apiRequest<EmailSendoutCandidate[]>(
    apiClient.GET("/api/panel/email-sendouts/eligible-recipients", {
      params: { query: { search } },
    }),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendouts(): Promise<EmailSendout[]> {
  return apiRequest<EmailSendout[]>(
    apiClient.GET("/api/panel/email-sendouts"),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendout(id: string): Promise<EmailSendoutDetail> {
  return apiRequest<EmailSendoutDetail>(
    apiClient.GET("/api/panel/email-sendouts/{id}", { params: { path: { id } } }),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendoutRecipients(
  id: string,
  status: RecipientStatus | ""
): Promise<EmailSendoutRecipient[]> {
  return apiRequest<EmailSendoutRecipient[]>(
    apiClient.GET("/api/panel/email-sendouts/{id}/recipients", {
      params: { path: { id }, query: status ? { status } : {} },
    }),
    m.error_email_sendouts_load()
  );
}

export function emailSendoutContextQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.emailSendoutContext(),
    queryFn: fetchEmailSendoutContext,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
}

export function emailSendoutsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.emailSendouts(),
    queryFn: fetchEmailSendouts,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
}

export function emailSendoutQueryOptions(id: string) {
  return queryOptions({
    queryKey: queryKeys.emailSendout(id),
    queryFn: () => fetchEmailSendout(id),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
}

export function emailSendoutRecipientsQueryOptions(id: string, status: RecipientStatus | "") {
  return queryOptions({
    queryKey: queryKeys.emailSendoutRecipients(id, status),
    queryFn: () => fetchEmailSendoutRecipients(id, status),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
  });
}

export function createEmailSendout(body: EmailSendoutCreateRequest): Promise<EmailSendout> {
  return apiRequest<EmailSendout>(
    apiClient.POST("/api/panel/email-sendouts", { body }),
    m.error_email_sendout_create(),
    m.error_email_sendout_create_network()
  );
}

export function cancelEmailSendout(id: string): Promise<EmailSendout> {
  return apiRequest<EmailSendout>(
    apiClient.POST("/api/panel/email-sendouts/{id}/cancel", { params: { path: { id } } }),
    m.error_email_sendout_cancel()
  );
}

export function resendEmailSendout(
  id: string,
  recipientIds: string[]
): Promise<EmailSendoutResendResponse> {
  return apiRequest<EmailSendoutResendResponse>(
    apiClient.POST("/api/panel/email-sendouts/{id}/resend", {
      params: { path: { id } },
      body: { recipient_ids: recipientIds },
    }),
    m.error_email_sendout_resend()
  );
}

export function fetchMonitors(): Promise<Monitor[]> {
  return apiRequest<Monitor[]>(apiClient.GET("/api/panel/monitors"));
}

export function monitorsQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.monitors(),
    queryFn: fetchMonitors,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function createMonitor(body: MonitorCreateRequest): Promise<Monitor> {
  return apiRequest<Monitor>(apiClient.POST("/api/panel/monitors", { body }));
}

export function updateMonitor(id: string, body: MonitorUpdateRequest): Promise<Monitor> {
  return apiRequest<Monitor>(
    apiClient.PATCH("/api/panel/monitors/{id}", { params: { path: { id } }, body })
  );
}

export function deleteMonitor(id: string): Promise<{ deleted: boolean }> {
  return apiRequest<{ deleted: boolean }>(
    apiClient.DELETE("/api/panel/monitors/{id}", { params: { path: { id } } })
  );
}

export type AlertsQuery = {
  page?: number;
  per_page?: 25 | 50 | 100;
  status?: "firing" | "resolved" | "cancelled" | "history";
  severity?: "warning" | "critical";
};

export function fetchAlerts(query: AlertsQuery = {}): Promise<AlertListResponse> {
  return apiRequest<AlertListResponse>(apiClient.GET("/api/panel/alerts", { params: { query } }));
}

export function alertsQueryOptions(query: AlertsQuery = {}) {
  return queryOptions({
    queryKey: queryKeys.alerts(query),
    queryFn: () => fetchAlerts(query),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function fetchAlertSummary(): Promise<AlertSummaryResponse> {
  return apiRequest<AlertSummaryResponse>(apiClient.GET("/api/panel/alerts/summary"));
}

export function alertSummaryQueryOptions() {
  return queryOptions({
    queryKey: queryKeys.alertSummary(),
    queryFn: fetchAlertSummary,
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
}

export function fetchNodeAlerts(nodeId: string): Promise<AlertListResponse> {
  return apiRequest<AlertListResponse>(
    apiClient.GET("/api/panel/nodes/{id}/alerts", {
      params: { path: { id: nodeId }, query: { status: "firing", per_page: 100 } },
    })
  );
}

export function nodeAlertsQueryOptions(nodeId: string) {
  return queryOptions({
    queryKey: queryKeys.nodeAlerts(nodeId),
    queryFn: () => fetchNodeAlerts(nodeId),
    enabled: canQueryPanelApi(),
    staleTime: REFRESH_MS,
    refetchInterval: REFRESH_MS,
  });
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
