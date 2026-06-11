import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { ChevronLeft } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchPanelTrafficSeries,
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
import { Dot, PanelMessage, Section } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, relTime } from "~/lib/format";

type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];

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

  const trafficSeriesQuery = useQuery({
    queryKey: queryKeys.analyticsOverview(trafficQuery),
    queryFn: () => fetchPanelTrafficSeries(trafficQuery!),
    enabled: queryEnabled && trafficQuery !== null,
    refetchInterval: REFRESH_MS,
  });

  const series = trafficSeriesQuery.data ?? null;
  const rangeLoading = trafficQuery === null || trafficSeriesQuery.isPending;
  const rangeError = trafficSeriesQuery.error
    ? queryErrorMessage(trafficSeriesQuery.error)
    : "";
  const queryErrors = [
    { key: "range", message: rangeError ? `Range: ${rangeError}` : "" },
  ].filter((e) => e.message);
  const updatedAt = trafficSeriesQuery.dataUpdatedAt || null;

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

        <RangeTrafficSection
          loading={rangeLoading}
          error={rangeError}
          onTrafficRangeChange={setTrafficRange}
          series={series}
          trafficRange={trafficRange}
        />
      </main>
    </div>
  );
}

function RangeTrafficSection({
  error,
  loading,
  onTrafficRangeChange,
  series,
  trafficRange,
}: {
  error: string;
  loading: boolean;
  onTrafficRangeChange: (range: LocalDateRange) => void;
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
      action={
        trafficRange ? (
          <TrafficRangePicker
            value={trafficRange}
            onChange={onTrafficRangeChange}
          />
        ) : (
          <div
            className="h-8 w-full shrink-0 rounded-(--radius) border border-(--border) bg-(--surface-secondary) animate-pulse sm:w-40"
            aria-hidden
          />
        )
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
