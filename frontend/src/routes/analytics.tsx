import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { ChevronLeft } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchAnalyticsOverview,
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
  Dot,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, relTime } from "~/lib/format";

type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type PanelNodeTraffic = components["schemas"]["PanelNodeTrafficResponse"];

export const Route = createFileRoute("/analytics")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: AnalyticsPage,
});

function AnalyticsPage() {
  const { auth } = Route.useRouteContext();
  const [trafficRange, setTrafficRange] = useState<LocalDateRange | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setTrafficRange(defaultLocalTrafficRange());

    const id = setInterval(() => {
      setNow(Date.now());
    }, 5_000);
    return () => clearInterval(id);
  }, []);

  const queryEnabled = canQueryPanelApi();
  const trafficQuery = trafficRange ? toTrafficRangeQuery(trafficRange) : null;

  const overviewQuery = useQuery({
    queryKey: queryKeys.analyticsOverview(trafficQuery),
    queryFn: () => fetchAnalyticsOverview(trafficQuery!),
    enabled: queryEnabled && trafficQuery !== null,
    refetchInterval: REFRESH_MS,
  });

  const overview = overviewQuery.data ?? null;
  const series = overview?.series ?? null;
  const nodeTraffic = overview?.nodeTraffic ?? null;
  const rangeLoading = trafficQuery === null || overviewQuery.isPending;
  const rangeError = overviewQuery.error
    ? queryErrorMessage(overviewQuery.error)
    : "";
  const queryErrors = [
    { key: "range", message: rangeError ? `Range: ${rangeError}` : "" },
  ].filter((e) => e.message);
  const updatedAt = overviewQuery.dataUpdatedAt || null;

  return (
    <div className="min-h-svh bg-(--background) text-(--foreground)">
      <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
        <div className="mx-auto flex h-12 max-w-7xl items-center justify-between px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-2.5">
            <Link
              to="/"
              aria-label="Back to dashboard"
              className="grid size-6 shrink-0 place-items-center rounded text-(--muted) transition-colors duration-150 hover:bg-(--surface-secondary) hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
            >
              <ChevronLeft className="size-3.5" aria-hidden />
            </Link>
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate text-[13px] font-semibold tracking-tight">
                Analytics
              </span>
            </div>
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

        <div className="flex items-center justify-between gap-3">
          <h2 className="text-[13px] font-semibold text-(--foreground)">
            Traffic
          </h2>
          {trafficRange ? (
            <TrafficRangePicker
              value={trafficRange}
              onChange={setTrafficRange}
            />
          ) : (
            <div
              className="h-8 w-full shrink-0 rounded-(--radius) border border-(--border) bg-(--surface-secondary) animate-pulse sm:w-40"
              aria-hidden
            />
          )}
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
          nodeTraffic={nodeTraffic}
        />
      </main>
    </div>
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
  trafficRange: LocalDateRange | null;
}) {
  const points = series?.points ?? [];
  const granularity = trafficRange
    ? granularityForLocalRange(trafficRange)
    : "hourly";
  const totalTx = points.reduce((sum, p) => sum + (p.tx ?? 0), 0);
  const totalRx = points.reduce((sum, p) => sum + (p.rx ?? 0), 0);

  return (
    <Section
      title="All nodes traffic"
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
                No traffic recorded in this window.
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
  nodeTraffic,
}: {
  error: string;
  loading: boolean;
  nodeTraffic: PanelNodeTraffic | null;
}) {
  const rows = nodeTraffic?.by_node ?? [];

  return (
    <Section title="By node">
      {error ? (
        <PanelMessage>{error}</PanelMessage>
      ) : loading ? (
        <TableSkeleton rows={4} />
      ) : rows.length === 0 ? (
        <PanelMessage>No node traffic in this window.</PanelMessage>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
                <Th>Name</Th>
                <Th className="text-right">Total</Th>
                <Th className="text-right">TX</Th>
                <Th className="text-right">RX</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--separator)">
              {rows.map((row, i) => {
                const tx = row.tx ?? 0;
                const rx = row.rx ?? 0;
                const id = row.node?.id ?? "";
                const name = row.node?.name || "—";
                return (
                  <tr
                    key={id || `${name}-${i}`}
                    className="transition-colors duration-150 hover:bg-(--surface-secondary)"
                  >
                    <Td>
                      {id ? (
                        <Link
                          to="/nodes/$nodeId"
                          params={{ nodeId: id }}
                          className="block max-w-[180px] truncate rounded-sm font-medium underline-offset-2 hover:text-(--accent) hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                        >
                          {name}
                        </Link>
                      ) : (
                        <span className="font-medium">{name}</span>
                      )}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      {formatBytes(tx + rx)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      <span className="text-(--muted)">↑</span>{" "}
                      {formatBytes(tx)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      <span className="text-(--muted)">↓</span>{" "}
                      {formatBytes(rx)}
                    </Td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}
