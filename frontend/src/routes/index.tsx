import { useCallback, useEffect, useState, type ReactNode } from "react";
import { Link, createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import { apiClient } from "~/api/client";
import type { components } from "~/api/schema";
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
import { formatBytes, plural, relTime, relTimeFromISO } from "~/lib/format";

type Node = components["schemas"]["Node"];
type PanelUser = components["schemas"]["PanelUser"];
type TrafficToday = components["schemas"]["TrafficTodayResponse"];

const REFRESH_MS = 20_000;

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
  const [nodes, setNodes] = useState<Node[]>([]);
  const [users, setUsers] = useState<PanelUser[]>([]);
  const [trafficToday, setTrafficToday] = useState<TrafficToday | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());

  const fetchData = useCallback(async () => {
    try {
      const [nodesRes, usersRes, trafficTodayRes] = await Promise.all([
        apiClient.GET("/api/panel/nodes"),
        apiClient.GET("/api/panel/users"),
        apiClient.GET("/api/panel/traffic/today"),
      ]);
      if (nodesRes.error || usersRes.error || trafficTodayRes.error) {
        setError("Couldn't reach the panel API.");
        return;
      }
      setNodes(nodesRes.data ?? []);
      setUsers(usersRes.data ?? []);
      setTrafficToday(trafficTodayRes.data ?? null);
      setError("");
      setUpdatedAt(Date.now());
    } catch {
      setError("Network error. Retrying on the next refresh.");
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial load + silent background refresh, so the panel reads as live.
  useEffect(() => {
    fetchData();
    const id = setInterval(fetchData, REFRESH_MS);
    return () => clearInterval(id);
  }, [fetchData]);

  // Tick the clock so relative timestamps stay current between refreshes.
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(id);
  }, []);

  const enabledNodes = nodes.filter((n) => n.enabled);
  const healthyNodes = enabledNodes.filter((n) => n.health === "ok");
  const errorNodes = enabledNodes.filter((n) => n.health === "error");
  const activeUsers = users.filter((u) => u.status === "active");
  const totalTx = trafficToday?.total?.tx ?? 0;
  const totalRx = trafficToday?.total?.rx ?? 0;

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
        {error && (
          <div
            className="mb-4 flex items-center gap-2 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            <Dot tone="error" />
            <span>{error}</span>
          </div>
        )}

        {/* Summary rail: one connected strip, not free-floating metric cards. */}
        <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
          <Stat label="Nodes" loading={loading} value={nodes.length}>
            {enabledNodes.length} enabled
          </Stat>
          <Stat
            label="Healthy"
            loading={loading}
            value={healthyNodes.length}
            dot={
              <Dot tone={errorNodes.length > 0 ? "error" : healthyNodes.length > 0 ? "ok" : "idle"} />
            }
          >
            {errorNodes.length > 0 ? (
              <span className="text-(--danger)">{errorNodes.length} down</span>
            ) : enabledNodes.length > 0 ? (
              `of ${enabledNodes.length} enabled`
            ) : null}
          </Stat>
          <Stat label="Users" loading={loading} value={users.length}>
            {activeUsers.length} active
          </Stat>
          <Stat label="Traffic UTC Today" loading={loading} value={formatBytes(totalTx + totalRx)}>
            <span className="font-mono">
              <span className="text-(--muted)">↑</span> {formatBytes(totalTx)}
              <span className="mx-1.5 opacity-40">·</span>
              <span className="text-(--muted)">↓</span> {formatBytes(totalRx)}
            </span>
          </Stat>
        </div>

        <Section
          title="Nodes"
          meta={
            !loading && nodes.length > 0
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
          {loading ? (
            <TableSkeleton />
          ) : nodes.length > 0 ? (
            <NodesTable nodes={nodes} now={now} />
          ) : error ? (
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
            !loading && users.length > 0
              ? `${users.length} ${plural(users.length, "user")} · ${activeUsers.length} active`
              : undefined
          }
        >
          {loading ? (
            <TableSkeleton />
          ) : users.length > 0 ? (
            <UsersTable users={users} />
          ) : error ? (
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
}: {
  label: string;
  value: ReactNode;
  loading: boolean;
  dot?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="flex-1 px-4 py-3">
      <div className="text-[11px] font-medium uppercase tracking-wider text-(--muted)">
        {label}
      </div>
      {loading ? (
        <div className="mt-1.5 h-6 w-14 animate-pulse rounded bg-(--surface-secondary)" />
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

/* ── Tables ────────────────────────────────────────────────────────────── */

function NodesTable({ nodes, now }: { nodes: Node[]; now: number }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
            <Th>Node</Th>
            <Th>Endpoint</Th>
            <Th>Interval</Th>
            <Th className="text-right">Last poll</Th>
            <Th>State</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {nodes.map((node) => {
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
                <Td>
                  <span className="block max-w-[280px] truncate font-mono text-xs text-(--muted)">
                    {node.api_url || "—"}
                  </span>
                </Td>
                <Td className="whitespace-nowrap font-mono text-xs tabular-nums text-(--muted)">
                  {node.poll_interval ? `${node.poll_interval}s` : "—"}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                  {node.last_polled_at ? relTimeFromISO(node.last_polled_at, now) : "—"}
                </Td>
                <Td>
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
      <span className="block max-w-[260px] truncate text-xs text-(--danger)" title={msg}>
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
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
            <Th>User</Th>
            <Th>Auth key</Th>
            <Th className="text-right">TX</Th>
            <Th className="text-right">RX</Th>
            <Th className="text-right">Status</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {users.map((user) => {
            const active = (user.status ?? "active") === "active";
            return (
              <tr
                key={user.id}
                className={`transition-colors duration-150 hover:bg-(--surface-secondary) ${active ? "" : "opacity-60"}`}
              >
                <Td>
                  <div className="flex items-center gap-2.5">
                    <Dot tone={active ? "ok" : "idle"} title={active ? "active" : "disabled"} />
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
                  <span className="text-(--muted)">↑</span> {formatBytes(user.used_tx ?? 0)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↓</span> {formatBytes(user.used_rx ?? 0)}
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
