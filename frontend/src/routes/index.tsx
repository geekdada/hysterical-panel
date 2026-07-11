import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { Link, createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import type { components } from "~/api/schema";
import {
  dashboardNodeTrafficQueryOptions,
  dashboardNodesQueryOptions,
  dashboardTrafficQueryOptions,
  alertSummaryQueryOptions,
  queryErrorMessage,
  toTrafficRangeQuery,
  userStatsQueryOptions,
  type TrafficRangeQuery,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  Brand,
  Dot,
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  SeverityBadge,
  SortableTh,
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, formatBytesPerSecond, relTime, relTimeFromISO } from "~/lib/format";
import {
  defaultLocalTrafficRange,
  type LocalDateRange,
  type TrafficRangeShortcut,
  trafficShortcutRange,
} from "~/lib/traffic-range";
import { FALLBACK_TIME_ZONE } from "~/lib/timezone";
import { useActiveTimeZone } from "~/lib/use-timezone";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import { defaultUsersListSearch, type UsersListSearch } from "~/lib/users-list-search";
import * as m from "~/paraglide/messages.js";

type Node = components["schemas"]["Node"];
type NodeTodayTraffic = NonNullable<
  components["schemas"]["PanelNodeTrafficResponse"]["by_node"]
>[number];
type NodeTableRow = {
  node: Node;
  rxSpeed: number;
  status: string;
  todayTotal: number;
  todayTraffic?: NodeTodayTraffic;
  txSpeed: number;
};

type TrafficPeriod = "today" | "t-1" | "7d";

const TRAFFIC_PERIOD_LABELS: Record<TrafficPeriod, string> = {
  today: "T",
  "t-1": "T-1",
  "7d": "7d",
};

// Map the dashboard's compact period toggle onto the range-based traffic API.
const TRAFFIC_PERIOD_SHORTCUT: Record<TrafficPeriod, TrafficRangeShortcut> = {
  today: "today",
  "t-1": "yesterday",
  "7d": "last-7d",
};

function dashboardTrafficRangeQuery(period: TrafficPeriod, tz: string): TrafficRangeQuery {
  return toTrafficRangeQuery(trafficShortcutRange(TRAFFIC_PERIOD_SHORTCUT[period], tz), tz);
}

function dashboardNodeTrafficRangeQuery(tz: string): TrafficRangeQuery {
  return toTrafficRangeQuery(defaultLocalTrafficRange(tz), tz);
}

export const Route = createFileRoute("/")({
  beforeLoad: ({ context }) => {
    if (!context.auth) {
      throw redirect({ to: "/login" });
    }
    if (context.auth.user.role !== "admin") {
      throw redirect({
        to: "/users/$userId",
        params: { userId: context.auth.user.id },
      });
    }
  },
  loader: async ({ context }) => {
    markResponsePrivate();
    const tz = context.timeZone ?? FALLBACK_TIME_ZONE;
    await Promise.allSettled([
      context.queryClient.ensureQueryData(dashboardNodesQueryOptions()),
      context.queryClient.ensureQueryData(userStatsQueryOptions()),
      context.queryClient.ensureQueryData(
        dashboardTrafficQueryOptions(dashboardTrafficRangeQuery("today", tz))
      ),
      context.queryClient.ensureQueryData(
        dashboardNodeTrafficQueryOptions(dashboardNodeTrafficRangeQuery(tz))
      ),
      context.queryClient.ensureQueryData(alertSummaryQueryOptions()),
    ]);
  },
  component: DashboardPage,
});

function DashboardPage() {
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const isAdmin = auth?.user.role === "admin";
  const tz = useActiveTimeZone();
  const [trafficPeriod, setTrafficPeriod] = useState<TrafficPeriod>("today");
  const [nodeTrafficRange, setNodeTrafficRange] = useState<LocalDateRange>(() =>
    defaultLocalTrafficRange(tz)
  );
  const now = useHydratedNow();

  const nodeTrafficQuery = toTrafficRangeQuery(nodeTrafficRange, tz);
  const trafficRangeQuery = dashboardTrafficRangeQuery(trafficPeriod, tz);
  const nodesQuery = useQuery(dashboardNodesQueryOptions());
  const userStatsQuery = useQuery(userStatsQueryOptions());
  const trafficQuery = useQuery(dashboardTrafficQueryOptions(trafficRangeQuery));
  const nodeTrafficSummaryQuery = useQuery(dashboardNodeTrafficQueryOptions(nodeTrafficQuery));
  const alertSummaryQuery = useQuery(alertSummaryQueryOptions());

  // Keep the local "today" range current and re-seed it when the timezone
  // preference changes. The hydration-safe relative-time clock updates separately.
  useEffect(() => {
    const updateToday = () => {
      const today = defaultLocalTrafficRange(tz);
      setNodeTrafficRange((current) =>
        current && current.start.compare(today.start) === 0 && current.end.compare(today.end) === 0
          ? current
          : today
      );
    };

    updateToday();
    const id = setInterval(() => {
      updateToday();
    }, 5_000);
    return () => clearInterval(id);
  }, [tz]);

  const nodes = nodesQuery.data ?? [];
  const userStats = userStatsQuery.data ?? null;
  const panelTraffic = trafficQuery.data ?? null;
  const nodeTrafficSummary = nodeTrafficSummaryQuery.data ?? null;
  const nodesLoading = nodesQuery.isPending;
  const usersLoading = userStatsQuery.isPending;
  const trafficLoading = trafficQuery.isPending;
  const nodeTrafficLoading = nodeTrafficSummaryQuery.isPending;
  const nodesError = nodesQuery.error ? queryErrorMessage(nodesQuery.error) : "";
  const usersError = userStatsQuery.error ? queryErrorMessage(userStatsQuery.error) : "";
  const trafficError = trafficQuery.error ? queryErrorMessage(trafficQuery.error) : "";
  const nodeTrafficError = nodeTrafficSummaryQuery.error
    ? queryErrorMessage(nodeTrafficSummaryQuery.error)
    : "";
  const queryErrors = [
    {
      key: "nodes",
      message: nodesError ? m.error_prefix_nodes({ message: nodesError }) : "",
    },
    {
      key: "nodeTraffic",
      message: nodeTrafficError ? m.error_prefix_node_traffic({ message: nodeTrafficError }) : "",
    },
    {
      key: "users",
      message: usersError ? m.error_prefix_users({ message: usersError }) : "",
    },
    {
      key: "traffic",
      message: trafficError ? m.error_prefix_traffic({ message: trafficError }) : "",
    },
  ].filter((err) => err.message);
  const updatedAt =
    Math.max(
      nodesQuery.dataUpdatedAt,
      userStatsQuery.dataUpdatedAt,
      trafficQuery.dataUpdatedAt,
      nodeTrafficSummaryQuery.dataUpdatedAt,
      alertSummaryQuery.dataUpdatedAt
    ) || null;
  const nodeTrafficById = useMemo(() => {
    const byId = new Map<string, NodeTodayTraffic>();
    for (const row of nodeTrafficSummary?.by_node ?? []) {
      const id = row.node?.id;
      if (id) byId.set(id, row);
    }
    return byId;
  }, [nodeTrafficSummary]);
  const enabledNodes = nodes.filter((n) => n.enabled);
  const healthyNodes = enabledNodes.filter((n) => n.health === "ok");
  const errorNodes = enabledNodes.filter((n) => n.health === "error");
  const activeUsers = userStats?.active ?? 0;
  const healthyTone =
    nodesError || errorNodes.length > 0 ? "error" : healthyNodes.length > 0 ? "ok" : "idle";
  const hasCriticalAlerts = (alertSummaryQuery.data?.critical ?? 0) > 0;
  const totalTx = panelTraffic?.total?.tx ?? 0;
  const totalRx = panelTraffic?.total?.rx ?? 0;

  return (
    <PageShell
      headerLeft={<Brand />}
      headerRight={
        <div className="flex items-center gap-3 text-xs text-muted">
          {updatedAt !== null && (
            <span
              className="hidden tabular-nums sm:inline"
              title={new Date(updatedAt).toLocaleString(undefined, { timeZone: tz })}
            >
              {m.common_updated({ time: relTime(updatedAt, now) })}
            </span>
          )}
          <span className="hidden h-3.5 w-px bg-border sm:block" />
          {auth && <UserMenu auth={auth} />}
        </div>
      }
    >
      {queryErrors.map((error) => (
        <ErrorAlert key={error.key} message={error.message} icon className="mb-4" />
      ))}

      {(alertSummaryQuery.data?.total ?? 0) > 0 ? (
        <Link
          to="/settings/monitoring"
          className={`mb-4 flex items-center justify-between rounded-lg border px-3 py-2.5 no-underline ${
            hasCriticalAlerts
              ? "border-danger/40 bg-danger-soft text-danger-soft-foreground"
              : "border-warning/40 bg-warning-soft text-warning-soft-foreground"
          }`}
        >
          <span className="flex items-center gap-2 text-[13px] font-medium">
            <SeverityBadge
              severity={hasCriticalAlerts ? "critical" : "warning"}
              label={hasCriticalAlerts ? m.monitoring_critical() : m.monitoring_warning()}
            />
            {m.monitoring_active_summary({ count: String(alertSummaryQuery.data?.total ?? 0) })}
          </span>
          <span className="text-xs text-muted">{m.nav_monitoring()} →</span>
        </Link>
      ) : null}

      {/* Summary rail: one connected strip, not free-floating metric cards. */}
      <div className="flex flex-col divide-y divide-border rounded-lg border bg-surface sm:flex-row sm:divide-x sm:divide-y-0">
        <Stat label={m.nav_nodes()} loading={nodesLoading} value={nodesError ? "—" : nodes.length}>
          {nodesError ? (
            <span className="text-danger">{m.common_unavailable()}</span>
          ) : (
            m.common_enabled_count({ count: String(enabledNodes.length) })
          )}
        </Stat>
        <Stat
          label={m.nav_healthy()}
          loading={nodesLoading}
          value={nodesError ? "—" : healthyNodes.length}
          dot={<Dot tone={healthyTone} />}
        >
          {nodesError ? (
            <span className="text-danger">{m.common_unavailable()}</span>
          ) : errorNodes.length > 0 ? (
            <span className="text-danger">
              {m.common_down_count({ count: String(errorNodes.length) })}
            </span>
          ) : enabledNodes.length > 0 ? (
            m.common_of_enabled({ count: String(enabledNodes.length) })
          ) : null}
        </Stat>
        <Stat
          label={m.nav_users_label()}
          loading={usersLoading}
          value={usersError ? "—" : (userStats?.total ?? 0)}
          href="/users"
          linkSearch={defaultUsersListSearch()}
        >
          {usersError ? (
            <span className="text-danger">{m.common_unavailable()}</span>
          ) : (
            m.common_active_count({ count: String(activeUsers) })
          )}
        </Stat>
        <Stat
          label={m.nav_traffic()}
          loading={trafficLoading}
          value={trafficError ? "—" : formatBytes(totalTx + totalRx)}
          headerAction={<TrafficPeriodToggle value={trafficPeriod} onChange={setTrafficPeriod} />}
        >
          {trafficError ? (
            <span className="text-danger">{m.common_unavailable()}</span>
          ) : (
            <span className="font-mono">
              <span className="text-muted">↑</span> {formatBytes(totalTx)}
              <span className="mx-1.5 opacity-40">·</span>
              <span className="text-muted">↓</span> {formatBytes(totalRx)}
            </span>
          )}
        </Stat>
      </div>

      <Section
        title={m.nav_nodes()}
        meta={
          !nodesLoading && !nodesError && nodes.length > 0
            ? m.common_nodes_meta({
                count: String(nodes.length),
                enabled: String(enabledNodes.length),
              })
            : undefined
        }
        action={
          isAdmin ? (
            <Button size="sm" variant="secondary" onPress={() => navigate({ to: "/nodes/new" })}>
              {m.dashboard_add_node()}
            </Button>
          ) : undefined
        }
      >
        {nodesLoading ? (
          <TableSkeleton />
        ) : nodes.length > 0 ? (
          <NodesTable
            nodes={nodes}
            now={now}
            todayTrafficByNode={nodeTrafficById}
            todayTrafficLoading={nodeTrafficLoading}
            todayTrafficUnavailable={Boolean(nodeTrafficError)}
          />
        ) : nodesError ? (
          <PanelMessage>{m.dashboard_couldnt_load_nodes()}</PanelMessage>
        ) : (
          <Teaching
            title={m.dashboard_no_nodes_title()}
            hint={m.dashboard_no_nodes_hint()}
            action={
              isAdmin ? (
                <Button size="sm" variant="primary" onPress={() => navigate({ to: "/nodes/new" })}>
                  {m.dashboard_add_first_node()}
                </Button>
              ) : undefined
            }
          />
        )}
      </Section>
    </PageShell>
  );
}

/* ── Layout primitives ─────────────────────────────────────────────────── */

function TrafficPeriodToggle({
  value,
  onChange,
}: {
  value: TrafficPeriod;
  onChange: (p: TrafficPeriod) => void;
}) {
  const opts: TrafficPeriod[] = ["today", "t-1", "7d"];
  return (
    <div className="inline-flex shrink-0 rounded-lg border p-0.5">
      {opts.map((o) => (
        <button
          key={o}
          type="button"
          onClick={() => onChange(o)}
          className={`rounded-[calc(var(--radius)-2px)] px-1.5 py-0.5 text-[10px] font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus ${
            value === o
              ? "bg-surface-secondary text-foreground"
              : "text-muted hover:text-foreground"
          }`}
          aria-pressed={value === o}
        >
          {TRAFFIC_PERIOD_LABELS[o]}
        </button>
      ))}
    </div>
  );
}

function Stat({
  label,
  value,
  loading,
  dot,
  children,
  headerAction,
  href,
  linkSearch,
}: {
  label: string;
  value: ReactNode;
  loading: boolean;
  dot?: ReactNode;
  children?: ReactNode;
  headerAction?: ReactNode;
  href?: string;
  linkSearch?: UsersListSearch;
}) {
  const valueContent = (
    <span className="whitespace-nowrap text-xl font-semibold tabular-nums">{value}</span>
  );

  return (
    <div className="relative flex-1 px-4 py-3">
      {headerAction ? <div className="absolute top-3 right-4 z-10">{headerAction}</div> : null}
      <div className="text-[11px] font-medium uppercase tracking-wider text-muted">{label}</div>
      {loading ? (
        <StatSkeleton withDot={Boolean(dot)} wide={label === m.nav_traffic()} />
      ) : (
        <>
          <div className="mt-0.5 flex items-baseline gap-2">
            {href ? (
              <Link
                to={href}
                search={linkSearch}
                className="rounded-sm transition-colors duration-150 hover:text-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
              >
                {valueContent}
              </Link>
            ) : (
              valueContent
            )}
            {dot}
          </div>
          {children && (
            <div className="mt-1 whitespace-nowrap text-xs tabular-nums text-muted">{children}</div>
          )}
        </>
      )}
    </div>
  );
}

function StatSkeleton({ withDot, wide }: { withDot: boolean; wide: boolean }) {
  return (
    <div className="mt-1" aria-hidden>
      <div className="flex h-6 items-center gap-2">
        <div
          className={`h-5 animate-pulse rounded bg-surface-secondary ${wide ? "w-20" : "w-9"}`}
        />
        {withDot && <div className="size-2 animate-pulse rounded-full bg-surface-secondary" />}
      </div>
      <div
        className={`mt-1.5 h-3 animate-pulse rounded bg-surface-secondary ${
          wide ? "w-28" : "w-16"
        }`}
      />
    </div>
  );
}

/* ── Tables ────────────────────────────────────────────────────────────── */

function NodesTable({
  nodes,
  now,
  todayTrafficByNode,
  todayTrafficLoading,
  todayTrafficUnavailable,
}: {
  nodes: Node[];
  now: number | null;
  todayTrafficByNode: Map<string, NodeTodayTraffic>;
  todayTrafficLoading: boolean;
  todayTrafficUnavailable: boolean;
}) {
  const [sorting, setSorting] = useState<SortingState>([{ id: "name", desc: false }]);
  const rows = useMemo<NodeTableRow[]>(
    () =>
      nodes.map((node) => {
        const todayTraffic = node.id ? todayTrafficByNode.get(node.id) : undefined;
        return {
          node,
          rxSpeed: node.enabled ? (node.current_rx_speed ?? 0) : 0,
          status: nodeStatusSortValue(node),
          todayTotal: (todayTraffic?.tx ?? 0) + (todayTraffic?.rx ?? 0),
          todayTraffic,
          txSpeed: node.enabled ? (node.current_tx_speed ?? 0) : 0,
        };
      }),
    [nodes, todayTrafficByNode]
  );
  const columns = useMemo<ColumnDef<NodeTableRow>[]>(
    () => [
      {
        accessorFn: (row) => row.node.name ?? "",
        id: "name",
        sortDescFirst: false,
      },
      {
        accessorFn: (row) => row.todayTotal,
        id: "today",
        sortDescFirst: false,
      },
      {
        accessorKey: "txSpeed",
        id: "txSpeed",
        sortDescFirst: false,
      },
      {
        accessorKey: "rxSpeed",
        id: "rxSpeed",
        sortDescFirst: false,
      },
      {
        accessorFn: (row) => row.status,
        id: "status",
        sortDescFirst: false,
      },
    ],
    []
  );
  const table = useReactTable({
    columns,
    data: rows,
    enableMultiSort: false,
    enableSortingRemoval: false,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    onSortingChange: setSorting,
    state: { sorting },
  });

  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-border bg-surface-secondary text-left">
            <SortableTh column={table.getColumn("name")!}>{m.common_name()}</SortableTh>
            <SortableTh column={table.getColumn("today")!} align="right" className="text-right">
              {m.dashboard_today()}
            </SortableTh>
            <SortableTh column={table.getColumn("txSpeed")!} align="right" className="text-right">
              {m.dashboard_tx_speed()}
            </SortableTh>
            <SortableTh column={table.getColumn("rxSpeed")!} align="right" className="text-right">
              {m.dashboard_rx_speed()}
            </SortableTh>
            <Th className="text-right">{m.dashboard_last_poll()}</Th>
            <SortableTh column={table.getColumn("status")!} align="right" className="text-right">
              {m.common_status()}
            </SortableTh>
          </tr>
        </thead>
        <tbody className="divide-y divide-separator">
          {table.getRowModel().rows.map((row) => {
            const { node, rxSpeed, todayTraffic, txSpeed } = row.original;
            const enabled = node.enabled ?? false;
            const health = node.health ?? "never";
            const tone = !enabled
              ? "idle"
              : health === "ok"
                ? "ok"
                : health === "error"
                  ? "error"
                  : "idle";
            return (
              <tr
                key={node.id}
                className={`transition-colors duration-150 hover:bg-surface-secondary ${enabled ? "" : "opacity-60"}`}
              >
                <Td>
                  <div className="flex items-center gap-2.5">
                    <Dot tone={tone} title={enabled ? health : m.common_disabled()} />
                    <Link
                      to="/nodes/$nodeId"
                      params={{ nodeId: node.id ?? "" }}
                      className="block max-w-[180px] truncate rounded-sm font-medium underline-offset-2 hover:text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
                    >
                      {node.name || m.common_em_dash()}
                    </Link>
                  </div>
                </Td>
                <Td className="whitespace-nowrap text-right">
                  <NodeTodayUsage
                    loading={todayTrafficLoading}
                    unavailable={todayTrafficUnavailable}
                    traffic={todayTraffic}
                  />
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-muted">↑</span> {formatBytesPerSecond(txSpeed)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-muted">↓</span> {formatBytesPerSecond(rxSpeed)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
                  {node.last_polled_at
                    ? relTimeFromISO(node.last_polled_at, now)
                    : m.common_em_dash()}
                </Td>
                <Td className="text-right">
                  <NodeState enabled={enabled} health={health} lastError={node.last_error} />
                </Td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function nodeStatusSortValue(node: Node): string {
  if (!(node.enabled ?? false)) {
    return "disabled";
  }
  const health = node.health ?? "never";
  if (health === "ok") {
    return "healthy";
  }
  if (health === "error") {
    return "error";
  }
  return "never polled";
}

function NodeTodayUsage({
  loading,
  unavailable,
  traffic,
}: {
  loading: boolean;
  unavailable: boolean;
  traffic?: NodeTodayTraffic;
}) {
  if (loading) {
    return (
      <span
        className="ml-auto block h-3 w-14 animate-pulse rounded bg-surface-secondary"
        aria-hidden
      />
    );
  }
  if (unavailable) {
    return <span className="font-mono text-xs text-muted">{m.common_em_dash()}</span>;
  }

  const tx = traffic?.tx ?? 0;
  const rx = traffic?.rx ?? 0;
  return (
    <span
      className="font-mono text-xs tabular-nums"
      title={m.dashboard_node_traffic_title({ tx: formatBytes(tx), rx: formatBytes(rx) })}
    >
      {formatBytes(tx + rx)}
    </span>
  );
}

function NodeState({
  enabled,
  health,
  lastError,
}: {
  enabled: boolean;
  health: string;
  lastError?: string;
}) {
  if (!enabled) {
    return <span className="text-xs text-muted">{m.common_disabled()}</span>;
  }
  if (health === "error") {
    const msg = lastError || m.common_error();
    return (
      <span className="block max-w-[260px] truncate text-xs text-danger" title={msg}>
        {msg}
      </span>
    );
  }
  if (health === "ok") {
    return <span className="text-xs text-muted">{m.common_healthy()}</span>;
  }
  return <span className="text-xs text-muted">{m.common_never_polled()}</span>;
}
