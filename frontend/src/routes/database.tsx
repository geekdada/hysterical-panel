import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@heroui/react";
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
import {
  BackLink,
  Dot,
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, formatLocaleCount, formatLocaleDateTime, relTime } from "~/lib/format";
import { useActiveTimeZone } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

type DatabaseStats = components["schemas"]["DatabaseStatsResponse"];
type DatabasePrune = components["schemas"]["DatabasePruneResponse"];
type StorageFile = NonNullable<NonNullable<DatabaseStats["storage"]>["files"]>[number];
type TrafficTable = NonNullable<DatabaseStats["traffic_tables"]>[number];

export const Route = createFileRoute("/database")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: DatabasePage,
});

function DatabasePage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const tz = useActiveTimeZone();
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
      void queryClient.invalidateQueries({
        queryKey: queryKeys.databaseStats(),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.dashboardBase(),
      });
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
  const pruneEligible = tables.reduce((sum, table) => sum + (table.older_than_30_days ?? 0), 0);
  const hourly = trafficTable(tables, "traffic_hourly");
  const daily = trafficTable(tables, "traffic_daily");
  const loading = statsQuery.isPending;
  const error = statsQuery.error ? queryErrorMessage(statsQuery.error) : "";
  const pruneError = pruneMutation.error
    ? queryErrorMessage(pruneMutation.error, m.error_database_prune_network())
    : "";
  const updatedAt = statsQuery.dataUpdatedAt || null;

  function handlePrune() {
    if (pruneEligible <= 0 || pruneMutation.isPending) return;
    const ok = window.confirm(
      m.database_prune_confirm({ count: formatLocaleCount(pruneEligible) })
    );
    if (ok) pruneMutation.mutate();
  }

  return (
    <PageShell
      headerLeft={
        <div className="flex min-w-0 items-center gap-3">
          <BackLink preferHistoryBack label={m.common_back_settings()} />
          <span className="truncate text-[13px] font-semibold tracking-tight">
            {m.database_title()}
          </span>
        </div>
      }
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
      <ErrorAlert message={error} icon className="mb-4" />

      <SummaryRail
        loading={loading}
        storageBytes={storage?.total_bytes ?? 0}
        hourly={hourly?.points ?? 0}
        daily={daily?.points ?? 0}
        pruneEligible={pruneEligible}
      />

      <Section
        title={m.database_section_traffic_points()}
        meta={
          stats?.cutoff
            ? m.database_retention_cutoff({ cutoff: formatCutoff(stats.cutoff, tz) })
            : undefined
        }
      >
        <TrafficTableSection loading={loading} tables={tables} />
      </Section>

      <Section title={m.database_section_storage()}>
        <StorageTable loading={loading} files={files} totalBytes={storage?.total_bytes ?? 0} />
      </Section>

      <Section
        title={m.database_section_maintenance()}
        meta={m.database_maintenance_meta()}
        action={
          <Button
            size="sm"
            variant="secondary"
            isDisabled={pruneEligible <= 0 || pruneMutation.isPending}
            onPress={handlePrune}
            className="border-(--danger) text-(--danger) hover:bg-(--danger-soft)"
          >
            {pruneMutation.isPending ? m.database_prune_deleting() : m.database_prune_button()}
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
    </PageShell>
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
      <RailItem
        label={m.database_rail_storage()}
        loading={loading}
        value={formatBytes(storageBytes)}
      >
        {m.database_rail_storage_hint()}
      </RailItem>
      <RailItem
        label={m.database_rail_hourly()}
        loading={loading}
        value={formatLocaleCount(hourly)}
      >
        {m.database_rail_hourly_hint()}
      </RailItem>
      <RailItem label={m.database_rail_daily()} loading={loading} value={formatLocaleCount(daily)}>
        {m.database_rail_daily_hint()}
      </RailItem>
      <RailItem
        label={m.database_rail_prune_eligible()}
        loading={loading}
        value={formatLocaleCount(pruneEligible)}
        tone={pruneEligible > 0 ? "danger" : "muted"}
      >
        {m.database_rail_prune_eligible_hint()}
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
      <p className="text-[11px] font-medium uppercase tracking-wider text-(--muted)">{label}</p>
      <p
        className={`mt-1 font-mono text-[15px] tabular-nums ${tone === "danger" ? "text-(--danger)" : "text-(--foreground)"}`}
      >
        {value}
      </p>
      <p className="mt-0.5 truncate text-xs text-(--muted)">{children}</p>
    </div>
  );
}

function TrafficTableSection({ loading, tables }: { loading: boolean; tables: TrafficTable[] }) {
  if (loading) return <TableSkeleton rows={2} />;
  if (tables.length === 0) return <PanelMessage>{m.database_no_traffic_tables()}</PanelMessage>;

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-[13px]">
        <thead className="border-b border-(--border) bg-(--surface-secondary)">
          <tr>
            <Th>{m.database_th_table()}</Th>
            <Th className="text-right">{m.database_th_data_points()}</Th>
            <Th className="text-right">{m.database_th_older_than_30d()}</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {tables.map((table) => {
            const old = table.older_than_30_days ?? 0;
            return (
              <tr
                key={table.table}
                className="transition-colors duration-150 hover:bg-(--surface-secondary)"
              >
                <Td>
                  <span className="font-mono text-[13px] text-(--foreground)">
                    {table.table ?? m.common_em_dash()}
                  </span>
                  <span className="ml-2 text-xs text-(--muted)">{tableLabel(table.table)}</span>
                </Td>
                <Td className="text-right font-mono tabular-nums">
                  {formatLocaleCount(table.points ?? 0)}
                </Td>
                <Td
                  className={`text-right font-mono tabular-nums ${old > 0 ? "text-(--danger)" : "text-(--muted)"}`}
                >
                  {formatLocaleCount(old)}
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
            <Th>{m.database_th_file()}</Th>
            <Th className="text-right">{m.database_th_size()}</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {files.map((file) => (
            <tr
              key={file.name}
              className="transition-colors duration-150 hover:bg-(--surface-secondary)"
            >
              <Td className="font-mono text-(--foreground)">{file.name ?? m.common_em_dash()}</Td>
              <Td className="text-right font-mono tabular-nums">{formatBytes(file.bytes ?? 0)}</Td>
            </tr>
          ))}
          <tr className="border-t border-(--border) bg-(--surface-secondary)">
            <Td className="font-medium text-(--foreground)">{m.common_total()}</Td>
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
  const tz = useActiveTimeZone();
  const deleted = result?.deleted ?? [];
  const deletedTotal = deleted.reduce((sum, row) => sum + (row.deleted_rows ?? 0), 0);

  return (
    <div className="divide-y divide-(--separator)">
      <div className="px-4 py-3 text-[13px] text-(--muted)">
        {m.database_maintenance_eligible({
          bucketField: "bucket",
          cutoff: cutoff ? formatCutoff(cutoff, tz) : m.common_em_dash(),
          count: formatLocaleCount(pruneEligible),
        })}
      </div>
      {pruneError && (
        <div className="flex items-center gap-2 bg-(--danger-soft) px-4 py-3 text-[13px] text-(--danger-soft-foreground)">
          <Dot tone="error" />
          <span>{pruneError}</span>
        </div>
      )}
      {result && (
        <div className="px-4 py-3 text-[13px] text-(--foreground)">
          {m.database_maintenance_deleted({ count: formatLocaleCount(deletedTotal) })}
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--muted)">
            {deleted.map((row) => (
              <span key={row.table} className="font-mono tabular-nums">
                {m.database_maintenance_deleted_row({
                  table: row.table ?? m.common_em_dash(),
                  count: formatLocaleCount(row.deleted_rows ?? 0),
                })}
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
  if (table === "traffic_hourly") return m.database_table_hour_buckets();
  if (table === "traffic_daily") return m.database_table_day_buckets();
  return "";
}

function formatCutoff(value: string, tz: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) return value;
  return formatLocaleDateTime(parsed, undefined, tz);
}
