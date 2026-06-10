import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type Column,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import {
  Link,
  createFileRoute,
  redirect,
  useNavigate,
} from "@tanstack/react-router";
import { Button } from "@heroui/react";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchDashboardNodeTraffic,
  fetchDashboardNodes,
  fetchDashboardTraffic,
  fetchDashboardUsers,
  queryErrorMessage,
  queryKeys,
  REFRESH_MS,
  toTrafficRangeQuery,
} from "~/api/queries";
import { TrafficRangePicker } from "~/components/traffic-range-picker";
import {
  CopyButton,
  Dot,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import {
  formatBytes,
  formatBytesPerSecond,
  plural,
  relTime,
  relTimeFromISO,
} from "~/lib/format";
import {
  defaultLocalTrafficRange,
  type LocalDateRange,
} from "~/lib/traffic-range";

type Node = components["schemas"]["Node"];
type NodeRangeTraffic = NonNullable<
  components["schemas"]["PanelNodeTrafficResponse"]["by_node"]
>[number];
type PanelUser = components["schemas"]["PanelUser"];
type NodeTableRow = {
  node: Node;
  rxSpeed: number;
  rangeTraffic?: NodeRangeTraffic;
  rangeTotal: number;
  status: string;
  txSpeed: number;
};
type UserTableRow = {
  email: string;
  rx: number;
  status: string;
  tx: number;
  user: PanelUser;
};

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
  component: DashboardPage,
});

function DashboardPage() {
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const isAdmin = auth?.user.role === "admin";
  const [trafficRange, setTrafficRange] = useState<LocalDateRange | null>(null);
  const [now, setNow] = useState(() => Date.now());

  const queryEnabled = canQueryPanelApi();
  const trafficRangeQuery = trafficRange
    ? toTrafficRangeQuery(trafficRange)
    : null;
  const nodesQuery = useQuery({
    queryKey: queryKeys.dashboardNodes(),
    queryFn: fetchDashboardNodes,
    enabled: queryEnabled,
    refetchInterval: REFRESH_MS,
  });
  const usersQuery = useQuery({
    queryKey: queryKeys.dashboardUsers(),
    queryFn: fetchDashboardUsers,
    enabled: queryEnabled,
    refetchInterval: REFRESH_MS,
  });
  const trafficQuery = useQuery({
    queryKey: queryKeys.dashboardTraffic(trafficRangeQuery),
    queryFn: () => fetchDashboardTraffic(trafficRangeQuery!),
    enabled: queryEnabled && trafficRangeQuery !== null,
    refetchInterval: REFRESH_MS,
  });
  const nodeTrafficSummaryQuery = useQuery({
    queryKey: queryKeys.dashboardNodeTraffic(trafficRangeQuery),
    queryFn: () => fetchDashboardNodeTraffic(trafficRangeQuery!),
    enabled: queryEnabled && trafficRangeQuery !== null,
    refetchInterval: REFRESH_MS,
  });

  // Tick the clock so relative timestamps and rolling ranges stay current.
  useEffect(() => {
    const ensureDefaultRange = () => {
      setTrafficRange((current) => current ?? defaultLocalTrafficRange());
    };

    ensureDefaultRange();
    const id = setInterval(() => {
      setNow(Date.now());
      ensureDefaultRange();
    }, 5_000);
    return () => clearInterval(id);
  }, []);

  const nodes = nodesQuery.data ?? [];
  const users = usersQuery.data ?? [];
  const panelTraffic = trafficQuery.data ?? null;
  const nodeTrafficSummary = nodeTrafficSummaryQuery.data ?? null;
  const nodesLoading = nodesQuery.isPending;
  const usersLoading = usersQuery.isPending;
  const trafficLoading = trafficQuery.isPending;
  const nodeTrafficLoading =
    trafficRange === null || nodeTrafficSummaryQuery.isPending;
  const nodesError = nodesQuery.error ? queryErrorMessage(nodesQuery.error) : "";
  const usersError = usersQuery.error ? queryErrorMessage(usersQuery.error) : "";
  const trafficError = trafficQuery.error
    ? queryErrorMessage(trafficQuery.error)
    : "";
  const nodeTrafficError = nodeTrafficSummaryQuery.error
    ? queryErrorMessage(nodeTrafficSummaryQuery.error)
    : "";
  const queryErrors = [
    { key: "nodes", message: nodesError ? `Nodes: ${nodesError}` : "" },
    {
      key: "nodeTraffic",
      message: nodeTrafficError ? `Node traffic: ${nodeTrafficError}` : "",
    },
    { key: "users", message: usersError ? `Users: ${usersError}` : "" },
    {
      key: "traffic",
      message: trafficError ? `Traffic: ${trafficError}` : "",
    },
  ].filter((err) => err.message);
  const updatedAt =
    Math.max(
      nodesQuery.dataUpdatedAt,
      usersQuery.dataUpdatedAt,
      trafficQuery.dataUpdatedAt,
      nodeTrafficSummaryQuery.dataUpdatedAt,
    ) || null;
  const nodeTrafficById = useMemo(() => {
    const byId = new Map<string, NodeRangeTraffic>();
    for (const row of nodeTrafficSummary?.by_node ?? []) {
      const id = row.node?.id;
      if (id) byId.set(id, row);
    }
    return byId;
  }, [nodeTrafficSummary]);
  const enabledNodes = nodes.filter((n) => n.enabled);
  const healthyNodes = enabledNodes.filter((n) => n.health === "ok");
  const errorNodes = enabledNodes.filter((n) => n.health === "error");
  const activeUsers = users.filter((u) => u.status === "active");
  const healthyTone =
    nodesError || errorNodes.length > 0
      ? "error"
      : healthyNodes.length > 0
        ? "ok"
        : "idle";
  const totalTx = panelTraffic?.total?.tx ?? 0;
  const totalRx = panelTraffic?.total?.rx ?? 0;

  return (
    <div className="min-h-svh bg-(--background) text-(--foreground)">
      <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
        <div className="mx-auto flex h-12 max-w-7xl items-center justify-between px-4 sm:px-6">
          <div className="flex items-center gap-2">
            <span className="grid size-5 place-items-center rounded-[5px] bg-(--accent) text-[11px] font-bold text-(--accent-foreground)">
              H
            </span>
            <span className="text-[13px] font-semibold tracking-tight">
              Hysterical Panel
            </span>
          </div>
          <div className="flex items-center gap-3 text-xs text-(--muted)">
            {updatedAt !== null && (
              <span
                className="hidden tabular-nums sm:inline"
                title={new Date(updatedAt).toLocaleString()}
              >
                Updated {relTime(updatedAt, now)}
              </span>
            )}
            <span className="hidden h-3.5 w-px bg-(--border) sm:block" />
            {auth && <UserMenu auth={auth} />}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-5 sm:px-6">
        {queryErrors.map((error) => (
          <div
            key={error.key}
            className="mb-4 flex items-center gap-2 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            <Dot tone="error" />
            <span>{error.message}</span>
          </div>
        ))}

        {/* Summary rail: one connected strip, not free-floating metric cards. */}
        <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
          <Stat
            label="Nodes"
            loading={nodesLoading}
            value={nodesError ? "—" : nodes.length}
          >
            {nodesError ? (
              <span className="text-(--danger)">Unavailable</span>
            ) : (
              `${enabledNodes.length} enabled`
            )}
          </Stat>
          <Stat
            label="Healthy"
            loading={nodesLoading}
            value={nodesError ? "—" : healthyNodes.length}
            dot={<Dot tone={healthyTone} />}
          >
            {nodesError ? (
              <span className="text-(--danger)">Unavailable</span>
            ) : errorNodes.length > 0 ? (
              <span className="text-(--danger)">{errorNodes.length} down</span>
            ) : enabledNodes.length > 0 ? (
              `of ${enabledNodes.length} enabled`
            ) : null}
          </Stat>
          <Stat
            label="Users"
            loading={usersLoading}
            value={usersError ? "—" : users.length}
          >
            {usersError ? (
              <span className="text-(--danger)">Unavailable</span>
            ) : (
              `${activeUsers.length} active`
            )}
          </Stat>
          <Stat
            label="Traffic"
            loading={trafficLoading}
            value={trafficError ? "—" : formatBytes(totalTx + totalRx)}
            headerAction={
              trafficRange ? (
                <TrafficRangePicker value={trafficRange} onChange={setTrafficRange} />
              ) : (
                <div
                  className="h-8 w-32 shrink-0 rounded-(--radius) border border-(--border) bg-(--surface-secondary) animate-pulse"
                  aria-hidden
                />
              )
            }
          >
            {trafficError ? (
              <span className="text-(--danger)">Unavailable</span>
            ) : (
              <span className="font-mono">
                <span className="text-(--muted)">↑</span> {formatBytes(totalTx)}
                <span className="mx-1.5 opacity-40">·</span>
                <span className="text-(--muted)">↓</span> {formatBytes(totalRx)}
              </span>
            )}
          </Stat>
        </div>

        <Section
          title="Nodes"
          meta={
            !nodesLoading && !nodesError && nodes.length > 0
              ? `${nodes.length} ${plural(nodes.length, "node")} · ${enabledNodes.length} enabled`
              : undefined
          }
          action={
            isAdmin ? (
              <Button
                size="sm"
                variant="secondary"
                onPress={() => navigate({ to: "/nodes/new" })}
              >
                Add node
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
              rangeTrafficByNode={nodeTrafficById}
              rangeTrafficLoading={nodeTrafficLoading}
              rangeTrafficUnavailable={Boolean(nodeTrafficError)}
            />
          ) : nodesError ? (
            <PanelMessage>Couldn't load nodes.</PanelMessage>
          ) : (
            <Teaching
              title="No nodes yet"
              hint="Add a node's API URL and secret to start collecting traffic."
              action={
                isAdmin ? (
                  <Button
                    size="sm"
                    variant="primary"
                    onPress={() => navigate({ to: "/nodes/new" })}
                  >
                    Add your first node
                  </Button>
                ) : undefined
              }
            />
          )}
        </Section>

        <Section
          title="Users"
          meta={
            !usersLoading && !usersError && users.length > 0
              ? `${users.length} ${plural(users.length, "user")} · ${activeUsers.length} active`
              : undefined
          }
        >
          {usersLoading ? (
            <TableSkeleton />
          ) : users.length > 0 ? (
            <UsersTable users={users} />
          ) : usersError ? (
            <PanelMessage>Couldn't load users.</PanelMessage>
          ) : (
            <Teaching
              title="No users yet"
              hint="Create a user to issue a Hysteria auth key and track its traffic."
            />
          )}
        </Section>
      </main>
    </div>
  );
}

/* ── Layout primitives ─────────────────────────────────────────────────── */

function Stat({
  label,
  value,
  loading,
  dot,
  children,
  headerAction,
}: {
  label: string;
  value: ReactNode;
  loading: boolean;
  dot?: ReactNode;
  children?: ReactNode;
  headerAction?: ReactNode;
}) {
  return (
    <div className="flex-1 px-4 py-3">
      <div className="flex justify-between gap-2">
        <div className="shrink-0 text-[11px] font-medium uppercase tracking-wider text-(--muted)">
          {label}
        </div>
        {headerAction}
      </div>
      {loading ? (
        <StatSkeleton withDot={Boolean(dot)} wide={label === "Traffic"} />
      ) : (
        <>
          <div className="mt-0.5 flex items-baseline gap-2">
            <span className="whitespace-nowrap text-xl font-semibold tabular-nums">
              {value}
            </span>
            {dot}
          </div>
          {children && (
            <div className="mt-1 whitespace-nowrap text-xs tabular-nums text-(--muted)">
              {children}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function StatSkeleton({
  withDot,
  wide,
}: {
  withDot: boolean;
  wide: boolean;
}) {
  return (
    <div className="mt-1" aria-hidden>
      <div className="flex h-6 items-center gap-2">
        <div
          className={`h-5 animate-pulse rounded bg-(--surface-secondary) ${
            wide ? "w-20" : "w-9"
          }`}
        />
        {withDot && (
          <div className="size-2 animate-pulse rounded-full bg-(--surface-secondary)" />
        )}
      </div>
      <div
        className={`mt-1.5 h-3 animate-pulse rounded bg-(--surface-secondary) ${
          wide ? "w-28" : "w-16"
        }`}
      />
    </div>
  );
}

/* ── Tables ────────────────────────────────────────────────────────────── */

function SortableTh<TData>({
  column,
  children,
  align = "left",
  className = "",
}: {
  column: Column<TData, unknown>;
  children: ReactNode;
  align?: "left" | "right";
  className?: string;
}) {
  const sorted = column.getIsSorted();
  const ariaSort =
    sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : "none";

  return (
    <th
      aria-sort={ariaSort}
      className={`px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-(--muted) ${className}`}
    >
      <button
        type="button"
        onClick={() => column.toggleSorting(sorted === "asc")}
        className={`inline-flex items-center gap-1 rounded-sm uppercase transition-colors duration-150 hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
          align === "right" ? "ml-auto justify-end" : ""
        }`}
      >
        <span>{children}</span>
        <span
          aria-hidden
          className={`inline-block w-3 text-center font-mono text-[10px] ${
            sorted ? "text-(--foreground)" : "text-(--muted)"
          }`}
        >
          {sorted === "asc" ? "↑" : sorted === "desc" ? "↓" : "↕"}
        </span>
      </button>
    </th>
  );
}

function NodesTable({
  nodes,
  now,
  rangeTrafficByNode,
  rangeTrafficLoading,
  rangeTrafficUnavailable,
}: {
  nodes: Node[];
  now: number;
  rangeTrafficByNode: Map<string, NodeRangeTraffic>;
  rangeTrafficLoading: boolean;
  rangeTrafficUnavailable: boolean;
}) {
  const [sorting, setSorting] = useState<SortingState>([
    { id: "name", desc: false },
  ]);
  const rows = useMemo<NodeTableRow[]>(
    () =>
      nodes.map((node) => {
        const rangeTraffic = node.id
          ? rangeTrafficByNode.get(node.id)
          : undefined;
        return {
          node,
          rxSpeed: node.enabled ? (node.current_rx_speed ?? 0) : 0,
          rangeTotal: (rangeTraffic?.tx ?? 0) + (rangeTraffic?.rx ?? 0),
          rangeTraffic,
          status: nodeStatusSortValue(node),
          txSpeed: node.enabled ? (node.current_tx_speed ?? 0) : 0,
        };
      }),
    [nodes, rangeTrafficByNode],
  );
  const columns = useMemo<ColumnDef<NodeTableRow>[]>(
    () => [
      {
        accessorFn: (row) => row.node.name ?? "",
        id: "name",
        sortDescFirst: false,
      },
      {
        accessorFn: (row) => row.rangeTotal,
        id: "traffic",
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
    [],
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
          <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
            <SortableTh column={table.getColumn("name")!}>Name</SortableTh>
            <SortableTh
              column={table.getColumn("traffic")!}
              align="right"
              className="text-right"
            >
              Traffic
            </SortableTh>
            <SortableTh
              column={table.getColumn("txSpeed")!}
              align="right"
              className="text-right"
            >
              TX speed
            </SortableTh>
            <SortableTh
              column={table.getColumn("rxSpeed")!}
              align="right"
              className="text-right"
            >
              RX speed
            </SortableTh>
            <Th className="text-right">Last poll</Th>
            <SortableTh
              column={table.getColumn("status")!}
              align="right"
              className="text-right"
            >
              Status
            </SortableTh>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {table.getRowModel().rows.map((row) => {
            const { node, rangeTraffic, rxSpeed, txSpeed } = row.original;
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
                className={`transition-colors duration-150 hover:bg-(--surface-secondary) ${enabled ? "" : "opacity-60"}`}
              >
                <Td>
                  <div className="flex items-center gap-2.5">
                    <Dot tone={tone} title={enabled ? health : "disabled"} />
                    <Link
                      to="/nodes/$nodeId"
                      params={{ nodeId: node.id ?? "" }}
                      className="block max-w-[180px] truncate rounded-sm font-medium underline-offset-2 hover:text-(--accent) hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                    >
                      {node.name || "—"}
                    </Link>
                  </div>
                </Td>
                <Td className="whitespace-nowrap text-right">
                  <NodeRangeUsage
                    loading={rangeTrafficLoading}
                    unavailable={rangeTrafficUnavailable}
                    traffic={rangeTraffic}
                  />
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↑</span>{" "}
                  {formatBytesPerSecond(txSpeed)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↓</span>{" "}
                  {formatBytesPerSecond(rxSpeed)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                  {node.last_polled_at
                    ? relTimeFromISO(node.last_polled_at, now)
                    : "—"}
                </Td>
                <Td className="text-right">
                  <NodeState
                    enabled={enabled}
                    health={health}
                    lastError={node.last_error}
                  />
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

function NodeRangeUsage({
  loading,
  unavailable,
  traffic,
}: {
  loading: boolean;
  unavailable: boolean;
  traffic?: NodeRangeTraffic;
}) {
  if (loading) {
    return (
      <span
        className="ml-auto block h-3 w-14 animate-pulse rounded bg-(--surface-secondary)"
        aria-hidden
      />
    );
  }
  if (unavailable) {
    return <span className="font-mono text-xs text-(--muted)">—</span>;
  }

  const tx = traffic?.tx ?? 0;
  const rx = traffic?.rx ?? 0;
  return (
    <span
      className="font-mono text-xs tabular-nums"
      title={`TX ${formatBytes(tx)} · RX ${formatBytes(rx)}`}
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
    return <span className="text-xs text-(--muted)">Disabled</span>;
  }
  if (health === "error") {
    const msg = lastError || "Error";
    return (
      <span
        className="block max-w-[260px] truncate text-xs text-(--danger)"
        title={msg}
      >
        {msg}
      </span>
    );
  }
  if (health === "ok") {
    return <span className="text-xs text-(--muted)">Healthy</span>;
  }
  return <span className="text-xs text-(--muted)">Never polled</span>;
}

function UsersTable({ users }: { users: PanelUser[] }) {
  const [sorting, setSorting] = useState<SortingState>([
    { id: "email", desc: false },
  ]);
  const rows = useMemo<UserTableRow[]>(
    () =>
      users.map((user) => ({
        email: user.email ?? "",
        rx: user.used_rx ?? 0,
        status: user.status ?? "active",
        tx: user.used_tx ?? 0,
        user,
      })),
    [users],
  );
  const columns = useMemo<ColumnDef<UserTableRow>[]>(
    () => [
      {
        accessorKey: "email",
        id: "email",
        sortDescFirst: false,
      },
      {
        accessorKey: "tx",
        id: "tx",
        sortDescFirst: false,
      },
      {
        accessorKey: "rx",
        id: "rx",
        sortDescFirst: false,
      },
      {
        accessorKey: "status",
        id: "status",
        sortDescFirst: false,
      },
    ],
    [],
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
          <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
            <SortableTh column={table.getColumn("email")!}>Email</SortableTh>
            <Th>Auth key</Th>
            <SortableTh
              column={table.getColumn("tx")!}
              align="right"
              className="text-right"
            >
              TX
            </SortableTh>
            <SortableTh
              column={table.getColumn("rx")!}
              align="right"
              className="text-right"
            >
              RX
            </SortableTh>
            <SortableTh
              column={table.getColumn("status")!}
              align="right"
              className="text-right"
            >
              Status
            </SortableTh>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {table.getRowModel().rows.map((row) => {
            const { user } = row.original;
            const active = (user.status ?? "active") === "active";
            return (
              <tr
                key={user.id}
                className={`transition-colors duration-150 hover:bg-(--surface-secondary) ${active ? "" : "opacity-60"}`}
              >
                <Td>
                  <div className="flex items-center gap-2.5">
                    <Dot
                      tone={active ? "ok" : "idle"}
                      title={active ? "active" : "disabled"}
                    />
                    <Link
                      to="/users/$userId"
                      params={{ userId: user.id ?? "" }}
                      className="block max-w-[200px] truncate rounded-sm font-medium underline-offset-2 hover:text-(--accent) hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                    >
                      {user.email || "—"}
                    </Link>
                  </div>
                </Td>
                <Td>
                  <div className="group/key flex items-center gap-1.5">
                    <span className="block max-w-[200px] truncate font-mono text-xs text-(--muted)">
                      {user.auth_string || "—"}
                    </span>
                    {user.auth_string && (
                      <CopyButton value={user.auth_string} label="auth key" />
                    )}
                  </div>
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↑</span>{" "}
                  {formatBytes(user.used_tx ?? 0)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↓</span>{" "}
                  {formatBytes(user.used_rx ?? 0)}
                </Td>
                <Td className="text-right">
                  <span className="text-xs text-(--muted)">
                    {active ? "Active" : "Disabled"}
                  </span>
                </Td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
