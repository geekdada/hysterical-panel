import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import { clearAuth } from "~/api/auth";
import { apiClient } from "~/api/client";
import type { components } from "~/api/schema";

type Node = components["schemas"]["Node"];
type PanelUser = components["schemas"]["PanelUser"];

const REFRESH_MS = 20_000;

export const Route = createFileRoute("/")({
  beforeLoad: ({ context }) => {
    if (!context.auth) {
      throw redirect({ to: "/login" });
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
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());

  const fetchData = useCallback(async () => {
    try {
      const [nodesRes, usersRes] = await Promise.all([
        apiClient.GET("/api/panel/nodes"),
        apiClient.GET("/api/panel/users"),
      ]);
      if (nodesRes.error || usersRes.error) {
        setError("Couldn't reach the panel API.");
        return;
      }
      setNodes(nodesRes.data ?? []);
      setUsers(usersRes.data ?? []);
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

  function handleLogout() {
    clearAuth();
    window.location.href = "/login";
  }

  const enabledNodes = nodes.filter((n) => n.enabled);
  const healthyNodes = enabledNodes.filter((n) => n.health === "ok");
  const errorNodes = enabledNodes.filter((n) => n.health === "error");
  const activeUsers = users.filter((u) => u.status === "active");
  const totalTx = activeUsers.reduce((s, u) => s + (u.used_tx ?? 0), 0);
  const totalRx = activeUsers.reduce((s, u) => s + (u.used_rx ?? 0), 0);

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
            <span className="hidden max-w-[180px] truncate sm:inline">
              {auth?.user.email}
            </span>
            <Button variant="ghost" size="sm" onPress={handleLogout}>
              Sign out
            </Button>
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
            <span className="text-xs tabular-nums text-(--muted)">
              {enabledNodes.length} enabled
            </span>
          </Stat>
          <Stat
            label="Healthy"
            loading={loading}
            value={healthyNodes.length}
            dot={
              <Dot tone={errorNodes.length > 0 ? "error" : healthyNodes.length > 0 ? "ok" : "idle"} />
            }
          >
            {errorNodes.length > 0 && (
              <span className="text-xs tabular-nums text-(--danger)">
                {errorNodes.length} down
              </span>
            )}
          </Stat>
          <Stat label="Users" loading={loading} value={users.length}>
            <span className="text-xs tabular-nums text-(--muted)">
              {activeUsers.length} active
            </span>
          </Stat>
          <Stat label="Traffic" loading={loading} value={formatBytes(totalTx + totalRx)}>
            <span className="font-mono text-xs tabular-nums text-(--muted)">
              ↑ {formatBytes(totalTx)} · ↓ {formatBytes(totalRx)}
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

function Section({
  title,
  meta,
  action,
  children,
}: {
  title: string;
  meta?: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="mt-6">
      <div className="mb-2 flex min-h-7 items-center justify-between gap-3 px-0.5">
        <h2 className="text-[13px] font-semibold text-(--foreground)">{title}</h2>
        <div className="flex items-center gap-3">
          {meta && <span className="text-xs tabular-nums text-(--muted)">{meta}</span>}
          {action}
        </div>
      </div>
      <div className="overflow-hidden rounded-(--radius) border border-(--border) bg-(--surface)">
        {children}
      </div>
    </section>
  );
}

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
        <div className="mt-0.5 flex items-baseline gap-2">
          <span className="text-xl font-semibold tabular-nums">{value}</span>
          {dot}
          {children}
        </div>
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
                    <span className="block max-w-[180px] truncate font-medium">
                      {node.name || "—"}
                    </span>
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
                    <span className="block max-w-[200px] truncate font-medium">
                      {user.email || "—"}
                    </span>
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

function Th({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return (
    <th
      className={`px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-(--muted) ${className}`}
    >
      {children}
    </th>
  );
}

function Td({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return <td className={`px-3 py-2 align-middle ${className}`}>{children}</td>;
}

/* ── State / feedback ──────────────────────────────────────────────────── */

function Dot({ tone, title }: { tone: "ok" | "error" | "warn" | "idle"; title?: string }) {
  const fill = {
    ok: "bg-(--success)",
    error: "bg-(--danger)",
    warn: "bg-(--warning)",
    idle: "border border-(--muted) bg-transparent",
  }[tone];
  return (
    <span
      className={`inline-block size-2 shrink-0 rounded-full ${fill}`}
      title={title}
      aria-label={title}
    />
  );
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard unavailable (insecure context); nothing actionable to do.
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      title={copied ? "Copied" : `Copy ${label}`}
      aria-label={copied ? "Copied" : `Copy ${label}`}
      className={`inline-grid size-5 shrink-0 place-items-center rounded transition-[opacity,color] duration-150 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
        copied
          ? "text-(--success) opacity-100"
          : "text-(--muted) opacity-0 hover:text-(--foreground) group-hover/key:opacity-100"
      }`}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
    </button>
  );
}

function Teaching({
  title,
  hint,
  action,
}: {
  title: string;
  hint: string;
  action?: ReactNode;
}) {
  return (
    <div className="px-6 py-12 text-center">
      <p className="text-[13px] font-medium text-(--foreground)">{title}</p>
      <p className="mx-auto mt-1 max-w-sm text-xs text-(--muted)">{hint}</p>
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}

function PanelMessage({ children }: { children: ReactNode }) {
  return (
    <div className="px-6 py-10 text-center text-[13px] text-(--muted)">{children}</div>
  );
}

function TableSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="divide-y divide-(--separator)">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 px-3 py-[0.6875rem]">
          <span className="size-2 shrink-0 animate-pulse rounded-full bg-(--surface-secondary)" />
          <span className="h-3 w-28 animate-pulse rounded bg-(--surface-secondary)" />
          <span className="hidden h-3 w-44 animate-pulse rounded bg-(--surface-secondary) sm:block" />
          <span className="ml-auto h-3 w-16 animate-pulse rounded bg-(--surface-secondary)" />
        </div>
      ))}
    </div>
  );
}

/* ── Icons ─────────────────────────────────────────────────────────────── */

function CopyIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V5a2 2 0 0 1 2-2h10" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}

/* ── Formatting ────────────────────────────────────────────────────────── */

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  return `${val.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function relTime(fromMs: number, nowMs: number): string {
  const s = Math.max(0, Math.round((nowMs - fromMs) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

function relTimeFromISO(iso: string, nowMs: number): string {
  const t = Date.parse(iso);
  return Number.isNaN(t) ? "—" : relTime(t, nowMs);
}

function plural(n: number, word: string): string {
  return n === 1 ? word : `${word}s`;
}
