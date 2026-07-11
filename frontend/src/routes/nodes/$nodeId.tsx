import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Modal } from "@heroui/react";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  deleteNode,
  fetchNodeLive,
  isNotFoundError,
  nodeOverviewQueryOptions,
  nodeAlertsQueryOptions,
  queryErrorMessage,
  queryKeys,
  resetNodeAPISecret,
  toTrafficRangeQuery,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { TrafficRangePicker } from "~/components/traffic-range-picker";
import { TrafficChart } from "~/components/traffic";
import {
  defaultLocalTrafficRange,
  granularityForLocalRange,
  type LocalDateRange,
} from "~/lib/traffic-range";
import { FALLBACK_TIME_ZONE } from "~/lib/timezone";
import {
  BrandLink,
  CopyButton,
  Dot,
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  SeverityBadge,
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from "~/components/ui";
import { SetBreadcrumbTitle } from "~/components/breadcrumbs";
import { UserMenu } from "~/components/user-menu";
import { cn } from "~/lib/cn";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import {
  formatBytes,
  formatBytesPerSecond,
  formatDuration,
  relTime,
  relTimeFromISO,
} from "~/lib/format";
import { useActiveTimeZone } from "~/lib/use-timezone";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

type Node = components["schemas"]["Node"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type NodeTrafficSummary = components["schemas"]["NodeTrafficSummaryResponse"];
type NodeLive = components["schemas"]["NodeLiveResponse"];
type NodeAPISecretReset = components["schemas"]["NodeAPISecretResetResponse"];
type Alert = components["schemas"]["Alert"];
type AlertItem = NonNullable<components["schemas"]["AlertListResponse"]["items"]>[number];

export const Route = createFileRoute("/nodes/$nodeId")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.node_fallback_title(),
    dynamic: true,
  }),
  loader: async ({ context, params }) => {
    markResponsePrivate();
    const tz = context.timeZone ?? FALLBACK_TIME_ZONE;
    const range = toTrafficRangeQuery(defaultLocalTrafficRange(tz), tz);
    await Promise.allSettled([
      context.queryClient.ensureQueryData(nodeOverviewQueryOptions(params.nodeId, range)),
      context.queryClient.ensureQueryData(nodeAlertsQueryOptions(params.nodeId)),
    ]);
  },
  component: NodeDetailPage,
});

function NodeDetailPage() {
  const { nodeId } = Route.useParams();
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const tz = useActiveTimeZone();

  const [trafficRange, setTrafficRange] = useState<LocalDateRange>(() =>
    defaultLocalTrafficRange(tz)
  );
  const now = useHydratedNow();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteStep, setDeleteStep] = useState<1 | 2>(1);
  const [resetOpen, setResetOpen] = useState(false);
  const [resetResult, setResetResult] = useState<NodeAPISecretReset | null>(null);

  useEffect(() => {
    setTrafficRange((current) => {
      const next = defaultLocalTrafficRange(tz);
      return current.start.compare(next.start) === 0 && current.end.compare(next.end) === 0
        ? current
        : next;
    });
  }, [tz]);

  const trafficQuery = toTrafficRangeQuery(trafficRange, tz);
  const overviewQuery = useQuery(nodeOverviewQueryOptions(nodeId, trafficQuery));
  const alertsQuery = useQuery(nodeAlertsQueryOptions(nodeId));

  const node = overviewQuery.data?.node ?? null;
  const summary = overviewQuery.data?.summary ?? null;
  const series = overviewQuery.data?.series ?? null;
  const loading = overviewQuery.isPending;
  const notFound = isNotFoundError(overviewQuery.error);
  const error = overviewQuery.error && !notFound ? queryErrorMessage(overviewQuery.error) : "";
  const updatedAt = overviewQuery.dataUpdatedAt || null;
  const health = node?.health ?? "never";
  const enabled = node?.enabled ?? false;
  const tone = !enabled ? "idle" : health === "ok" ? "ok" : health === "error" ? "error" : "idle";

  const deleteMutation = useMutation({
    mutationFn: () => deleteNode(nodeId),
    onSuccess: () => {
      setDeleteOpen(false);
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboardBase() });
      void navigate({ to: "/" });
    },
  });

  const deleteError = deleteMutation.error
    ? queryErrorMessage(deleteMutation.error, m.error_node_delete())
    : "";

  function handleDeleteOpenChange(open: boolean) {
    setDeleteOpen(open);
    if (!open) {
      setDeleteStep(1);
      deleteMutation.reset();
    }
  }

  function handleDeleteRequest() {
    setDeleteStep(1);
    setDeleteOpen(true);
  }

  function handleDeleteContinue() {
    setDeleteStep(2);
  }

  function handleDeleteConfirm() {
    deleteMutation.mutate();
  }

  const resetMutation = useMutation({
    mutationFn: () => resetNodeAPISecret(nodeId),
    onSuccess: (result) => {
      setResetResult(result);
      void queryClient.invalidateQueries({
        queryKey: queryKeys.nodeOverview(nodeId, trafficQuery),
      });
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboardBase() });
    },
  });

  const resetError = resetMutation.error
    ? queryErrorMessage(resetMutation.error, m.error_node_reset_api_secret())
    : "";

  function handleResetOpenChange(open: boolean) {
    setResetOpen(open);
    if (!open) {
      resetMutation.reset();
      setResetResult(null);
    }
  }

  function handleResetConfirm() {
    resetMutation.mutate();
  }

  return (
    <PageShell
      headerLeft={
        <>
          <SetBreadcrumbTitle title={node?.name} />
          <BrandLink />
        </>
      }
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
      <ErrorAlert message={error} icon className="mb-4" />

      {notFound ? (
        <Teaching
          title={m.node_not_found_title()}
          hint={m.node_not_found_hint()}
          action={
            <Button size="sm" variant="secondary" onPress={() => navigate({ to: "/" })}>
              {m.common_back_dashboard()}
            </Button>
          }
        />
      ) : (
        <>
          <DetailRail node={node} loading={loading && !node} now={now} />

          <NodeAlertsSection
            alerts={(alertsQuery.data?.items ?? []).filter((alert) => alert.status === "firing")}
            now={now}
          />

          <TrafficSection
            loading={loading && !series}
            trafficRange={trafficRange}
            onTrafficRangeChange={setTrafficRange}
            series={series}
            summary={summary}
          />

          <StreamsSection nodeId={nodeId} />

          {node && (
            <>
              <NodeDangerSection
                deletePending={deleteMutation.isPending}
                resetPending={resetMutation.isPending}
                onDeleteRequest={handleDeleteRequest}
                onResetRequest={() => setResetOpen(true)}
              />
              <DeleteNodeModal
                isOpen={deleteOpen}
                step={deleteStep}
                name={node.name || m.node_fallback_title()}
                pending={deleteMutation.isPending}
                error={deleteError}
                onOpenChange={handleDeleteOpenChange}
                onContinue={handleDeleteContinue}
                onConfirm={handleDeleteConfirm}
              />
              <ResetAPISecretModal
                isOpen={resetOpen}
                onOpenChange={handleResetOpenChange}
                pending={resetMutation.isPending}
                error={resetMutation.isError && !resetResult ? resetError : ""}
                result={resetResult}
                onConfirm={handleResetConfirm}
              />
            </>
          )}
        </>
      )}
    </PageShell>
  );
}

function NodeAlertsSection({ alerts, now }: { alerts: AlertItem[]; now: number | null }) {
  if (alerts.length === 0) return null;
  return (
    <Section title={m.monitoring_node_alerts()} meta={String(alerts.length)}>
      <div className="divide-y divide-separator">
        {alerts.map((alert) => (
          <Link
            key={alert.id}
            to="/settings/monitoring"
            className="flex items-center gap-3 px-3 py-2.5 text-foreground no-underline hover:bg-surface-secondary"
          >
            <SeverityBadge
              severity={alert.severity === "critical" ? "critical" : "warning"}
              label={
                alert.severity === "critical" ? m.monitoring_critical() : m.monitoring_warning()
              }
            />
            <span className="min-w-0 flex-1 truncate text-[13px] font-medium">
              {alert.monitor_name ?? m.monitoring_monitor()}
            </span>
            <span className="text-xs text-muted">
              {alert.started_at ? relTimeFromISO(alert.started_at, now) : m.common_em_dash()}
            </span>
          </Link>
        ))}
      </div>
    </Section>
  );
}

/* ── Detail rail (config + health) ─────────────────────────────────────── */

const railValue = "min-w-0 truncate text-[13px] leading-5 text-foreground";
const railMono = cn(railValue, "font-mono tabular-nums");

function DetailRail({
  node,
  loading,
  now,
}: {
  node: Node | null;
  loading: boolean;
  now: number | null;
}) {
  if (loading) {
    return (
      <div className="flex flex-col divide-y divide-border rounded-lg border bg-surface sm:flex-row sm:divide-x sm:divide-y-0">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="flex-1 px-4 py-3">
            <div className="h-3 w-16 animate-pulse rounded bg-surface-secondary" />
            <div className="mt-1 flex h-5 items-center">
              <div className="h-3.5 w-28 animate-pulse rounded bg-surface-secondary" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  const enabled = node?.enabled ?? false;
  const health = node?.health ?? "never";
  const stateLabel = !enabled
    ? m.node_state_disabled()
    : health === "error"
      ? node?.last_error || m.node_state_error()
      : health === "ok"
        ? m.node_state_healthy()
        : m.node_state_never_polled();
  const stateTone = !enabled ? "muted" : health === "error" ? "danger" : "muted";
  const txSpeed = enabled ? (node?.current_tx_speed ?? 0) : 0;
  const rxSpeed = enabled ? (node?.current_rx_speed ?? 0) : 0;

  return (
    <div className="flex flex-col divide-y divide-border rounded-lg border bg-surface sm:flex-row sm:divide-x sm:divide-y-0">
      <RailItem label={m.node_rail_endpoint()} className="sm:flex-[2]">
        <div className="group/key flex min-w-0 items-center gap-1.5">
          <span className={railMono}>{node?.api_url || m.common_em_dash()}</span>
          {node?.api_url && <CopyButton value={node.api_url} label={m.common_copy_endpoint()} />}
        </div>
      </RailItem>
      <RailItem label={m.node_rail_poll_interval()}>
        <span className={railMono}>
          {node?.poll_interval ? `${node.poll_interval}s` : m.common_em_dash()}
        </span>
      </RailItem>
      <RailItem label={m.node_rail_tx_speed()}>
        <span className={railMono}>
          <span className="text-muted">↑</span> {formatBytesPerSecond(txSpeed)}
        </span>
      </RailItem>
      <RailItem label={m.node_rail_rx_speed()}>
        <span className={railMono}>
          <span className="text-muted">↓</span> {formatBytesPerSecond(rxSpeed)}
        </span>
      </RailItem>
      <RailItem label={m.node_rail_last_poll()}>
        <span className={railValue}>
          {node?.last_polled_at ? relTimeFromISO(node.last_polled_at, now) : m.common_em_dash()}
        </span>
      </RailItem>
      <RailItem label={m.node_rail_state()}>
        <span className={cn(railValue, stateTone === "danger" && "text-danger")} title={stateLabel}>
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
    <div className={cn("min-w-0 flex-1 px-4 py-3", className)}>
      <div className="text-[11px] font-medium uppercase tracking-wider text-muted">{label}</div>
      <div className="mt-1 flex h-5 min-w-0 items-center">{children}</div>
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
  trafficRange: LocalDateRange;
  onTrafficRangeChange: (range: LocalDateRange) => void;
  series: TrafficSeries | null;
  summary: NodeTrafficSummary | null;
}) {
  const points = series?.points ?? [];
  const granularity = granularityForLocalRange(trafficRange);
  const totalTx = points.reduce((sum, p) => sum + (p.tx ?? 0), 0);
  const totalRx = points.reduce((sum, p) => sum + (p.rx ?? 0), 0);
  const byUser = (summary?.by_user ?? []).slice(0, 8);

  return (
    <Section
      title={m.common_traffic()}
      meta={
        !loading ? (
          <span className="font-mono tabular-nums">
            ↑ {formatBytes(totalTx)} · ↓ {formatBytes(totalRx)}
          </span>
        ) : undefined
      }
      action={<TrafficRangePicker value={trafficRange} onChange={onTrafficRangeChange} />}
    >
      <div className="p-3 sm:p-4">
        {loading ? (
          <div className="h-[220px] animate-pulse rounded bg-surface-secondary" />
        ) : points.length === 0 ? (
          <div className="grid h-[220px] place-items-center text-[13px] text-muted">
            {m.common_no_traffic_in_window()}
          </div>
        ) : (
          <TrafficChart points={points} granularity={granularity} idPrefix="node-traffic" />
        )}
      </div>

      {!loading && byUser.length > 0 && (
        <div className="overflow-x-auto border-t border-border">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-border bg-surface-secondary text-left">
                <Th>{m.node_traffic_top_users()}</Th>
                <Th className="text-right">{m.common_th_tx()}</Th>
                <Th className="text-right">{m.common_th_rx()}</Th>
                <Th className="text-right">{m.common_th_total()}</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-separator">
              {byUser.map((u, i) => (
                <tr
                  key={u.user?.id || i}
                  className="transition-colors duration-150 hover:bg-surface-secondary"
                >
                  <Td>
                    {u.user?.id ? (
                      <Link
                        to="/users/$userId"
                        params={{ userId: u.user.id }}
                        className="block max-w-[280px] truncate rounded-sm font-medium underline-offset-2 hover:text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
                      >
                        {u.user?.email || m.common_unknown()}
                      </Link>
                    ) : (
                      <span className="block max-w-[280px] truncate font-medium">
                        {u.user?.email || m.common_unknown()}
                      </span>
                    )}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-muted">↑</span> {formatBytes(u.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-muted">↓</span> {formatBytes(u.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
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

/* ── Danger zone ───────────────────────────────────────────────────────── */

function DangerRow({
  label,
  description,
  action,
}: {
  label: string;
  description: string;
  action: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-foreground">{label}</p>
        <p className="mt-0.5 text-xs leading-5 text-muted">{description}</p>
      </div>
      <div className="shrink-0">{action}</div>
    </div>
  );
}

function NodeDangerSection({
  deletePending,
  resetPending,
  onDeleteRequest,
  onResetRequest,
}: {
  deletePending: boolean;
  resetPending: boolean;
  onDeleteRequest: () => void;
  onResetRequest: () => void;
}) {
  return (
    <Section title={m.node_danger_section_title()}>
      <div className="flex flex-col gap-4 p-4">
        <DangerRow
          label={m.node_delete_title()}
          description={m.node_delete_hint()}
          action={
            <Button
              size="sm"
              variant="danger-soft"
              isDisabled={deletePending}
              onPress={onDeleteRequest}
            >
              {deletePending ? m.common_deleting() : m.common_delete()}
            </Button>
          }
        />
        <div className="h-px bg-separator" />
        <DangerRow
          label={m.node_reset_api_secret_title()}
          description={m.node_reset_api_secret_desc()}
          action={
            <Button
              size="sm"
              variant="secondary"
              isDisabled={resetPending}
              onPress={onResetRequest}
            >
              {resetPending ? m.user_manage_reset_submitting() : m.node_reset_api_secret_button()}
            </Button>
          }
        />
      </div>
    </Section>
  );
}

function ResetAPISecretModal({
  isOpen,
  onOpenChange,
  pending,
  error,
  result,
  onConfirm,
}: {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  result: NodeAPISecretReset | null;
  onConfirm: () => void;
}) {
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{m.node_reset_api_secret_confirm_title()}</Modal.Heading>
            <p className="mt-1.5 text-sm leading-5 text-muted">
              {m.node_reset_api_secret_confirm_body()}
            </p>
          </Modal.Header>
          <Modal.Body>
            {error && (
              <p className="text-[13px] text-danger" role="alert">
                {error}
              </p>
            )}

            {result?.api_secret && (
              <div className="rounded-lg border bg-surface-secondary p-3">
                <div className="flex items-center gap-2 text-[13px]">
                  <Dot tone="ok" />
                  <span className="font-medium">{m.node_reset_api_secret_done()}</span>
                </div>
                <p className="mt-1 text-xs text-muted">{m.node_reset_api_secret_new_hint()}</p>
                <div className="group/key mt-2 flex items-center gap-1.5">
                  <span className="min-w-0 truncate font-mono text-[12px] text-muted">
                    {result.api_secret}
                  </span>
                  <CopyButton value={result.api_secret} label={m.nodes_add_copy_api_secret()} />
                </div>
              </div>
            )}
          </Modal.Body>
          <Modal.Footer>
            <Button size="sm" variant="secondary" onPress={() => onOpenChange(false)}>
              {result ? m.common_sure() : m.user_manage_reset_cancel()}
            </Button>
            {!result && (
              <Button size="sm" variant="primary" isPending={pending} onPress={onConfirm}>
                {pending ? m.user_manage_reset_submitting() : m.node_reset_api_secret_button()}
              </Button>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

/* ── Delete node ───────────────────────────────────────────────────────── */

function DeleteNodeModal({
  isOpen,
  step,
  name,
  pending,
  error,
  onOpenChange,
  onContinue,
  onConfirm,
}: {
  isOpen: boolean;
  step: 1 | 2;
  name: string;
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
  onContinue: () => void;
  onConfirm: () => void;
}) {
  const title = step === 1 ? m.node_delete_title() : m.node_delete_confirm_title();
  const body = step === 1 ? m.node_delete_confirm({ name }) : m.node_delete_confirm_body({ name });

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
            <p className="mt-1.5 text-sm leading-5 text-muted">{body}</p>
          </Modal.Header>
          {step === 2 && error ? (
            <Modal.Body>
              <p className="text-[13px] text-danger" role="alert">
                {error}
              </p>
            </Modal.Body>
          ) : null}
          <Modal.Footer>
            <Button size="sm" variant="secondary" onPress={() => onOpenChange(false)}>
              {m.common_cancel()}
            </Button>
            {step === 1 ? (
              <Button size="sm" variant="primary" onPress={onContinue}>
                {m.node_delete_continue()}
              </Button>
            ) : (
              <Button
                size="sm"
                variant="primary"
                isPending={pending}
                onPress={onConfirm}
                className="bg-danger text-danger-foreground hover:opacity-90"
              >
                {pending ? m.common_deleting() : m.common_delete()}
              </Button>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

/* ── Streams dump (on demand) ──────────────────────────────────────────── */

function StreamsSection({ nodeId }: { nodeId: string }) {
  const tz = useActiveTimeZone();
  const now = useHydratedNow();
  const liveQuery = useQuery({
    queryKey: queryKeys.nodeLive(nodeId),
    queryFn: () => fetchNodeLive(nodeId),
    enabled: false,
  });

  const live = liveQuery.data ?? null;
  const loading = liveQuery.isFetching;
  const fetchedAt = liveQuery.dataUpdatedAt || null;
  const reqError = liveQuery.error
    ? queryErrorMessage(liveQuery.error, m.error_streams_network())
    : "";
  const byUser = live?.by_user ?? [];
  const topDomains = (live?.top_domains ?? []).slice(0, 12);
  const byConnection = live?.by_connection ?? [];

  return (
    <Section
      title={m.common_live_streams()}
      meta={
        live && !live.error ? (
          <span className="font-mono tabular-nums">
            {m.node_live_meta({
              devices: live.online_devices ?? 0,
              streams: live.active_streams ?? 0,
            })}
          </span>
        ) : undefined
      }
      action={
        <div className="flex items-center gap-2.5">
          {fetchedAt !== null && (
            <span
              className="hidden text-xs tabular-nums text-muted sm:inline"
              title={new Date(fetchedAt).toLocaleString(undefined, { timeZone: tz })}
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
            {fetchedAt === null ? m.common_fetch_streams() : m.common_refresh()}
          </Button>
        </div>
      }
    >
      {reqError ? (
        <PanelMessage>{reqError}</PanelMessage>
      ) : loading && !live ? (
        <TableSkeleton />
      ) : live?.error ? (
        <div className="px-4 py-3 text-[13px] text-danger" title={live.error}>
          {live.error}
        </div>
      ) : !live ? (
        <Teaching title={m.common_no_snapshot_title()} hint={m.node_live_empty_snapshot_hint()} />
      ) : byUser.length === 0 ? (
        <Teaching title={m.common_no_active_streams_title()} hint={m.node_live_no_streams_hint()} />
      ) : (
        <div className="flex flex-col">
          {byUser.map((u, i) => (
            <UserStreams key={u.user?.id || `unknown-${i}`} group={u} />
          ))}

          {topDomains.length > 0 && <TopDomainsTable rows={topDomains} />}
          {byConnection.length > 0 && <ByConnectionTable rows={byConnection} />}
        </div>
      )}
    </Section>
  );
}

function UserStreams({ group }: { group: NonNullable<NodeLive["by_user"]>[number] }) {
  const streams = group.streams ?? [];
  return (
    <div className="border-t border-border first:border-t-0">
      <div className="flex items-center justify-between gap-3 bg-surface-secondary px-3 py-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone="ok" />
          <span className="truncate text-xs font-medium">
            {group.user?.email || m.common_unknown()}
          </span>
        </div>
        <span className="shrink-0 font-mono text-xs tabular-nums text-muted">
          {m.node_live_user_meta({
            devices: group.online_devices ?? 0,
            count: streams.length,
          })}
        </span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-b border-separator text-left">
              <Th>{m.common_th_target()}</Th>
              <Th>{m.common_th_state()}</Th>
              <Th className="text-right">{m.common_th_tx()}</Th>
              <Th className="text-right">{m.common_th_rx()}</Th>
              <Th className="text-right">{m.common_th_lifetime()}</Th>
              <Th className="text-right">{m.common_th_idle()}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-separator">
            {streams.map((s, i) => {
              const target = s.hooked_req_addr || s.req_addr || m.common_em_dash();
              return (
                <tr
                  key={`${s.connection}-${s.stream}-${i}`}
                  className="transition-colors duration-150 hover:bg-surface-secondary"
                >
                  <Td>
                    <span
                      className="block max-w-[320px] truncate font-mono text-xs text-foreground"
                      title={target}
                    >
                      {target}
                    </span>
                  </Td>
                  <Td className="whitespace-nowrap font-mono text-xs text-muted">
                    {s.state || m.common_em_dash()}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    {formatBytes(s.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    {formatBytes(s.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
                    {formatDuration(s.lifetime_sec ?? -1)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
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
    <div className="bg-surface">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-y border-border bg-surface-secondary text-left">
              <Th>{m.common_top_domains()}</Th>
              <Th>{m.common_th_asn()}</Th>
              <Th>{m.common_th_country()}</Th>
              <Th className="text-right">{m.common_th_streams()}</Th>
              <Th className="text-right">{m.common_th_total()}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-separator">
            {rows.map((d, i) => {
              const domain = d.domain || m.common_em_dash();
              const meta = d.ip_meta;
              const countryTitle = meta?.country_name || meta?.country_code || "";
              return (
                <tr key={(d.domain || "") + i} className="hover:bg-surface-secondary">
                  <Td>
                    {meta?.ipinfo_url ? (
                      <a
                        href={meta.ipinfo_url}
                        target="_blank"
                        rel="noreferrer"
                        className="block max-w-[260px] truncate font-mono text-xs text-foreground underline decoration-border underline-offset-2 transition-colors duration-150 hover:text-accent"
                        title={m.common_ipinfo_open({ target: meta.ip || domain })}
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
                  <Td className="whitespace-nowrap font-mono text-xs text-muted">
                    {meta?.asn || m.common_em_dash()}
                  </Td>
                  <Td className="whitespace-nowrap text-xs text-muted">
                    {meta?.country_code ? (
                      <span title={countryTitle}>
                        <span className="font-mono text-foreground">{meta.country_code}</span>
                        {meta.country_name && (
                          <span className="ml-1 hidden max-w-[140px] truncate align-bottom sm:inline-block">
                            {meta.country_name}
                          </span>
                        )}
                      </span>
                    ) : (
                      m.common_em_dash()
                    )}
                  </Td>
                  <Td className="text-right font-mono text-xs tabular-nums text-muted">
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
    <div className="bg-surface">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-y border-border bg-surface-secondary text-left">
              <Th>{m.common_th_device()}</Th>
              <Th>{m.common_th_top_domain()}</Th>
              <Th className="text-right">{m.common_th_streams()}</Th>
              <Th className="text-right">{m.common_th_total()}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-separator">
            {rows.map((c, i) => (
              <tr key={`${c.connection}-${i}`} className="hover:bg-surface-secondary">
                <Td className="whitespace-nowrap font-mono text-xs tabular-nums text-muted">
                  #{c.connection ?? 0}
                </Td>
                <Td>
                  <span
                    className="block max-w-[200px] truncate font-mono text-xs"
                    title={c.top_domain || ""}
                  >
                    {c.top_domain || m.common_em_dash()}
                  </span>
                </Td>
                <Td className="text-right font-mono text-xs tabular-nums text-muted">
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
