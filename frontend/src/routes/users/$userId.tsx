import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Chip, Modal } from "@heroui/react";
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
  createIgnoredConnectionIP,
  fetchPanelConfigQuery,
  fetchUserLive,
  isNotFoundError,
  panelConfigQueryOptions,
  queryErrorMessage,
  queryKeys,
  resetUserAuthString,
  toTrafficRangeQuery,
  updateUserStatus,
  userOverviewQueryOptions,
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
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from "~/components/ui";
import { SetBreadcrumbTitle } from "~/components/breadcrumbs";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatBytes, formatDuration, relTime, relTimeFromISO } from "~/lib/format";
import { cn } from "~/lib/cn";
import { useActiveTimeZone } from "~/lib/use-timezone";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

type PanelUser = components["schemas"]["PanelUser"];
type UserDetail = components["schemas"]["UserDetail"];
type TrafficSummary = components["schemas"]["TrafficSummaryResponse"];
type TrafficSeries = components["schemas"]["TrafficSeriesResponse"];
type UserLive = components["schemas"]["LiveResponse"];

export const Route = createFileRoute("/users/$userId")({
  beforeLoad: ({ context, params }) => requireAdminOrSelf(context.auth, params.userId),
  staticData: breadcrumbStaticData({
    label: () => m.user_fallback_account(),
    dynamic: true,
  }),
  loader: async ({ context, params }) => {
    markResponsePrivate();
    const tz = context.timeZone ?? FALLBACK_TIME_ZONE;
    const range = toTrafficRangeQuery(defaultLocalTrafficRange(tz), tz);
    await Promise.allSettled([
      context.queryClient.ensureQueryData(userOverviewQueryOptions(params.userId, range)),
      context.queryClient.ensureQueryData(panelConfigQueryOptions()),
    ]);
  },
  component: AccountDetailPage,
});

function AccountDetailPage() {
  const { userId } = Route.useParams();
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const isAdmin = auth?.user.role === "admin";
  const tz = useActiveTimeZone();

  const [trafficRange, setTrafficRange] = useState<LocalDateRange>(() =>
    defaultLocalTrafficRange(tz)
  );
  const now = useHydratedNow();

  useEffect(() => {
    setTrafficRange((current) => {
      const next = defaultLocalTrafficRange(tz);
      return current.start.compare(next.start) === 0 && current.end.compare(next.end) === 0
        ? current
        : next;
    });
  }, [tz]);

  const trafficQuery = toTrafficRangeQuery(trafficRange, tz);
  const overviewQuery = useQuery(userOverviewQueryOptions(userId, trafficQuery));

  const user = overviewQuery.data?.user ?? null;
  const summary = overviewQuery.data?.summary ?? null;
  const series = overviewQuery.data?.series ?? null;
  const loading = overviewQuery.isPending;
  const notFound = isNotFoundError(overviewQuery.error);
  const error = overviewQuery.error && !notFound ? queryErrorMessage(overviewQuery.error) : "";
  const updatedAt = overviewQuery.dataUpdatedAt || null;

  function handleLogout() {
    clearAuth();
    window.location.href = "/login";
  }

  return (
    <PageShell
      headerLeft={
        <>
          <SetBreadcrumbTitle title={user?.email} />
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
          title={m.user_not_found_title()}
          hint={m.user_not_found_hint()}
          action={
            isAdmin ? (
              <Button size="sm" variant="secondary" onPress={() => navigate({ to: "/" })}>
                {m.common_back_dashboard()}
              </Button>
            ) : (
              <Button size="sm" variant="secondary" onPress={handleLogout}>
                {m.user_sign_out()}
              </Button>
            )
          }
        />
      ) : (
        <>
          <AccountRail user={user} loading={loading && !user} now={now} />

          <TrafficSection
            loading={loading && !series}
            trafficRange={trafficRange}
            onTrafficRangeChange={setTrafficRange}
            series={series}
            summary={summary}
            isAdmin={isAdmin}
          />

          {user && (
            <RecentConnectionsSection
              rows={user.recent_connections ?? []}
              now={now}
              isAdmin={isAdmin}
              userId={userId}
            />
          )}

          {isAdmin && <LiveSection userId={userId} />}

          {isAdmin && user && (
            <ManageSection userId={userId} user={user} isSelf={auth?.user.id === userId} />
          )}

          {isAdmin && user && <PasskeysSection userId={userId} isSelf={auth?.user.id === userId} />}
        </>
      )}
    </PageShell>
  );
}

function AccountRail({
  user,
  loading,
  now,
}: {
  user: UserDetail | null;
  loading: boolean;
  now: number | null;
}) {
  const tz = useActiveTimeZone();
  if (loading) {
    return (
      <div className="overflow-hidden rounded-lg border bg-surface">
        <div className="grid divide-y divide-border md:grid-cols-[minmax(0,1.5fr)_minmax(0,1.5fr)_8rem_9rem_9rem] md:divide-x md:divide-y-0">
          <RailSkeletonCell labelClassName="max-w-16" valueClassName="max-w-44" />
          <RailSkeletonCell labelClassName="max-w-20" valueClassName="max-w-48" />
          <RailSkeletonCell labelClassName="max-w-12" valueClassName="max-w-16" />
          <RailSkeletonCell labelClassName="max-w-14" valueClassName="max-w-20" />
          <RailSkeletonCell labelClassName="max-w-24" valueClassName="max-w-24" />
        </div>
        <div className="grid border-t border-border divide-y divide-border md:grid-cols-4 md:divide-x md:divide-y-0">
          <RailSkeletonCell labelClassName="max-w-24" valueClassName="max-w-28" />
          <RailSkeletonCell labelClassName="max-w-10" valueClassName="max-w-24" />
          <RailSkeletonCell labelClassName="max-w-10" valueClassName="max-w-24" />
          <RailSkeletonCell labelClassName="max-w-16" valueClassName="max-w-24" />
        </div>
      </div>
    );
  }

  const status = user?.status ?? "active";
  const active = status === "active";
  const usedTx = user?.used_tx ?? 0;
  const usedRx = user?.used_rx ?? 0;

  return (
    <div className="overflow-hidden rounded-lg border bg-surface">
      <div className="grid divide-y divide-border md:grid-cols-[minmax(0,1.5fr)_minmax(0,1.5fr)_8rem_9rem_9rem] md:divide-x md:divide-y-0">
        <RailItem label={m.user_rail_email()}>
          <span className="block truncate text-[13px]" title={user?.email || ""}>
            {user?.email || m.common_em_dash()}
          </span>
        </RailItem>
        <RailItem label={m.user_rail_auth_key()}>
          <div className="group/key flex min-w-0 items-center gap-1.5">
            <span className="block min-w-0 truncate font-mono text-[13px] text-foreground">
              {user?.auth_string || m.common_em_dash()}
            </span>
            {user?.auth_string && (
              <CopyButton value={user.auth_string} label={m.common_copy_auth_key()} />
            )}
          </div>
        </RailItem>
        <RailItem label={m.common_role()}>
          <span className="font-mono text-[13px]">{user?.role || m.common_em_dash()}</span>
        </RailItem>
        <RailItem label={m.common_status()}>
          <Chip
            size="sm"
            variant="soft"
            color={active ? "success" : "default"}
            className="capitalize"
          >
            {active ? m.common_active() : m.common_disabled()}
          </Chip>
        </RailItem>
        <RailItem label={m.user_rail_last_connect()}>
          <span
            className="text-[13px] tabular-nums"
            title={
              user?.last_connected_at
                ? new Date(user.last_connected_at).toLocaleString(undefined, { timeZone: tz })
                : undefined
            }
          >
            {user?.last_connected_at
              ? relTimeFromISO(user.last_connected_at, now)
              : m.common_never()}
          </span>
        </RailItem>
      </div>
      <div className="grid border-t border-border divide-y divide-border md:grid-cols-4 md:divide-x md:divide-y-0">
        <RailItem label={m.user_rail_used_total()}>
          <span className="font-mono text-[15px] font-semibold tabular-nums">
            {formatBytes(usedTx + usedRx)}
          </span>
        </RailItem>
        <RailItem label={m.common_th_tx()}>
          <span className="font-mono text-[13px] tabular-nums">
            <span className="text-muted">↑</span> {formatBytes(usedTx)}
          </span>
        </RailItem>
        <RailItem label={m.common_th_rx()}>
          <span className="font-mono text-[13px] tabular-nums">
            <span className="text-muted">↓</span> {formatBytes(usedRx)}
          </span>
        </RailItem>
        <RailItem label={m.user_rail_online_devices()}>
          <span className="text-[13px] tabular-nums">
            {user?.online_devices == null ? m.common_em_dash() : user.online_devices}
          </span>
        </RailItem>
      </div>
    </div>
  );
}

function RailSkeletonCell({
  labelClassName,
  valueClassName,
}: {
  labelClassName: string;
  valueClassName: string;
}) {
  return (
    <div className="min-w-0 px-4 py-3">
      <div
        className={cn("h-3 w-full animate-pulse rounded bg-surface-secondary", labelClassName)}
      />
      <div
        className={cn("mt-2 h-4 w-full animate-pulse rounded bg-surface-secondary", valueClassName)}
      />
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
      <div className="mt-1 min-w-0">{children}</div>
    </div>
  );
}

function RecentConnectionsSection({
  rows,
  now,
  isAdmin,
  userId,
}: {
  rows: NonNullable<PanelUser["recent_connections"]>;
  now: number | null;
  isAdmin: boolean;
  userId: string;
}) {
  const tz = useActiveTimeZone();
  const queryClient = useQueryClient();
  const [ignoringIp, setIgnoringIp] = useState<string | null>(null);

  const ignoreMutation = useMutation({
    mutationFn: (ip: string) => createIgnoredConnectionIP({ ip }),
    onMutate: (ip) => setIgnoringIp(ip),
    onSettled: () => setIgnoringIp(null),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.all, "users", userId] });
      void queryClient.invalidateQueries({ queryKey: queryKeys.ignoredConnectionIPs() });
    },
  });

  return (
    <Section title={m.user_recent_connections_title()}>
      {rows.length === 0 ? (
        <Teaching
          title={m.user_recent_connections_empty_title()}
          hint={m.user_recent_connections_empty_hint()}
        />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-border bg-surface-secondary text-left">
                <Th>{m.common_th_ip()}</Th>
                <Th>{m.common_th_asn()}</Th>
                <Th>{m.common_th_country()}</Th>
                <Th className="text-right">{m.common_th_last_seen()}</Th>
                {isAdmin && <Th className="w-24 text-right" />}
              </tr>
            </thead>
            <tbody className="divide-y divide-separator">
              {rows.map((row, i) => {
                const ip = row.ip || m.common_em_dash();
                const meta = row.ip_meta;
                const countryTitle = meta?.country_name || meta?.country_code || "";
                const canIgnore = Boolean(row.ip) && ignoringIp !== row.ip;
                const isIgnoring = Boolean(row.ip) && ignoringIp === row.ip;
                return (
                  <tr key={`${row.ip || "ip"}-${i}`} className="hover:bg-surface-secondary">
                    <Td>
                      {meta?.ipinfo_url ? (
                        <a
                          href={meta.ipinfo_url}
                          target="_blank"
                          rel="noreferrer"
                          className="block max-w-[260px] truncate font-mono text-xs text-foreground underline decoration-border underline-offset-2 transition-colors duration-150 hover:text-accent"
                          title={m.common_ipinfo_open({ target: meta.ip || ip })}
                        >
                          {ip}
                        </a>
                      ) : (
                        <span className="block max-w-[260px] truncate font-mono text-xs" title={ip}>
                          {ip}
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
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
                      <span
                        title={
                          row.last_seen_at
                            ? new Date(row.last_seen_at).toLocaleString(undefined, { timeZone: tz })
                            : undefined
                        }
                      >
                        {row.last_seen_at
                          ? relTimeFromISO(row.last_seen_at, now)
                          : m.common_em_dash()}
                      </span>
                    </Td>
                    {isAdmin && (
                      <Td className="text-right">
                        {row.ip ? (
                          <Button
                            variant="secondary"
                            size="sm"
                            isDisabled={!canIgnore || ignoreMutation.isPending}
                            onPress={() => ignoreMutation.mutate(row.ip!)}
                          >
                            {isIgnoring
                              ? m.user_recent_connections_ignoring()
                              : m.user_recent_connections_ignore()}
                          </Button>
                        ) : null}
                      </Td>
                    )}
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

function ManageRow({
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

function ManageSection({
  userId,
  user,
  isSelf,
}: {
  userId: string;
  user: PanelUser;
  isSelf: boolean;
}) {
  const queryClient = useQueryClient();
  const [resetOpen, setResetOpen] = useState(false);
  const [resetResult, setResetResult] = useState<PanelUser | null>(null);

  function invalidateAllUsers() {
    void queryClient.invalidateQueries({ queryKey: [...queryKeys.all, "users"] });
    void queryClient.invalidateQueries({ queryKey: queryKeys.userStats() });
  }

  const toggleMutation = useMutation({
    mutationFn: ({ status }: { status: "active" | "disabled" }) => updateUserStatus(userId, status),
    onSuccess: invalidateAllUsers,
  });

  const resetMutation = useMutation({
    mutationFn: () => resetUserAuthString(userId),
    onSuccess: (updated) => {
      setResetResult(updated);
      invalidateAllUsers();
    },
  });

  const active = (user.status ?? "active") === "active";
  const toggleError = toggleMutation.error
    ? queryErrorMessage(toggleMutation.error, m.error_user_update_network())
    : "";
  const resetError = resetMutation.error
    ? queryErrorMessage(resetMutation.error, m.error_user_reset_auth_network())
    : "";
  const error = toggleError || resetError;

  function handleToggleStatus() {
    const next: "active" | "disabled" = active ? "disabled" : "active";
    toggleMutation.mutate({ status: next });
  }

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
    <>
      <Section title={m.user_manage_title()}>
        {error && (
          <div
            className="border-b border-border bg-danger-soft px-4 py-2 text-[13px] text-danger-soft-foreground"
            role="alert"
          >
            {error}
          </div>
        )}
        <div className="flex flex-col gap-4 p-4">
          <ManageRow
            label={m.user_manage_status_label()}
            description={
              active ? m.user_manage_status_active_desc() : m.user_manage_status_disabled_desc()
            }
            action={
              <Button
                size="sm"
                variant={active ? "danger-soft" : "primary"}
                isPending={toggleMutation.isPending}
                isDisabled={isSelf}
                onPress={handleToggleStatus}
              >
                {active ? m.users_deactivate() : m.users_activate()}
              </Button>
            }
          />

          <div className="h-px bg-separator" />

          <ManageRow
            label={m.user_manage_reset_label()}
            description={m.user_manage_reset_desc()}
            action={
              <Button
                size="sm"
                variant="secondary"
                onPress={() => {
                  setResetResult(null);
                  resetMutation.reset();
                  setResetOpen(true);
                }}
              >
                {m.user_manage_reset_auth_key()}
              </Button>
            }
          />
        </div>
      </Section>

      <ResetAuthKeyModal
        isOpen={resetOpen}
        onOpenChange={handleResetOpenChange}
        pending={resetMutation.isPending}
        error={resetMutation.isError && !resetResult ? resetError : ""}
        result={resetResult}
        onConfirm={handleResetConfirm}
      />
    </>
  );
}

function ResetAuthKeyModal({
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
  result: PanelUser | null;
  onConfirm: () => void;
}) {
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{m.user_manage_reset_confirm_title()}</Modal.Heading>
            <p className="mt-1.5 text-sm leading-5 text-muted">
              {m.user_manage_reset_confirm_body()}
            </p>
          </Modal.Header>
          <Modal.Body>
            {error && (
              <p className="text-[13px] text-danger" role="alert">
                {error}
              </p>
            )}

            {result && (
              <div className="rounded-lg border bg-surface-secondary p-3">
                <div className="flex items-center gap-2 text-[13px]">
                  <Dot tone="ok" />
                  <span className="font-medium">{m.user_manage_reset_done()}</span>
                </div>
                <p className="mt-1 text-xs text-muted">{m.user_manage_reset_new_key_hint()}</p>
                {result.auth_string && (
                  <div className="group/key mt-2 flex items-center gap-1.5">
                    <span className="min-w-0 truncate font-mono text-[12px] text-muted">
                      {result.auth_string}
                    </span>
                    <CopyButton value={result.auth_string} label={m.common_copy_auth_key()} />
                  </div>
                )}
              </div>
            )}
          </Modal.Body>
          <Modal.Footer>
            <Button slot="close" variant="secondary">
              {result ? m.common_cancel() : m.user_manage_reset_cancel()}
            </Button>
            {!result && (
              <Button variant="danger" isPending={pending} isDisabled={pending} onPress={onConfirm}>
                {pending ? m.user_manage_reset_submitting() : m.user_manage_reset_submit()}
              </Button>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function PasskeysSection({ userId, isSelf }: { userId: string; isSelf: boolean }) {
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
    mutationFn: () => registerPasskey(userId, m.user_passkeys_default_name(), false),
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
      ? queryErrorMessage(addMutation.error, m.error_passkey_add())
      : "";
  const deleteError = deleteMutation.error
    ? queryErrorMessage(deleteMutation.error, m.error_passkey_delete())
    : "";
  const error = passkeysQuery.error
    ? queryErrorMessage(passkeysQuery.error)
    : addError || deleteError;

  return (
    <Section
      title={m.user_passkeys_title()}
      meta={loading ? undefined : m.user_passkeys_meta({ count: String(rows.length) })}
      action={
        isSelf ? (
          <Button
            size="sm"
            variant="secondary"
            isDisabled={addMutation.isPending}
            onPress={() => addMutation.mutate()}
          >
            {addMutation.isPending ? m.user_passkeys_adding() : m.user_passkeys_add()}
          </Button>
        ) : undefined
      }
    >
      {error && (
        <div
          className="border-b border-border bg-danger-soft px-3 py-2 text-[13px] text-danger-soft-foreground"
          role="alert"
        >
          {error}
        </div>
      )}
      {loading ? (
        <TableSkeleton rows={2} />
      ) : rows.length === 0 ? (
        <Teaching
          title={m.user_passkeys_empty_title()}
          hint={isSelf ? m.user_passkeys_empty_hint_self() : m.user_passkeys_empty_hint_other()}
        />
      ) : (
        <PasskeysTable
          rows={rows}
          deletingId={
            deleteMutation.isPending && deleteMutation.variables
              ? deleteMutation.variables.passkeyId
              : undefined
          }
          onDelete={(passkey) => {
            const name = passkey.name || m.user_passkeys_default_name();
            if (window.confirm(m.user_passkeys_delete_confirm({ name }))) {
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
  deletingId,
  onDelete,
}: {
  rows: Passkey[];
  deletingId?: string;
  onDelete: (passkey: Passkey) => void;
}) {
  const now = useHydratedNow();
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">
        <thead>
          <tr className="border-b border-border bg-surface-secondary text-left">
            <Th>{m.common_name()}</Th>
            <Th>{m.user_passkeys_th_transports()}</Th>
            <Th>{m.user_passkeys_th_backup()}</Th>
            <Th className="text-right">{m.user_passkeys_th_last_used()}</Th>
            <Th className="text-right">{m.user_passkeys_th_action()}</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-separator">
          {rows.map((passkey) => (
            <tr
              key={passkey.id}
              className="transition-colors duration-150 hover:bg-surface-secondary"
            >
              <Td>
                <span className="font-medium">
                  {passkey.name || m.user_passkeys_default_name()}
                </span>
              </Td>
              <Td>
                <span className="font-mono text-xs text-muted">
                  {passkey.transports?.length ? passkey.transports.join(", ") : m.common_em_dash()}
                </span>
              </Td>
              <Td>
                <span className="text-xs text-muted">
                  {passkey.backup_state
                    ? m.user_passkeys_backup_synced()
                    : passkey.backup_eligible
                      ? m.user_passkeys_backup_eligible()
                      : m.user_passkeys_backup_device_bound()}
                </span>
              </Td>
              <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
                {passkey.last_used_at
                  ? relTimeFromISO(passkey.last_used_at, now)
                  : m.common_never()}
              </Td>
              <Td className="text-right">
                <Button
                  size="sm"
                  variant="secondary"
                  isDisabled={deletingId === passkey.id}
                  onPress={() => onDelete(passkey)}
                >
                  {deletingId === passkey.id ? m.common_deleting() : m.common_delete()}
                </Button>
              </Td>
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
  trafficRange: LocalDateRange;
  onTrafficRangeChange: (range: LocalDateRange) => void;
  series: TrafficSeries | null;
  summary: TrafficSummary | null;
  isAdmin: boolean;
}) {
  const points = series?.points ?? [];
  const granularity = granularityForLocalRange(trafficRange);
  const totalTx = points.reduce((sum, p) => sum + (p.tx ?? 0), 0);
  const totalRx = points.reduce((sum, p) => sum + (p.rx ?? 0), 0);
  const byNode = [...(summary?.by_node ?? [])]
    .sort((a, b) => (b.tx ?? 0) + (b.rx ?? 0) - ((a.tx ?? 0) + (a.rx ?? 0)))
    .slice(0, 8);

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
          <TrafficChart points={points} granularity={granularity} idPrefix="account-traffic" />
        )}
      </div>

      {!loading && byNode.length > 0 && (
        <div className="overflow-x-auto border-t border-border">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-border bg-surface-secondary text-left">
                <Th>{m.user_traffic_top_nodes()}</Th>
                <Th className="text-right">{m.common_th_tx()}</Th>
                <Th className="text-right">{m.common_th_rx()}</Th>
                <Th className="text-right">{m.common_th_total()}</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-separator">
              {byNode.map((n, i) => (
                <tr
                  key={n.node?.id || i}
                  className="transition-colors duration-150 hover:bg-surface-secondary"
                >
                  <Td>
                    {isAdmin && n.node?.id ? (
                      <Link
                        to="/nodes/$nodeId"
                        params={{ nodeId: n.node.id }}
                        className="block max-w-[280px] truncate rounded-sm font-medium underline-offset-2 hover:text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
                      >
                        {n.node?.name || m.common_unknown()}
                      </Link>
                    ) : (
                      <span className="block max-w-[280px] truncate font-medium">
                        {n.node?.name || m.common_unknown()}
                      </span>
                    )}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-muted">↑</span> {formatBytes(n.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-muted">↓</span> {formatBytes(n.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-muted">
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
  const tz = useActiveTimeZone();
  const now = useHydratedNow();
  const liveQuery = useQuery({
    queryKey: queryKeys.userLive(userId),
    queryFn: () => fetchUserLive(userId),
    enabled: false,
  });

  const live = liveQuery.data ?? null;
  const loading = liveQuery.isFetching;
  const fetchedAt = liveQuery.dataUpdatedAt || null;
  const reqError = liveQuery.error
    ? queryErrorMessage(liveQuery.error, m.error_streams_network())
    : "";
  const byNode = live?.by_node ?? [];
  const visibleByNode = byNode.filter((n) => n.error || (n.streams?.length ?? 0) > 0);
  const topDomains = (live?.top_domains ?? []).slice(0, 12);
  const byConnection = live?.by_connection ?? [];

  return (
    <Section
      title={m.common_live_streams()}
      meta={
        live ? (
          <span className="font-mono tabular-nums">
            {m.user_live_meta({ streams: String(live.active_streams ?? 0) })}
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
      ) : !live ? (
        <Teaching title={m.common_no_snapshot_title()} hint={m.user_live_empty_snapshot_hint()} />
      ) : byNode.length === 0 ? (
        <Teaching title={m.user_live_no_nodes_title()} hint={m.user_live_no_nodes_hint()} />
      ) : visibleByNode.length === 0 ? (
        <Teaching title={m.common_no_active_streams_title()} hint={m.user_live_no_streams_hint()} />
      ) : (
        <div className="flex flex-col">
          {visibleByNode.map((n, i) => (
            <NodeStreams key={n.node?.id || i} group={n} />
          ))}

          {topDomains.length > 0 && <TopDomainsTable rows={topDomains} />}
          {byConnection.length > 0 && <ByConnectionTable rows={byConnection} />}
        </div>
      )}
    </Section>
  );
}

function NodeStreams({ group }: { group: NonNullable<UserLive["by_node"]>[number] }) {
  const streams = group.streams ?? [];
  const hasError = Boolean(group.error);
  return (
    <div className="border-t border-border first:border-t-0">
      <div className="flex items-center justify-between gap-3 bg-surface-secondary px-3 py-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone={hasError ? "error" : streams.length > 0 ? "ok" : "idle"} />
          <span className="truncate text-xs font-medium">
            {group.node?.name || m.common_unknown()}
          </span>
        </div>
        <span className="shrink-0 font-mono text-xs tabular-nums text-muted">
          {m.user_live_node_meta({ count: String(streams.length) })}
        </span>
      </div>
      {hasError ? (
        <div className="px-3 py-2 text-[13px] text-danger" title={group.error}>
          {group.error}
        </div>
      ) : (
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
      )}
    </div>
  );
}

function TopDomainsTable({ rows }: { rows: NonNullable<UserLive["top_domains"]> }) {
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

function ByConnectionTable({ rows }: { rows: NonNullable<UserLive["by_connection"]> }) {
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
