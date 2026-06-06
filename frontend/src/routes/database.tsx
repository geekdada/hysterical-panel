import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@heroui/react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchDatabaseStats,
  pruneDatabaseTraffic,
  queryErrorMessage,
  queryKeys,
  REFRESH_MS,
} from "~/api/queries";
import { Dot, PanelMessage, Section, TableSkeleton, Td, Th } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, relTime } from "~/lib/format";

type DatabaseStats = components["schemas"]["DatabaseStatsResponse"];
type DatabasePrune = components["schemas"]["DatabasePruneResponse"];
type StorageFile = NonNullable<
  NonNullable<DatabaseStats["storage"]>["files"]
>[number];
type TrafficTable = NonNullable<DatabaseStats["traffic_tables"]>[number];

export const Route = createFileRoute("/database")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: DatabasePage,
});

function DatabasePage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const [now, setNow] = useState(() => Date.now());

  const statsQuery = useQuery({
    queryKey: queryKeys.databaseStats(),
    queryFn: fetchDatabaseStats,
    enabled: canQueryPanelApi(),
    refetchInterval: REFRESH_MS,
  });

  const pruneMutation = useMutation({
    mutationFn: pruneDatabaseTraffic,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.databaseStats() });
      void queryClient.invalidateQueries({ queryKey: queryKeys.dashboardBase() });
    },
  });

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(id);
  }, []);

  const stats = statsQuery.data ?? null;
  const storage = stats?.storage ?? null;
  const files = storage?.files ?? [];
  const tables = useMemo(() => stats?.traffic_tables ?? [], [stats]);
  const pruneEligible = tables.reduce(
    (sum, table) => sum + (table.older_than_30_days ?? 0),
    0,
  );
  const hourly = trafficTable(tables, "traffic_hourly");
  const daily = trafficTable(tables, "traffic_daily");
  const loading = statsQuery.isPending;
  const error = statsQuery.error ? queryErrorMessage(statsQuery.error) : "";
  const pruneError = pruneMutation.error
    ? queryErrorMessage(
        pruneMutation.error,
        "Network error while deleting old traffic data.",
      )
    : "";
  const updatedAt = statsQuery.dataUpdatedAt || null;

  function handlePrune() {
    if (pruneEligible <= 0 || pruneMutation.isPending) return;
    const ok = window.confirm(
      `Delete ${formatCount(pruneEligible)} traffic data points older than 30 days? This cannot be undone.`,
    );
    if (ok) pruneMutation.mutate();
  }

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
              <BackIcon />
            </Link>
            <div className="flex min-w-0 items-center gap-2">
              <Dot tone={error ? "error" : "ok"} />
              <span className="truncate text-[13px] font-semibold tracking-tight">
                Database management
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
        {error && (
          <div
            className="mb-4 flex items-center gap-2 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            <Dot tone="error" />
            <span>{error}</span>
          </div>
        )}

        <SummaryRail
          loading={loading}
          storageBytes={storage?.total_bytes ?? 0}
          hourly={hourly?.points ?? 0}
          daily={daily?.points ?? 0}
          pruneEligible={pruneEligible}
        />

        <Section
          title="Traffic data points"
          meta={stats?.cutoff ? `Retention cutoff ${formatCutoff(stats.cutoff)}` : undefined}
        >
          <TrafficTableSection loading={loading} tables={tables} />
        </Section>

        <Section title="Storage footprint">
          <StorageTable loading={loading} files={files} totalBytes={storage?.total_bytes ?? 0} />
        </Section>

        <Section
          title="Maintenance"
          meta="Deletes only traffic_hourly and traffic_daily rows"
          action={
            <Button
              size="sm"
              variant="secondary"
              isDisabled={pruneEligible <= 0 || pruneMutation.isPending}
              onPress={handlePrune}
              className="border-(--danger) text-(--danger) hover:bg-(--danger-soft)"
            >
              {pruneMutation.isPending ? "Deleting..." : "Delete >30 days"}
            </Button>
          }
        >
          <MaintenancePanel
            cutoff={stats?.cutoff ?? ""}
            pruneEligible={pruneEligible}
            pruneError={pruneError}
            result={pruneMutation.data ?? null}
          />
        </Section>
      </main>
    </div>
  );
}

function SummaryRail({
  loading,
  storageBytes,
  hourly,
  daily,
  pruneEligible,
}: {
  loading: boolean;
  storageBytes: number;
  hourly: number;
  daily: number;
  pruneEligible: number;
}) {
  return (
    <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
      <RailItem label="Storage" loading={loading} value={formatBytes(storageBytes)}>
        data.db footprint
      </RailItem>
      <RailItem label="Hourly points" loading={loading} value={formatCount(hourly)}>
        traffic_hourly rows
      </RailItem>
      <RailItem label="Daily points" loading={loading} value={formatCount(daily)}>
        traffic_daily rows
      </RailItem>
      <RailItem
        label="Prune eligible"
        loading={loading}
        value={formatCount(pruneEligible)}
        tone={pruneEligible > 0 ? "danger" : "muted"}
      >
        older than 30 days
      </RailItem>
    </div>
  );
}

function RailItem({
  label,
  loading,
  value,
  tone = "muted",
  children,
}: {
  label: string;
  loading: boolean;
  value: string;
  tone?: "danger" | "muted";
  children: ReactNode;
}) {
  if (loading) {
    return (
      <div className="flex-1 px-4 py-3">
        <div className="h-3 w-20 animate-pulse rounded bg-(--surface-secondary)" />
        <div className="mt-2 h-4 w-24 animate-pulse rounded bg-(--surface-secondary)" />
      </div>
    );
  }

  return (
    <div className="flex-1 px-4 py-3">
      <p className="text-[11px] font-medium uppercase tracking-wider text-(--muted)">
        {label}
      </p>
      <p
        className={`mt-1 font-mono text-[15px] tabular-nums ${tone === "danger" ? "text-(--danger)" : "text-(--foreground)"}`}
      >
        {value}
      </p>
      <p className="mt-0.5 truncate text-xs text-(--muted)">{children}</p>
    </div>
  );
}

function TrafficTableSection({
  loading,
  tables,
}: {
  loading: boolean;
  tables: TrafficTable[];
}) {
  if (loading) return <TableSkeleton rows={2} />;
  if (tables.length === 0) return <PanelMessage>No traffic tables found.</PanelMessage>;

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-[13px]">
        <thead className="border-b border-(--border) bg-(--surface-secondary)">
          <tr>
            <Th>Table</Th>
            <Th className="text-right">Data points</Th>
            <Th className="text-right">Older than 30 days</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {tables.map((table) => {
            const old = table.older_than_30_days ?? 0;
            return (
              <tr key={table.table} className="transition-colors duration-150 hover:bg-(--surface-secondary)">
                <Td>
                  <span className="font-mono text-[13px] text-(--foreground)">
                    {table.table ?? "—"}
                  </span>
                  <span className="ml-2 text-xs text-(--muted)">
                    {tableLabel(table.table)}
                  </span>
                </Td>
                <Td className="text-right font-mono tabular-nums">
                  {formatCount(table.points ?? 0)}
                </Td>
                <Td
                  className={`text-right font-mono tabular-nums ${old > 0 ? "text-(--danger)" : "text-(--muted)"}`}
                >
                  {formatCount(old)}
                </Td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function StorageTable({
  loading,
  files,
  totalBytes,
}: {
  loading: boolean;
  files: StorageFile[];
  totalBytes: number;
}) {
  if (loading) return <TableSkeleton rows={3} />;

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-[13px]">
        <thead className="border-b border-(--border) bg-(--surface-secondary)">
          <tr>
            <Th>File</Th>
            <Th className="text-right">Size</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {files.map((file) => (
            <tr key={file.name} className="transition-colors duration-150 hover:bg-(--surface-secondary)">
              <Td className="font-mono text-(--foreground)">{file.name ?? "—"}</Td>
              <Td className="text-right font-mono tabular-nums">
                {formatBytes(file.bytes ?? 0)}
              </Td>
            </tr>
          ))}
          <tr className="border-t border-(--border) bg-(--surface-secondary)">
            <Td className="font-medium text-(--foreground)">Total</Td>
            <Td className="text-right font-mono font-medium tabular-nums">
              {formatBytes(totalBytes)}
            </Td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}

function MaintenancePanel({
  cutoff,
  pruneEligible,
  pruneError,
  result,
}: {
  cutoff: string;
  pruneEligible: number;
  pruneError: string;
  result: DatabasePrune | null;
}) {
  const deleted = result?.deleted ?? [];
  const deletedTotal = deleted.reduce((sum, row) => sum + (row.deleted_rows ?? 0), 0);

  return (
    <div className="divide-y divide-(--separator)">
      <div className="px-4 py-3 text-[13px] text-(--muted)">
        Rows with <span className="font-mono text-(--foreground)">bucket</span>{" "}
        before{" "}
        <span className="font-mono text-(--foreground)">
          {cutoff ? formatCutoff(cutoff) : "—"}
        </span>{" "}
        are eligible. Current eligible rows:{" "}
        <span className="font-mono text-(--foreground) tabular-nums">
          {formatCount(pruneEligible)}
        </span>
        .
      </div>
      {pruneError && (
        <div className="flex items-center gap-2 bg-(--danger-soft) px-4 py-3 text-[13px] text-(--danger-soft-foreground)">
          <Dot tone="error" />
          <span>{pruneError}</span>
        </div>
      )}
      {result && (
        <div className="px-4 py-3 text-[13px] text-(--foreground)">
          Deleted{" "}
          <span className="font-mono tabular-nums">{formatCount(deletedTotal)}</span>{" "}
          rows.
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--muted)">
            {deleted.map((row) => (
              <span key={row.table} className="font-mono tabular-nums">
                {row.table}: {formatCount(row.deleted_rows ?? 0)}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function trafficTable(tables: TrafficTable[], tableName: string): TrafficTable | null {
  return tables.find((table) => table.table === tableName) ?? null;
}

function tableLabel(table?: string): string {
  if (table === "traffic_hourly") return "hour buckets";
  if (table === "traffic_daily") return "day buckets";
  return "";
}

function formatCount(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
}

function formatCutoff(value: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) return value;
  return new Date(parsed).toLocaleString();
}

function BackIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}
