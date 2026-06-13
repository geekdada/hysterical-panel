import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { ChevronLeft } from "@gravity-ui/icons";
import { Button } from "@heroui/react";
import {
  clearAuth,
  deletePasskey,
  isPasskeySoftError,
  listPasskeys,
  registerPasskey,
  type Passkey,
} from "~/api/auth";
import { requireAdminOrSelf } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchPanelConfigQuery,
  fetchUserLive,
  fetchUserOverview,
  isNotFoundError,
  queryErrorMessage,
  queryKeys,
  REFRESH_MS,
  toTrafficRangeQuery,
} from "~/api/queries";
import { TrafficRangePicker } from "~/components/traffic-range-picker";
import { TrafficChart } from "~/components/traffic";
import {
  defaultLocalTrafficRange,
  granularityForLocalRange,
  type LocalDateRange,
} from "~/lib/traffic-range";
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
import { formatBytes, formatDuration, relTime, relTimeFromISO } from "~/lib/format";

type PanelUser = components["schemas"]["PanelUser"];
type TrafficSummary = components["schemas"]["TrafficSummaryResponse"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type UserLive = components["schemas"]["LiveResponse"];

export const Route = createFileRoute("/users/$userId")({
  beforeLoad: ({ context, params }) => requireAdminOrSelf(context.auth, params.userId),
  component: AccountDetailPage,
});

function AccountDetailPage() {
  const { userId } = Route.useParams();
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const isAdmin = auth?.user.role === "admin";

  const [trafficRange, setTrafficRange] = useState<LocalDateRange | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setTrafficRange(defaultLocalTrafficRange());
  }, []);

  const trafficQuery = trafficRange ? toTrafficRangeQuery(trafficRange) : null;
  const overviewQuery = useQuery({
    queryKey: queryKeys.userOverview(userId, trafficQuery),
    queryFn: () => fetchUserOverview(userId, trafficQuery!),
    enabled: canQueryPanelApi() && trafficQuery !== null,
    refetchInterval: REFRESH_MS,
  });

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(id);
  }, []);

  const user = overviewQuery.data?.user ?? null;
  const summary = overviewQuery.data?.summary ?? null;
  const series = overviewQuery.data?.series ?? null;
  const loading = trafficQuery === null || overviewQuery.isPending;
  const notFound = isNotFoundError(overviewQuery.error);
  const error = overviewQuery.error && !notFound ? queryErrorMessage(overviewQuery.error) : "";
  const updatedAt = overviewQuery.dataUpdatedAt || null;

  function handleLogout() {
    clearAuth();
    window.location.href = "/login";
  }

  return (
    <div className="min-h-svh bg-(--background) text-(--foreground)">
      <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
        <div className="mx-auto flex h-12 max-w-7xl items-center justify-between px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-2.5">
            {isAdmin ? (
              <Link
                to="/"
                aria-label="Back to dashboard"
                className="grid size-6 shrink-0 place-items-center rounded text-(--muted) transition-colors duration-150 hover:bg-(--surface-secondary) hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
              >
                <ChevronLeft className="size-4" aria-hidden />
              </Link>
            ) : (
              <span className="grid size-5 shrink-0 place-items-center rounded-[5px] bg-(--accent) text-[11px] font-bold text-(--accent-foreground)">
                H
              </span>
            )}
            {loading && !user ? (
              <span className="h-3.5 w-40 animate-pulse rounded bg-(--surface-secondary)" />
            ) : (
              <div className="flex min-w-0 items-center gap-2">
                <Dot
                  tone={(user?.status ?? "active") === "active" ? "ok" : "idle"}
                  title={user?.status ?? "active"}
                />
                <span className="truncate text-[13px] font-semibold tracking-tight">
                  {user?.email || "Account"}
                </span>
              </div>
            )}
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

        {notFound ? (
          <Teaching
            title="Account not found"
            hint="It may have been deleted or moved."
            action={
              isAdmin ? (
                <Button size="sm" variant="secondary" onPress={() => navigate({ to: "/" })}>
                  Back to dashboard
                </Button>
              ) : (
                <Button size="sm" variant="secondary" onPress={handleLogout}>
                  Sign out
                </Button>
              )
            }
          />
        ) : (
          <>
            <AccountRail user={user} loading={loading && !user} />

            <PasskeysSection
              userId={userId}
              isSelf={auth?.user.id === userId}
              canManage={Boolean(auth && (auth.user.role === "admin" || auth.user.id === userId))}
            />

            <TrafficSection
              loading={loading && !series}
              trafficRange={trafficRange}
              onTrafficRangeChange={setTrafficRange}
              series={series}
              summary={summary}
              isAdmin={isAdmin}
            />

            {isAdmin && <LiveSection userId={userId} />}
          </>
        )}
      </main>
    </div>
  );
}

function AccountRail({ user, loading }: { user: PanelUser | null; loading: boolean }) {
  if (loading) {
    return (
      <div className="overflow-hidden rounded-(--radius) border border-(--border) bg-(--surface)">
        <div className="grid divide-y divide-(--border) md:grid-cols-[minmax(0,1.5fr)_minmax(0,1.5fr)_8rem_9rem] md:divide-x md:divide-y-0">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="px-4 py-3">
              <div className="h-3 w-16 animate-pulse rounded bg-(--surface-secondary)" />
              <div className="mt-2 h-4 w-32 animate-pulse rounded bg-(--surface-secondary)" />
            </div>
          ))}
        </div>
        <div className="grid border-t border-(--border) divide-y divide-(--border) md:grid-cols-4 md:divide-x md:divide-y-0">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="px-4 py-3">
              <div className="h-3 w-16 animate-pulse rounded bg-(--surface-secondary)" />
              <div className="mt-2 h-4 w-24 animate-pulse rounded bg-(--surface-secondary)" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  const status = user?.status ?? "active";
  const active = status === "active";
  const usedTx = user?.used_tx ?? 0;
  const usedRx = user?.used_rx ?? 0;
  const quota = user?.quota_bytes ?? 0;

  return (
    <div className="overflow-hidden rounded-(--radius) border border-(--border) bg-(--surface)">
      <div className="grid divide-y divide-(--border) md:grid-cols-[minmax(0,1.5fr)_minmax(0,1.5fr)_8rem_9rem] md:divide-x md:divide-y-0">
        <RailItem label="Email">
          <span className="block truncate text-[13px]" title={user?.email || ""}>
            {user?.email || "—"}
          </span>
        </RailItem>
        <RailItem label="Auth key">
          <div className="group/key flex min-w-0 items-center gap-1.5">
            <span className="block min-w-0 truncate font-mono text-[13px] text-(--foreground)">
              {user?.auth_string || "—"}
            </span>
            {user?.auth_string && <CopyButton value={user.auth_string} label="auth key" />}
          </div>
        </RailItem>
        <RailItem label="Role">
          <span className="font-mono text-[13px]">{user?.role || "—"}</span>
        </RailItem>
        <RailItem label="Status">
          <span className="inline-flex min-w-0 items-center gap-2">
            <Dot tone={active ? "ok" : "idle"} title={status} />
            <span className="truncate text-[13px] capitalize">{status}</span>
          </span>
        </RailItem>
      </div>
      <div className="grid border-t border-(--border) divide-y divide-(--border) md:grid-cols-4 md:divide-x md:divide-y-0">
        <RailItem label="Used total">
          <span className="font-mono text-[15px] font-semibold tabular-nums">
            {formatBytes(usedTx + usedRx)}
          </span>
        </RailItem>
        <RailItem label="TX">
          <span className="font-mono text-[13px] tabular-nums">
            <span className="text-(--muted)">↑</span> {formatBytes(usedTx)}
          </span>
        </RailItem>
        <RailItem label="RX">
          <span className="font-mono text-[13px] tabular-nums">
            <span className="text-(--muted)">↓</span> {formatBytes(usedRx)}
          </span>
        </RailItem>
        <RailItem label="Quota">
          <span className="font-mono text-[13px] tabular-nums">
            {quota > 0 ? formatBytes(quota) : "No quota"}
          </span>
        </RailItem>
      </div>
    </div>
  );
}

function RailItem({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`min-w-0 flex-1 px-4 py-3 ${className}`}>
      <div className="text-[11px] font-medium uppercase tracking-wider text-(--muted)">{label}</div>
      <div className="mt-1 min-w-0">{children}</div>
    </div>
  );
}

function PasskeysSection({
  userId,
  isSelf,
  canManage,
}: {
  userId: string;
  isSelf: boolean;
  canManage: boolean;
}) {
  const queryClient = useQueryClient();
  const passkeysKey = ["panel", "users", userId, "passkeys"] as const;
  const configQuery = useQuery({
    queryKey: queryKeys.config(),
    queryFn: fetchPanelConfigQuery,
    enabled: canQueryPanelApi(),
    staleTime: Infinity,
  });
  const enabled = configQuery.data?.passkeys_enabled ?? false;
  const passkeysQuery = useQuery({
    queryKey: passkeysKey,
    queryFn: () => listPasskeys(userId),
    enabled: canQueryPanelApi() && enabled,
  });
  const addMutation = useMutation({
    mutationFn: () => registerPasskey(userId, "Passkey", false),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: passkeysKey });
    },
  });
  const deleteMutation = useMutation({
    mutationFn: ({ passkeyId }: { passkeyId: string }) => deletePasskey(userId, passkeyId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: passkeysKey });
    },
  });

  if (!enabled) return null;

  const rows = passkeysQuery.data ?? [];
  const loading = passkeysQuery.isPending;
  const addError =
    addMutation.error && !isPasskeySoftError(addMutation.error)
      ? queryErrorMessage(addMutation.error, "Couldn't add passkey.")
      : "";
  const deleteError = deleteMutation.error
    ? queryErrorMessage(deleteMutation.error, "Couldn't delete passkey.")
    : "";
  const error = passkeysQuery.error
    ? queryErrorMessage(passkeysQuery.error)
    : addError || deleteError;

  return (
    <Section
      title="Passkeys"
      meta={
        loading ? undefined : `${rows.length} ${rows.length === 1 ? "credential" : "credentials"}`
      }
      action={
        isSelf ? (
          <Button
            size="sm"
            variant="secondary"
            isDisabled={addMutation.isPending}
            onPress={() => addMutation.mutate()}
          >
            {addMutation.isPending ? "Adding..." : "Add passkey"}
          </Button>
        ) : undefined
      }
    >
      {error && (
        <div
          className="border-b border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
          role="alert"
        >
          {error}
        </div>
      )}
      {loading ? (
        <TableSkeleton rows={2} />
      ) : rows.length === 0 ? (
        <Teaching
          title="No passkeys"
          hint={
            isSelf
              ? "Add a passkey to sign in without typing your password."
              : "This account has not enrolled a passkey."
          }
        />
      ) : (
        <PasskeysTable
          rows={rows}
          canManage={canManage}
          deletingId={
            deleteMutation.isPending && deleteMutation.variables
              ? deleteMutation.variables.passkeyId
              : undefined
          }
          onDelete={(passkey) => {
            if (window.confirm(`Delete passkey "${passkey.name || "Passkey"}"?`)) {
              deleteMutation.mutate({ passkeyId: passkey.id });
            }
          }}
        />
      )}
    </Section>
  );
}

function PasskeysTable({
  rows,
  canManage,
  deletingId,
  onDelete,
}: {
  rows: Passkey[];
  canManage: boolean;
  deletingId?: string;
  onDelete: (passkey: Passkey) => void;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
            <Th>Name</Th>
            <Th>Transports</Th>
            <Th>Backup</Th>
            <Th className="text-right">Last used</Th>
            {canManage && <Th className="text-right">Action</Th>}
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {rows.map((passkey) => (
            <tr
              key={passkey.id}
              className="transition-colors duration-150 hover:bg-(--surface-secondary)"
            >
              <Td>
                <span className="font-medium">{passkey.name || "Passkey"}</span>
              </Td>
              <Td>
                <span className="font-mono text-xs text-(--muted)">
                  {passkey.transports?.length ? passkey.transports.join(", ") : "—"}
                </span>
              </Td>
              <Td>
                <span className="text-xs text-(--muted)">
                  {passkey.backup_state
                    ? "Synced"
                    : passkey.backup_eligible
                      ? "Eligible"
                      : "Device-bound"}
                </span>
              </Td>
              <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                {passkey.last_used_at ? relTimeFromISO(passkey.last_used_at, Date.now()) : "Never"}
              </Td>
              {canManage && (
                <Td className="text-right">
                  <Button
                    size="sm"
                    variant="secondary"
                    isDisabled={deletingId === passkey.id}
                    onPress={() => onDelete(passkey)}
                  >
                    {deletingId === passkey.id ? "Deleting..." : "Delete"}
                  </Button>
                </Td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function TrafficSection({
  loading,
  trafficRange,
  onTrafficRangeChange,
  series,
  summary,
  isAdmin,
}: {
  loading: boolean;
  trafficRange: LocalDateRange | null;
  onTrafficRangeChange: (range: LocalDateRange) => void;
  series: TrafficSeries | null;
  summary: TrafficSummary | null;
  isAdmin: boolean;
}) {
  const points = series?.points ?? [];
  const granularity = trafficRange ? granularityForLocalRange(trafficRange) : "hourly";
  const totalTx = points.reduce((sum, p) => sum + (p.tx ?? 0), 0);
  const totalRx = points.reduce((sum, p) => sum + (p.rx ?? 0), 0);
  const byNode = [...(summary?.by_node ?? [])]
    .sort((a, b) => (b.tx ?? 0) + (b.rx ?? 0) - ((a.tx ?? 0) + (a.rx ?? 0)))
    .slice(0, 8);

  return (
    <Section
      title="Traffic"
      meta={
        !loading ? (
          <span className="font-mono tabular-nums">
            ↑ {formatBytes(totalTx)} · ↓ {formatBytes(totalRx)}
          </span>
        ) : undefined
      }
      action={
        trafficRange ? (
          <TrafficRangePicker value={trafficRange} onChange={onTrafficRangeChange} />
        ) : (
          <div
            className="h-8 w-full shrink-0 rounded-(--radius) border border-(--border) bg-(--surface-secondary) animate-pulse sm:w-40"
            aria-hidden
          />
        )
      }
    >
      <div className="p-3 sm:p-4">
        {loading ? (
          <div className="h-[220px] animate-pulse rounded bg-(--surface-secondary)" />
        ) : points.length === 0 ? (
          <div className="grid h-[220px] place-items-center text-[13px] text-(--muted)">
            No traffic recorded in this window.
          </div>
        ) : (
          <TrafficChart points={points} granularity={granularity} idPrefix="account-traffic" />
        )}
      </div>

      {!loading && byNode.length > 0 && (
        <div className="overflow-x-auto border-t border-(--border)">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
                <Th>Top Nodes</Th>
                <Th className="text-right">TX</Th>
                <Th className="text-right">RX</Th>
                <Th className="text-right">Total</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--separator)">
              {byNode.map((n, i) => (
                <tr
                  key={n.node?.id || i}
                  className="transition-colors duration-150 hover:bg-(--surface-secondary)"
                >
                  <Td>
                    {isAdmin && n.node?.id ? (
                      <Link
                        to="/nodes/$nodeId"
                        params={{ nodeId: n.node.id }}
                        className="block max-w-[280px] truncate rounded-sm font-medium underline-offset-2 hover:text-(--accent) hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                      >
                        {n.node?.name || "unknown"}
                      </Link>
                    ) : (
                      <span className="block max-w-[280px] truncate font-medium">
                        {n.node?.name || "unknown"}
                      </span>
                    )}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-(--muted)">↑</span> {formatBytes(n.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-(--muted)">↓</span> {formatBytes(n.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                    {formatBytes((n.tx ?? 0) + (n.rx ?? 0))}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}

function LiveSection({ userId }: { userId: string }) {
  const [now, setNow] = useState(() => Date.now());
  const liveQuery = useQuery({
    queryKey: queryKeys.userLive(userId),
    queryFn: () => fetchUserLive(userId),
    enabled: false,
  });

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(id);
  }, []);

  const live = liveQuery.data ?? null;
  const loading = liveQuery.isFetching;
  const fetchedAt = liveQuery.dataUpdatedAt || null;
  const reqError = liveQuery.error
    ? queryErrorMessage(liveQuery.error, "Network error while fetching streams.")
    : "";
  const byNode = live?.by_node ?? [];
  const visibleByNode = byNode.filter((n) => n.error || (n.streams?.length ?? 0) > 0);
  const topDomains = (live?.top_domains ?? []).slice(0, 12);
  const byConnection = live?.by_connection ?? [];

  return (
    <Section
      title="Live streams"
      meta={
        live ? (
          <span className="font-mono tabular-nums">
            {live.online_devices ?? 0} online · {live.active_streams ?? 0} streams
          </span>
        ) : undefined
      }
      action={
        <div className="flex items-center gap-2.5">
          {fetchedAt !== null && (
            <span
              className="hidden text-xs tabular-nums text-(--muted) sm:inline"
              title={new Date(fetchedAt).toLocaleString()}
            >
              {relTime(fetchedAt, now)}
            </span>
          )}
          <Button
            size="sm"
            variant="secondary"
            onPress={() => {
              void liveQuery.refetch();
            }}
            isPending={loading}
          >
            {fetchedAt === null ? "Fetch streams" : "Refresh"}
          </Button>
        </div>
      }
    >
      {reqError ? (
        <PanelMessage>{reqError}</PanelMessage>
      ) : loading && !live ? (
        <TableSkeleton />
      ) : !live ? (
        <Teaching
          title="No snapshot yet"
          hint="Fetch a live snapshot to see this account's active streams across visible nodes."
        />
      ) : byNode.length === 0 ? (
        <Teaching
          title="No visible nodes"
          hint="This account has no enabled nodes available for live diagnostics."
        />
      ) : visibleByNode.length === 0 ? (
        <Teaching
          title="No active streams"
          hint="This account is not routing traffic through visible nodes right now."
        />
      ) : (
        <div className="flex flex-col">
          {visibleByNode.map((n, i) => (
            <NodeStreams key={n.node?.id || i} group={n} now={now} />
          ))}

          {topDomains.length > 0 && <TopDomainsTable rows={topDomains} />}
          {byConnection.length > 0 && <ByConnectionTable rows={byConnection} />}
        </div>
      )}
    </Section>
  );
}

function NodeStreams({
  group,
  now,
}: {
  group: NonNullable<UserLive["by_node"]>[number];
  now: number;
}) {
  const streams = group.streams ?? [];
  const hasError = Boolean(group.error);
  return (
    <div className="border-t border-(--border) first:border-t-0">
      <div className="flex items-center justify-between gap-3 bg-(--surface-secondary) px-3 py-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone={hasError ? "error" : streams.length > 0 ? "ok" : "idle"} />
          <span className="truncate text-xs font-medium">{group.node?.name || "unknown"}</span>
        </div>
        <span className="shrink-0 font-mono text-xs tabular-nums text-(--muted)">
          {group.online_devices ?? 0} online · {streams.length} streams
        </span>
      </div>
      {hasError ? (
        <div className="px-3 py-2 text-[13px] text-(--danger)" title={group.error}>
          {group.error}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-(--separator) text-left">
                <Th>Target</Th>
                <Th>State</Th>
                <Th className="text-right">TX</Th>
                <Th className="text-right">RX</Th>
                <Th className="text-right">Lifetime</Th>
                <Th className="text-right">Idle</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--separator)">
              {streams.map((s, i) => {
                const target = s.hooked_req_addr || s.req_addr || "—";
                return (
                  <tr
                    key={`${s.connection}-${s.stream}-${i}`}
                    className="transition-colors duration-150 hover:bg-(--surface-secondary)"
                  >
                    <Td>
                      <span
                        className="block max-w-[320px] truncate font-mono text-xs text-(--foreground)"
                        title={target}
                      >
                        {target}
                      </span>
                    </Td>
                    <Td className="whitespace-nowrap font-mono text-xs text-(--muted)">
                      {s.state || "—"}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      {formatBytes(s.tx ?? 0)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      {formatBytes(s.rx ?? 0)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                      {formatDuration(s.lifetime_sec ?? -1)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                      {formatDuration(s.idle_sec ?? -1)}
                    </Td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function TopDomainsTable({ rows }: { rows: NonNullable<UserLive["top_domains"]> }) {
  return (
    <div className="bg-(--surface)">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-y border-(--border) bg-(--surface-secondary) text-left">
              <Th>Top domains</Th>
              <Th>ASN</Th>
              <Th>Country</Th>
              <Th className="text-right">Streams</Th>
              <Th className="text-right">Total</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--separator)">
            {rows.map((d, i) => {
              const domain = d.domain || "—";
              const meta = d.ip_meta;
              const countryTitle = meta?.country_name || meta?.country_code || "";
              return (
                <tr key={(d.domain || "") + i} className="hover:bg-(--surface-secondary)">
                  <Td>
                    {meta?.ipinfo_url ? (
                      <a
                        href={meta.ipinfo_url}
                        target="_blank"
                        rel="noreferrer"
                        className="block max-w-[260px] truncate font-mono text-xs text-(--foreground) underline decoration-(--border) underline-offset-2 transition-colors duration-150 hover:text-(--accent)"
                        title={`Open ${meta.ip || domain} on ipinfo.io`}
                      >
                        {domain}
                      </a>
                    ) : (
                      <span
                        className="block max-w-[260px] truncate font-mono text-xs"
                        title={domain}
                      >
                        {domain}
                      </span>
                    )}
                  </Td>
                  <Td className="whitespace-nowrap font-mono text-xs text-(--muted)">
                    {meta?.asn || "—"}
                  </Td>
                  <Td className="whitespace-nowrap text-xs text-(--muted)">
                    {meta?.country_code ? (
                      <span title={countryTitle}>
                        <span className="font-mono text-(--foreground)">{meta.country_code}</span>
                        {meta.country_name && (
                          <span className="ml-1 hidden max-w-[140px] truncate align-bottom sm:inline-block">
                            {meta.country_name}
                          </span>
                        )}
                      </span>
                    ) : (
                      "—"
                    )}
                  </Td>
                  <Td className="text-right font-mono text-xs tabular-nums text-(--muted)">
                    {d.streams ?? 0}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    {formatBytes((d.tx ?? 0) + (d.rx ?? 0))}
                  </Td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ByConnectionTable({ rows }: { rows: NonNullable<UserLive["by_connection"]> }) {
  return (
    <div className="bg-(--surface)">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-y border-(--border) bg-(--surface-secondary) text-left">
              <Th>Device</Th>
              <Th>Top domain</Th>
              <Th className="text-right">Streams</Th>
              <Th className="text-right">Total</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--separator)">
            {rows.map((c, i) => (
              <tr key={`${c.connection}-${i}`} className="hover:bg-(--surface-secondary)">
                <Td className="whitespace-nowrap font-mono text-xs tabular-nums text-(--muted)">
                  #{c.connection ?? 0}
                </Td>
                <Td>
                  <span
                    className="block max-w-[200px] truncate font-mono text-xs"
                    title={c.top_domain || ""}
                  >
                    {c.top_domain || "—"}
                  </span>
                </Td>
                <Td className="text-right font-mono text-xs tabular-nums text-(--muted)">
                  {c.stream_count ?? 0}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  {formatBytes((c.tx ?? 0) + (c.rx ?? 0))}
                </Td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
