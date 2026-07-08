import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { Link, createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  analyticsOverviewQueryOptions,
  queryErrorMessage,
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
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  SortableTh,
  TableSkeleton,
  Td,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { cn } from "~/lib/cn";
import { formatBytes, formatLocaleDateTime, relTime } from "~/lib/format";
import { useActiveTimeZone } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type PanelNodeTraffic = components["schemas"]["PanelNodeTrafficResponse"];
type NodeBreakdownRow = NonNullable<PanelNodeTraffic["by_node"]>[number] & {
  total: number;
};

export const Route = createFileRoute("/analytics")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.analytics_title(),
    href: "/analytics",
  }),
  loader: async ({ context }) => {
    markResponsePrivate();
    const tz = context.timeZone ?? FALLBACK_TIME_ZONE;
    const range = toTrafficRangeQuery(defaultLocalTrafficRange(tz), tz);
    await Promise.allSettled([
      context.queryClient.ensureQueryData(analyticsOverviewQueryOptions(range)),
    ]);
  },
  component: AnalyticsPage,
});

function AnalyticsPage() {
  const { auth } = Route.useRouteContext();
  const tz = useActiveTimeZone();
  const [trafficRange, setTrafficRange] = useState<LocalDateRange>(() =>
    defaultLocalTrafficRange(tz)
  );
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setTrafficRange((current) => {
      const next = defaultLocalTrafficRange(tz);
      return current.start.compare(next.start) === 0 && current.end.compare(next.end) === 0
        ? current
        : next;
    });
  }, [tz]);

  useEffect(() => {
    const id = setInterval(() => {
      setNow(Date.now());
    }, 5_000);
    return () => clearInterval(id);
  }, []);

  const trafficQuery = toTrafficRangeQuery(trafficRange, tz);
  const overviewQuery = useQuery(analyticsOverviewQueryOptions(trafficQuery));

  const overview = overviewQuery.data ?? null;
  const series = overview?.series ?? null;
  const nodeTraffic = overview?.nodeTraffic ?? null;
  const rangeLoading = overviewQuery.isPending;
  const rangeError = overviewQuery.error ? queryErrorMessage(overviewQuery.error) : "";
  const queryErrors = [
    {
      key: "range",
      message: rangeError ? m.analytics_error_range_prefix({ error: rangeError }) : "",
    },
  ].filter((e) => e.message);
  const updatedAt = overviewQuery.dataUpdatedAt || null;

  return (
    <PageShell
      headerLeft={<BrandLink />}
      headerRight={
        <div className="flex items-center gap-3 text-xs text-(--muted)">
          {updatedAt !== null && (
            <span
              className="hidden tabular-nums sm:inline"
              title={formatLocaleDateTime(updatedAt, undefined, tz)}
            >
              {m.common_updated({ time: relTime(updatedAt, now) })}
            </span>
          )}
          <span className="hidden h-3.5 w-px bg-(--border) sm:block" />
          {auth && <UserMenu auth={auth} />}
        </div>
      }
    >
      {queryErrors.map((error) => (
        <ErrorAlert key={error.key} message={error.message} icon className="mb-4" />
      ))}

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
        <h2 className="shrink-0 text-[13px] font-semibold text-(--foreground)">
          {m.common_traffic()}
        </h2>
        <div className="min-w-0 w-full sm:w-auto">
          <TrafficRangePicker value={trafficRange} onChange={setTrafficRange} />
        </div>
      </div>

      <RangeTrafficSection
        loading={rangeLoading}
        error={rangeError}
        series={series}
        trafficRange={trafficRange}
      />

      <NodeBreakdownSection
        loading={rangeLoading}
        error={rangeError}
        rows={nodeTraffic?.by_node ?? []}
      />
    </PageShell>
  );
}

function RangeTrafficSection({
  error,
  loading,
  series,
  trafficRange,
}: {
  error: string;
  loading: boolean;
  series: TrafficSeries | null;
  trafficRange: LocalDateRange;
}) {
  const points = series?.points ?? [];
  const granularity = granularityForLocalRange(trafficRange);
  const totalTx = points.reduce((sum, p) => sum + (p.tx ?? 0), 0);
  const totalRx = points.reduce((sum, p) => sum + (p.rx ?? 0), 0);

  return (
    <Section
      title={m.analytics_section_all_nodes()}
      meta={
        !loading && !error ? (
          <span className="font-mono tabular-nums">
            ↑ {formatBytes(totalTx)} · ↓ {formatBytes(totalRx)}
          </span>
        ) : undefined
      }
    >
      {error ? (
        <PanelMessage>{error}</PanelMessage>
      ) : (
        <>
          <div className="p-3 sm:p-4">
            {loading ? (
              <div className="h-[220px] animate-pulse rounded bg-(--surface-secondary)" />
            ) : points.length === 0 ? (
              <div className="grid h-[220px] place-items-center text-[13px] text-(--muted)">
                {m.common_no_traffic_in_window()}
              </div>
            ) : (
              <TrafficChart
                points={points}
                granularity={granularity}
                idPrefix="analytics-traffic"
              />
            )}
          </div>
        </>
      )}
    </Section>
  );
}

function NodeBreakdownSection({
  error,
  loading,
  rows,
}: {
  error: string;
  loading: boolean;
  rows: NonNullable<PanelNodeTraffic["by_node"]>;
}) {
  return (
    <Section title={m.analytics_section_by_node()}>
      {error ? (
        <PanelMessage>{error}</PanelMessage>
      ) : loading ? (
        <TableSkeleton rows={4} />
      ) : rows.length === 0 ? (
        <PanelMessage>{m.analytics_no_node_traffic()}</PanelMessage>
      ) : (
        <NodeBreakdownTable rows={rows} />
      )}
    </Section>
  );
}

function formatNodeLabel(node: { name?: string; deleted?: boolean }) {
  const name = node?.name || m.common_em_dash();
  return node?.deleted ? `${name}${m.node_deleted_suffix()}` : name;
}

function NodeBreakdownTable({ rows }: { rows: NonNullable<PanelNodeTraffic["by_node"]> }) {
  const [sorting, setSorting] = useState<SortingState>([{ id: "total", desc: true }]);
  const tableRows = useMemo<NodeBreakdownRow[]>(
    () =>
      rows.map((row) => {
        const tx = row.tx ?? 0;
        const rx = row.rx ?? 0;
        return { ...row, total: tx + rx };
      }),
    [rows]
  );
  const columns = useMemo<ColumnDef<NodeBreakdownRow>[]>(
    () => [
      {
        accessorFn: (row) => row.node?.name ?? "",
        id: "name",
        sortDescFirst: false,
      },
      {
        accessorKey: "total",
        id: "total",
        sortDescFirst: true,
      },
      {
        accessorKey: "tx",
        id: "tx",
        sortDescFirst: true,
      },
      {
        accessorKey: "rx",
        id: "rx",
        sortDescFirst: true,
      },
    ],
    []
  );
  const table = useReactTable({
    columns,
    data: tableRows,
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
            <SortableTh column={table.getColumn("name")!}>{m.common_name()}</SortableTh>
            <SortableTh column={table.getColumn("total")!} align="right" className="text-right">
              {m.common_th_total()}
            </SortableTh>
            <SortableTh column={table.getColumn("tx")!} align="right" className="text-right">
              {m.common_th_tx()}
            </SortableTh>
            <SortableTh column={table.getColumn("rx")!} align="right" className="text-right">
              {m.common_th_rx()}
            </SortableTh>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {table.getRowModel().rows.map((row) => {
            const { node, rx, tx, total } = row.original;
            const id = node?.id ?? "";
            const deleted = node?.deleted ?? false;
            const label = formatNodeLabel(node ?? {});
            return (
              <tr
                key={id || `${label}-${row.id}`}
                className="transition-colors duration-150 hover:bg-(--surface-secondary)"
              >
                <Td>
                  {id && !deleted ? (
                    <Link
                      to="/nodes/$nodeId"
                      params={{ nodeId: id }}
                      className="block max-w-[180px] truncate rounded-sm font-medium underline-offset-2 hover:text-(--accent) hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                    >
                      {label}
                    </Link>
                  ) : (
                    <span
                      className={cn(
                        "block max-w-[180px] truncate font-medium",
                        deleted && "text-(--muted)"
                      )}
                    >
                      {label}
                    </span>
                  )}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  {formatBytes(total)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↑</span> {formatBytes(tx ?? 0)}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  <span className="text-(--muted)">↓</span> {formatBytes(rx ?? 0)}
                </Td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
