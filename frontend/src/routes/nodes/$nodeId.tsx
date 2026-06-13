import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchNodeLive,
  fetchNodeOverview,
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
  BackLink,
  CopyButton,
  Dot,
  PageShell,
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
  formatDuration,
  relTime,
  relTimeFromISO,
} from "~/lib/format";

type Node = components["schemas"]["Node"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type NodeTrafficSummary = components["schemas"]["NodeTrafficSummaryResponse"];
type NodeLive = components["schemas"]["NodeLiveResponse"];

export const Route = createFileRoute("/nodes/$nodeId")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: NodeDetailPage,
});

function NodeDetailPage() {
  const { nodeId } = Route.useParams();
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();

  const [trafficRange, setTrafficRange] = useState<LocalDateRange | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setTrafficRange(defaultLocalTrafficRange());
  }, []);

  const trafficQuery = trafficRange ? toTrafficRangeQuery(trafficRange) : null;
  const overviewQuery = useQuery({
    queryKey: queryKeys.nodeOverview(nodeId, trafficQuery),
    queryFn: () => fetchNodeOverview(nodeId, trafficQuery!),
    enabled: canQueryPanelApi() && trafficQuery !== null,
    refetchInterval: REFRESH_MS,
  });

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(id);
  }, []);

  const node = overviewQuery.data?.node ?? null;
  const summary = overviewQuery.data?.summary ?? null;
  const series = overviewQuery.data?.series ?? null;
  const loading = trafficQuery === null || overviewQuery.isPending;
  const notFound = isNotFoundError(overviewQuery.error);
  const error = overviewQuery.error && !notFound ? queryErrorMessage(overviewQuery.error) : "";
  const updatedAt = overviewQuery.dataUpdatedAt || null;
  const health = node?.health ?? "never";
  const enabled = node?.enabled ?? false;
  const tone = !enabled ? "idle" : health === "ok" ? "ok" : health === "error" ? "error" : "idle";

  return (
    <PageShell
      headerLeft={
        <div className="flex min-w-0 items-center gap-3">
          <BackLink />
          {loading && !node ? (
            <span className="h-3.5 w-32 animate-pulse rounded bg-(--surface-secondary)" />
          ) : (
            <div className="flex min-w-0 items-center gap-2">
              <Dot tone={tone} title={enabled ? health : "disabled"} />
              <span className="truncate text-[13px] font-semibold tracking-tight">
                {node?.name || "Node"}
              </span>
            </div>
          )}
        </div>
      }
      headerRight={
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
      }
    >
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
          title="Node not found"
          hint="It may have been deleted. Head back to the dashboard to see the current fleet."
          action={
            <Button size="sm" variant="secondary" onPress={() => navigate({ to: "/" })}>
              Back to dashboard
            </Button>
          }
        />
      ) : (
        <>
          <DetailRail node={node} loading={loading && !node} now={now} />

          <TrafficSection
            loading={loading && !series}
            trafficRange={trafficRange}
            onTrafficRangeChange={setTrafficRange}
            series={series}
            summary={summary}
          />

          <StreamsSection nodeId={nodeId} />
        </>
      )}
    </PageShell>
  );
}

/* ── Detail rail (config + health) ─────────────────────────────────────── */

function DetailRail({ node, loading, now }: { node: Node | null; loading: boolean; now: number }) {
  if (loading) {
    return (
      <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="flex-1 px-4 py-3">
            <div className="h-3 w-16 animate-pulse rounded bg-(--surface-secondary)" />
            <div className="mt-2 h-4 w-28 animate-pulse rounded bg-(--surface-secondary)" />
          </div>
        ))}
      </div>
    );
  }

  const enabled = node?.enabled ?? false;
  const health = node?.health ?? "never";
  const stateLabel = !enabled
    ? "Disabled"
    : health === "error"
      ? node?.last_error || "Error"
      : health === "ok"
        ? "Healthy"
        : "Never polled";
  const stateTone = !enabled ? "muted" : health === "error" ? "danger" : "muted";
  const txSpeed = enabled ? (node?.current_tx_speed ?? 0) : 0;
  const rxSpeed = enabled ? (node?.current_rx_speed ?? 0) : 0;

  return (
    <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
      <RailItem label="Endpoint" className="sm:flex-[2]">
        <div className="group/key flex items-center gap-1.5">
          <span className="block truncate font-mono text-[13px] text-(--foreground)">
            {node?.api_url || "—"}
          </span>
          {node?.api_url && <CopyButton value={node.api_url} label="endpoint" />}
        </div>
      </RailItem>
      <RailItem label="Poll interval">
        <span className="font-mono text-[13px] tabular-nums">
          {node?.poll_interval ? `${node.poll_interval}s` : "—"}
        </span>
      </RailItem>
      <RailItem label="TX speed">
        <span className="font-mono text-[13px] tabular-nums">
          <span className="text-(--muted)">↑</span> {formatBytesPerSecond(txSpeed)}
        </span>
      </RailItem>
      <RailItem label="RX speed">
        <span className="font-mono text-[13px] tabular-nums">
          <span className="text-(--muted)">↓</span> {formatBytesPerSecond(rxSpeed)}
        </span>
      </RailItem>
      <RailItem label="Last poll">
        <span className="font-mono text-[13px] tabular-nums">
          {node?.last_polled_at ? relTimeFromISO(node.last_polled_at, now) : "—"}
        </span>
      </RailItem>
      <RailItem label="State">
        <span
          className={`block truncate text-[13px] ${stateTone === "danger" ? "text-(--danger)" : "text-(--foreground)"}`}
          title={stateLabel}
        >
          {stateLabel}
        </span>
      </RailItem>
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
      <div className="mt-1">{children}</div>
    </div>
  );
}

/* ── Traffic details ───────────────────────────────────────────────────── */

function TrafficSection({
  loading,
  trafficRange,
  onTrafficRangeChange,
  series,
  summary,
}: {
  loading: boolean;
  trafficRange: LocalDateRange | null;
  onTrafficRangeChange: (range: LocalDateRange) => void;
  series: TrafficSeries | null;
  summary: NodeTrafficSummary | null;
}) {
  const points = series?.points ?? [];
  const granularity = trafficRange ? granularityForLocalRange(trafficRange) : "hourly";
  const totalTx = points.reduce((sum, p) => sum + (p.tx ?? 0), 0);
  const totalRx = points.reduce((sum, p) => sum + (p.rx ?? 0), 0);
  const byUser = (summary?.by_user ?? []).slice(0, 8);

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
          <TrafficChart points={points} granularity={granularity} idPrefix="node-traffic" />
        )}
      </div>

      {!loading && byUser.length > 0 && (
        <div className="overflow-x-auto border-t border-(--border)">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
                <Th>Top Users</Th>
                <Th className="text-right">TX</Th>
                <Th className="text-right">RX</Th>
                <Th className="text-right">Total</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--separator)">
              {byUser.map((u, i) => (
                <tr
                  key={u.user?.id || i}
                  className="transition-colors duration-150 hover:bg-(--surface-secondary)"
                >
                  <Td>
                    <span className="block max-w-[280px] truncate font-medium">
                      {u.user?.email || "unknown"}
                    </span>
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-(--muted)">↑</span> {formatBytes(u.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-(--muted)">↓</span> {formatBytes(u.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                    {formatBytes((u.tx ?? 0) + (u.rx ?? 0))}
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

/* ── Streams dump (on demand) ──────────────────────────────────────────── */

function StreamsSection({ nodeId }: { nodeId: string }) {
  const [now, setNow] = useState(() => Date.now());
  const liveQuery = useQuery({
    queryKey: queryKeys.nodeLive(nodeId),
    queryFn: () => fetchNodeLive(nodeId),
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
  const byUser = live?.by_user ?? [];
  const topDomains = (live?.top_domains ?? []).slice(0, 12);
  const byConnection = live?.by_connection ?? [];

  return (
    <Section
      title="Live streams"
      meta={
        live && !live.error ? (
          <span className="font-mono tabular-nums">
            {live.online_devices ?? 0} devices online · {live.active_streams ?? 0} streams
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
      ) : live?.error ? (
        <div className="px-4 py-3 text-[13px] text-(--danger)" title={live.error}>
          {live.error}
        </div>
      ) : !live ? (
        <Teaching
          title="No snapshot yet"
          hint="Fetch a live snapshot to see every active stream on this node, grouped by user."
        />
      ) : byUser.length === 0 ? (
        <Teaching
          title="No active streams"
          hint="Nobody is routing traffic through this node right now."
        />
      ) : (
        <div className="flex flex-col">
          {byUser.map((u, i) => (
            <UserStreams key={u.user?.id || `unknown-${i}`} group={u} now={now} />
          ))}

          {topDomains.length > 0 && <TopDomainsTable rows={topDomains} />}
          {byConnection.length > 0 && <ByConnectionTable rows={byConnection} />}
        </div>
      )}
    </Section>
  );
}

function UserStreams({
  group,
  now,
}: {
  group: NonNullable<NodeLive["by_user"]>[number];
  now: number;
}) {
  const streams = group.streams ?? [];
  return (
    <div className="border-t border-(--border) first:border-t-0">
      <div className="flex items-center justify-between gap-3 bg-(--surface-secondary) px-3 py-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone="ok" />
          <span className="truncate text-xs font-medium">{group.user?.email || "unknown"}</span>
        </div>
        <span className="shrink-0 font-mono text-xs tabular-nums text-(--muted)">
          {group.online_devices ?? 0} devices online · {streams.length} streams
        </span>
      </div>
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
    </div>
  );
}

function TopDomainsTable({ rows }: { rows: NonNullable<NodeLive["top_domains"]> }) {
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

function ByConnectionTable({ rows }: { rows: NonNullable<NodeLive["by_connection"]> }) {
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
