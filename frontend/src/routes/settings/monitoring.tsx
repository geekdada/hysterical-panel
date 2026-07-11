import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useForm } from "@tanstack/react-form";
import {
  Button,
  FieldError,
  Input,
  Label,
  Modal,
  NumberField,
  Separator,
  TextField,
} from "@heroui/react";
import { Plus, Pencil, TrashBin } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  alertsQueryOptions,
  createMonitor,
  deleteMonitor,
  dashboardNodesQueryOptions,
  monitorsQueryOptions,
  queryErrorMessage,
  queryKeys,
  fetchNotificationChannels,
  updateMonitor,
  type Alert,
  type AlertListResponse,
  type Monitor,
  type MonitorCreateRequest,
  type MonitorUpdateRequest,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  CheckboxListField,
  DestructiveConfirmModal,
  ErrorAlert,
  LabeledSwitch,
  PageShell,
  PanelMessage,
  Section,
  SelectField,
  SeverityBadge,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatBytesPerSecond, formatDuration, relTimeFromISO } from "~/lib/format";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/monitoring")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({ label: () => m.monitoring_title() }),
  loader: async ({ context }) => {
    markResponsePrivate();
    await Promise.allSettled([
      context.queryClient.ensureQueryData(monitorsQueryOptions()),
      context.queryClient.ensureQueryData(alertsQueryOptions({ status: "firing", per_page: 100 })),
      context.queryClient.ensureQueryData(alertsQueryOptions({ status: "history", per_page: 25 })),
      context.queryClient.ensureQueryData(dashboardNodesQueryOptions()),
    ]);
  },
  component: MonitoringPage,
});

type AlertItem = NonNullable<AlertListResponse["items"]>[number];

function MonitoringPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Monitor | "new" | null>(null);
  const [deleting, setDeleting] = useState<Monitor | null>(null);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyStatus, setHistoryStatus] = useState<"history" | "resolved" | "cancelled">(
    "history"
  );
  const [historySeverity, setHistorySeverity] = useState<"" | "warning" | "critical">("");
  const now = Date.now();
  const monitorsQuery = useQuery(monitorsQueryOptions());
  const activeQuery = useQuery(alertsQueryOptions({ status: "firing", per_page: 100 }));
  const historyQuery = useQuery(
    alertsQueryOptions({
      status: historyStatus,
      severity: historySeverity || undefined,
      page: historyPage,
      per_page: 25,
    })
  );
  const nodesQuery = useQuery(dashboardNodesQueryOptions());
  const channelsQuery = useQuery({
    queryKey: queryKeys.notificationChannels(),
    queryFn: fetchNotificationChannels,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.monitors() });
    void queryClient.invalidateQueries({ queryKey: queryKeys.alertsBase() });
    void queryClient.invalidateQueries({ queryKey: queryKeys.alertSummary() });
  };
  const createMutation = useMutation({
    mutationFn: createMonitor,
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
  });
  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: string; body: MonitorUpdateRequest }) =>
      updateMonitor(id, body),
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
  });
  const deleteMutation = useMutation({
    mutationFn: deleteMonitor,
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
  });

  const active = activeQuery.data?.items ?? [];
  const history = historyQuery.data?.items ?? [];
  const loadError = [
    monitorsQuery.error,
    activeQuery.error,
    historyQuery.error,
    nodesQuery.error,
    channelsQuery.error,
  ]
    .filter(Boolean)
    .map((error) => queryErrorMessage(error, m.monitoring_load_error()))
    .join(" ");

  return (
    <PageShell headerLeft={<BrandLink />} headerRight={auth ? <UserMenu auth={auth} /> : undefined}>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold tracking-tight">{m.monitoring_title()}</h1>
          <p className="mt-1 text-sm text-(--muted)">{m.monitoring_description()}</p>
        </div>
        <Button size="sm" variant="primary" onPress={() => setEditing("new")}>
          <Plus className="size-3.5" aria-hidden />
          {m.monitoring_new()}
        </Button>
      </div>
      <ErrorAlert message={loadError} icon className="mt-4" />

      <Section
        title={m.monitoring_active()}
        meta={m.monitoring_active_summary({ count: String(active.length) })}
      >
        {activeQuery.isPending ? (
          <TableSkeleton />
        ) : active.length === 0 ? (
          <div className="px-6 py-10 text-center">
            <p className="text-[13px] font-medium">{m.monitoring_no_active()}</p>
            <p className="mt-1 text-xs text-(--muted)">{m.monitoring_no_active_hint()}</p>
          </div>
        ) : (
          <AlertTable alerts={active} now={now} />
        )}
      </Section>

      <Section title={m.monitoring_monitors()}>
        {monitorsQuery.isPending ? (
          <TableSkeleton />
        ) : (monitorsQuery.data ?? []).length === 0 ? (
          <PanelMessage>{m.monitoring_no_monitors()}</PanelMessage>
        ) : (
          <div className="divide-y divide-(--separator)">
            {(monitorsQuery.data ?? []).map((monitor) => (
              <div key={monitor.id} className="flex items-center gap-3 px-3 py-2.5">
                <SeverityBadge
                  severity={monitor.severity === "critical" ? "critical" : "warning"}
                  label={
                    monitor.severity === "critical"
                      ? m.monitoring_critical()
                      : m.monitoring_warning()
                  }
                  className={monitor.enabled ? "" : "opacity-60"}
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-medium">{monitor.name}</p>
                  <p className="text-xs text-(--muted)">
                    {kindLabel(monitor.kind ?? "offline")} ·{" "}
                    {formatDuration(monitor.evaluation_window_seconds ?? 0)} ·{" "}
                    {monitor.node_scope === "all_enabled"
                      ? m.monitoring_scope_all()
                      : m.monitoring_scope_selected()}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={m.monitoring_edit()}
                  onPress={() => setEditing(monitor)}
                >
                  <Pencil className="size-3.5" aria-hidden />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={m.monitoring_delete()}
                  onPress={() => setDeleting(monitor)}
                >
                  <TrashBin className="size-3.5 text-(--danger)" aria-hidden />
                </Button>
              </div>
            ))}
          </div>
        )}
      </Section>

      <Section
        title={m.monitoring_history()}
        action={
          <div className="flex gap-2">
            <select
              className="h-8 rounded-(--radius) border border-(--border) bg-(--surface) px-2 text-xs"
              value={historyStatus}
              onChange={(event) => {
                setHistoryStatus(event.target.value as typeof historyStatus);
                setHistoryPage(1);
              }}
            >
              <option value="history">{m.monitoring_history_all()}</option>
              <option value="resolved">{m.monitoring_status_resolved()}</option>
              <option value="cancelled">{m.monitoring_status_cancelled()}</option>
            </select>
            <select
              className="h-8 rounded-(--radius) border border-(--border) bg-(--surface) px-2 text-xs"
              value={historySeverity}
              onChange={(event) => {
                setHistorySeverity(event.target.value as typeof historySeverity);
                setHistoryPage(1);
              }}
            >
              <option value="">{m.monitoring_severity_all()}</option>
              <option value="warning">{m.monitoring_warning()}</option>
              <option value="critical">{m.monitoring_critical()}</option>
            </select>
          </div>
        }
      >
        {historyQuery.isPending ? (
          <TableSkeleton />
        ) : history.length === 0 ? (
          <PanelMessage>{m.monitoring_no_history()}</PanelMessage>
        ) : (
          <>
            <AlertTable alerts={history} now={now} />
            <div className="flex items-center justify-end gap-2 border-t border-(--separator) px-3 py-2">
              <Button
                size="sm"
                variant="secondary"
                isDisabled={historyPage <= 1}
                onPress={() => setHistoryPage((page) => Math.max(1, page - 1))}
              >
                {m.users_pagination_previous()}
              </Button>
              <span className="text-xs tabular-nums text-(--muted)">{historyPage}</span>
              <Button
                size="sm"
                variant="secondary"
                isDisabled={historyPage * 25 >= (historyQuery.data?.total ?? 0)}
                onPress={() => setHistoryPage((page) => page + 1)}
              >
                {m.users_pagination_next()}
              </Button>
            </div>
          </>
        )}
      </Section>

      <MonitorModal
        key={editing === "new" ? "new" : (editing?.id ?? "closed")}
        value={editing}
        nodes={(nodesQuery.data ?? []).flatMap((node) =>
          node.id && node.name ? [{ id: node.id, name: node.name }] : []
        )}
        channels={(channelsQuery.data ?? []).flatMap((channel) =>
          channel.id && channel.name
            ? [{ id: channel.id, name: channel.name, enabled: channel.enabled ?? false }]
            : []
        )}
        pending={createMutation.isPending || updateMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(createMutation.error, m.monitoring_save_error())
            : updateMutation.error
              ? queryErrorMessage(updateMutation.error, m.monitoring_save_error())
              : ""
        }
        onOpenChange={(open) => !open && setEditing(null)}
        onSubmit={(body) => {
          if (editing === "new") createMutation.mutate(body as MonitorCreateRequest);
          else if (editing?.id) updateMutation.mutate({ id: editing.id, body });
        }}
      />
      <DestructiveConfirmModal
        isOpen={deleting !== null}
        title={m.monitoring_delete()}
        body={m.monitoring_delete_confirm({ name: deleting?.name ?? "" })}
        confirmLabel={m.monitoring_delete()}
        pendingLabel={m.monitoring_delete()}
        pending={deleteMutation.isPending}
        error={
          deleteMutation.error
            ? queryErrorMessage(deleteMutation.error, m.monitoring_delete_error())
            : ""
        }
        onOpenChange={(open) => !open && setDeleting(null)}
        onConfirm={() => deleting?.id && deleteMutation.mutate(deleting.id)}
      />
    </PageShell>
  );
}

function AlertTable({ alerts, now }: { alerts: AlertItem[]; now: number }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead>
          <tr>
            <Th>{m.monitoring_monitor()}</Th>
            <Th>{m.monitoring_node()}</Th>
            <Th>{m.monitoring_started()}</Th>
            <Th>{m.monitoring_duration()}</Th>
            <Th>{m.monitoring_delivery()}</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-(--separator)">
          {alerts.map((alert) => (
            <tr key={alert.id}>
              <Td>
                <div className="flex items-center gap-2">
                  <SeverityBadge
                    severity={alert.severity === "critical" ? "critical" : "warning"}
                    label={
                      alert.severity === "critical"
                        ? m.monitoring_critical()
                        : m.monitoring_warning()
                    }
                    className={alert.status === "firing" ? "" : "opacity-60"}
                  />
                  <div>
                    <p className="text-[13px] font-medium">
                      {alert.monitor_name ?? m.monitoring_monitor()}
                    </p>
                    <p className="text-xs text-(--muted)">
                      {statusLabel(alert.status ?? "cancelled")} · {alertValue(alert)}
                      {alert.resolution_reason ? ` · ${alert.resolution_reason}` : ""}
                    </p>
                  </div>
                </div>
              </Td>
              <Td>
                {alert.node?.id ? (
                  <Link
                    to="/nodes/$nodeId"
                    params={{ nodeId: alert.node.id }}
                    className="text-[13px] text-(--accent) no-underline hover:underline"
                  >
                    {alert.node.name ?? alert.node.id}
                  </Link>
                ) : (
                  m.common_em_dash()
                )}
              </Td>
              <Td className="text-xs text-(--muted)">
                {alert.started_at ? relTimeFromISO(alert.started_at, now) : m.common_em_dash()}
              </Td>
              <Td className="text-xs tabular-nums">
                {formatDuration(alert.duration_seconds ?? 0)}
              </Td>
              <Td className="whitespace-nowrap text-xs tabular-nums">
                <DeliverySummary alert={alert} />
              </Td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DeliverySummary({ alert }: { alert: AlertItem }) {
  const succeeded = alert.deliveries?.succeeded ?? 0;
  const failed = alert.deliveries?.failed ?? 0;
  const skipped = alert.deliveries?.skipped ?? 0;

  if ((alert.delivery_channel_count ?? 0) === 0) {
    return <span className="text-(--muted)">{m.monitoring_delivery_no_channels()}</span>;
  }

  if (succeeded + failed + skipped === 0) {
    return <span className="text-(--muted)">{m.monitoring_delivery_no_records()}</span>;
  }

  return m.monitoring_delivery_summary({
    succeeded: String(succeeded),
    failed: String(failed),
    skipped: String(skipped),
  });
}

function MonitorModal({
  value,
  nodes,
  channels,
  pending,
  error,
  onOpenChange,
  onSubmit,
}: {
  value: Monitor | "new" | null;
  nodes: Array<{ id: string; name: string }>;
  channels: Array<{ id: string; name: string; enabled: boolean }>;
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: MonitorCreateRequest | MonitorUpdateRequest) => void;
}) {
  const existing = value && value !== "new" ? value : null;
  // The parent keys this component by monitor id, so it remounts on every open —
  // these defaults are re-read each time the form is opened for a new/other row.
  const form = useForm({
    defaultValues: {
      name: existing?.name ?? "",
      kind: (existing?.kind ?? "offline") as "offline" | "high_traffic",
      severity: (existing?.severity ?? "warning") as "warning" | "critical",
      minutes: (existing?.evaluation_window_seconds ?? 300) / 60,
      // Threshold is edited in MB/s; stored in B/s.
      threshold: monitorThreshold(existing?.config) / 1_000_000,
      scope: (existing?.node_scope ?? "all_enabled") as "all_enabled" | "selected",
      nodeIDs: existing?.node_ids ?? [],
      channelIDs: existing?.channel_ids ?? [],
      enabled: existing?.enabled ?? true,
    },
    onSubmit: ({ value: v }) => {
      onSubmit({
        name: v.name.trim(),
        kind: v.kind,
        severity: v.severity,
        enabled: v.enabled,
        evaluation_window_seconds: Math.round(v.minutes * 60),
        node_scope: v.scope,
        node_ids: v.scope === "all_enabled" ? [] : v.nodeIDs,
        channel_ids: v.channelIDs,
        config:
          v.kind === "offline"
            ? {}
            : { threshold_bytes_per_second: Math.round(v.threshold * 1_000_000) },
      });
    },
  });

  return (
    <Modal.Backdrop isOpen={value !== null} onOpenChange={onOpenChange}>
      <Modal.Container size="lg" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(e) => {
              e.preventDefault();
              e.stopPropagation();
              void form.handleSubmit();
            }}
          >
            <Modal.Header>
              <Modal.Heading>
                {value === "new" ? m.monitoring_new() : m.monitoring_edit()}
              </Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-(--muted)">
                {value === "new" ? m.monitoring_new_hint() : m.monitoring_reconfigure_hint()}
              </p>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                {/* Inline errors stay hidden until a field is touched (typed, blurred,
                    or a submit attempt), so a freshly opened form isn't pre-painted red. */}
                <form.Field
                  name="name"
                  validators={{
                    onChange: ({ value: v }) =>
                      !v.trim() ? m.monitoring_name_required() : undefined,
                  }}
                >
                  {(field) => {
                    const invalid =
                      field.state.meta.isTouched && field.state.meta.errors.length > 0;
                    return (
                      <TextField
                        value={field.state.value}
                        onChange={field.handleChange}
                        onBlur={field.handleBlur}
                        isInvalid={invalid}
                        isRequired
                      >
                        <Label>{m.monitoring_name()}</Label>
                        <Input
                          autoFocus
                          maxLength={128}
                          autoComplete="off"
                          data-1p-ignore
                          data-lpignore="true"
                          data-form-type="other"
                        />
                        {invalid ? <FieldError>{m.monitoring_name_required()}</FieldError> : null}
                      </TextField>
                    );
                  }}
                </form.Field>

                <div className="grid gap-4 sm:grid-cols-2">
                  <form.Field name="kind">
                    {(field) => (
                      <SelectField
                        label={m.monitoring_kind()}
                        value={field.state.value}
                        onChange={(v) => field.handleChange(v as "offline" | "high_traffic")}
                        options={[
                          { value: "offline", label: m.monitoring_kind_offline() },
                          { value: "high_traffic", label: m.monitoring_kind_high_traffic() },
                        ]}
                      />
                    )}
                  </form.Field>
                  <form.Field name="severity">
                    {(field) => (
                      <SelectField
                        label={m.monitoring_severity()}
                        value={field.state.value}
                        onChange={(v) => field.handleChange(v as "warning" | "critical")}
                        options={[
                          { value: "warning", label: m.monitoring_warning() },
                          { value: "critical", label: m.monitoring_critical() },
                        ]}
                      />
                    )}
                  </form.Field>
                </div>

                <div className="flex flex-col gap-4">
                  <form.Field
                    name="minutes"
                    validators={{
                      onChange: ({ value: v }) => {
                        const seconds = Math.round(v * 60);
                        return !Number.isFinite(v) || seconds < 60 || seconds > 86400
                          ? m.monitoring_window_range()
                          : undefined;
                      },
                    }}
                  >
                    {(field) => {
                      const invalid =
                        field.state.meta.isTouched && field.state.meta.errors.length > 0;
                      return (
                        <NumberField
                          className="w-full sm:w-44 [--border-width-field:0px]"
                          value={field.state.value}
                          onChange={field.handleChange}
                          onBlur={field.handleBlur}
                          minValue={1}
                          maxValue={1440}
                          step={1}
                          isInvalid={invalid}
                          isRequired
                        >
                          <Label>{m.monitoring_window_minutes()}</Label>
                          <NumberField.Group>
                            <NumberField.DecrementButton />
                            <NumberField.Input />
                            <NumberField.IncrementButton />
                          </NumberField.Group>
                          {invalid ? <FieldError>{m.monitoring_window_range()}</FieldError> : null}
                        </NumberField>
                      );
                    }}
                  </form.Field>
                  {/* Threshold only applies to high-traffic monitors; unmounting it when
                      the kind switches away clears its validation so canSubmit stays right. */}
                  <form.Subscribe selector={(s) => s.values.kind}>
                    {(kind) =>
                      kind === "high_traffic" ? (
                        <form.Field
                          name="threshold"
                          validators={{
                            onChange: ({ value: v }) =>
                              !(v > 0) ? m.monitoring_threshold_positive() : undefined,
                          }}
                        >
                          {(field) => {
                            const invalid =
                              field.state.meta.isTouched && field.state.meta.errors.length > 0;
                            return (
                              <NumberField
                                className="w-full sm:w-60 [--border-width-field:0px]"
                                value={field.state.value}
                                onChange={field.handleChange}
                                onBlur={field.handleBlur}
                                minValue={0.001}
                                step={0.1}
                                isInvalid={invalid}
                                isRequired
                              >
                                <Label>{m.monitoring_threshold()}</Label>
                                <NumberField.Group>
                                  <NumberField.DecrementButton />
                                  <NumberField.Input />
                                  <NumberField.IncrementButton />
                                </NumberField.Group>
                                {invalid ? (
                                  <FieldError>{m.monitoring_threshold_positive()}</FieldError>
                                ) : null}
                              </NumberField>
                            );
                          }}
                        </form.Field>
                      ) : null
                    }
                  </form.Subscribe>
                </div>

                <form.Field name="scope">
                  {(field) => (
                    <SelectField
                      label={m.monitoring_scope()}
                      value={field.state.value}
                      onChange={(v) => field.handleChange(v as "all_enabled" | "selected")}
                      options={[
                        { value: "all_enabled", label: m.monitoring_scope_all() },
                        { value: "selected", label: m.monitoring_scope_selected() },
                      ]}
                    />
                  )}
                </form.Field>
                <form.Subscribe selector={(s) => s.values.scope}>
                  {(scope) =>
                    scope === "selected" ? (
                      <form.Field
                        name="nodeIDs"
                        validators={{
                          onChange: ({ value: v }) =>
                            v.length === 0 ? m.monitoring_nodes_required() : undefined,
                        }}
                      >
                        {(field) => (
                          <CheckboxListField
                            label={m.monitoring_nodes()}
                            values={field.state.value}
                            onChange={field.handleChange}
                            options={nodes}
                            emptyLabel={m.monitoring_nodes_empty()}
                            isInvalid={field.state.value.length === 0}
                            errorMessage={m.monitoring_nodes_required()}
                          />
                        )}
                      </form.Field>
                    ) : null
                  }
                </form.Subscribe>

                <form.Field name="channelIDs">
                  {(field) => (
                    <CheckboxListField
                      label={m.monitoring_channels()}
                      values={field.state.value}
                      onChange={field.handleChange}
                      options={channels.map((channel) => ({
                        id: channel.id,
                        name: channel.name,
                        note: channel.enabled ? undefined : m.monitoring_channel_disabled(),
                      }))}
                      emptyLabel={m.monitoring_channels_empty()}
                    />
                  )}
                </form.Field>

                <Separator />

                <form.Field name="enabled">
                  {(field) => (
                    <LabeledSwitch
                      label={m.monitoring_enabled()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
              </div>
              {error ? (
                <p className="mt-4 text-[13px] text-(--danger)" role="alert">
                  {error}
                </p>
              ) : null}
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {m.common_cancel()}
              </Button>
              <form.Subscribe selector={(s) => s.canSubmit}>
                {(canSubmit) => (
                  <Button
                    type="submit"
                    variant="primary"
                    isPending={pending}
                    isDisabled={pending || !canSubmit}
                  >
                    {value === "new" ? m.monitoring_create() : m.monitoring_save()}
                  </Button>
                )}
              </form.Subscribe>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function kindLabel(kind: string) {
  return kind === "offline" ? m.monitoring_kind_offline() : m.monitoring_kind_high_traffic();
}
function statusLabel(status: string) {
  return status === "firing"
    ? m.monitoring_status_firing()
    : status === "resolved"
      ? m.monitoring_status_resolved()
      : m.monitoring_status_cancelled();
}
function alertValue(alert: AlertItem | Alert) {
  const value = alert.status === "resolved" ? alert.recovery_value : alert.firing_value;
  const speed = value?.average_bytes_per_second;
  return typeof speed === "number"
    ? formatBytesPerSecond(speed)
    : kindLabel(alert.monitor_kind ?? "offline");
}
function monitorThreshold(config: unknown): number {
  return config &&
    typeof config === "object" &&
    "threshold_bytes_per_second" in config &&
    typeof config.threshold_bytes_per_second === "number"
    ? config.threshold_bytes_per_second
    : 20_000_000;
}
