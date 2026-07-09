import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Input, Label, Modal, Switch, TextField } from "@heroui/react";
import { Eye, Link as LinkIcon, Pencil, Play, Plus, TrashBin } from "@gravity-ui/icons";
import { isPasskeySoftError, listPasskeys } from "~/api/auth";
import { requireAdmin } from "~/api/guards";
import {
  createNotificationChannel,
  deleteNotificationChannel,
  fetchNotificationChannels,
  fetchPanelConfigQuery,
  queryErrorMessage,
  queryKeys,
  revealNotificationChannelURL,
  testNotificationChannel,
  updateNotificationChannel,
  type NotificationChannel,
  type NotificationChannelCreateRequest,
  type NotificationChannelUpdateRequest,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  CopyableCode,
  DestructiveConfirmModal,
  Dot,
  ErrorAlert,
  LabeledSwitch,
  PageShell,
  PanelMessage,
  Section,
  TableSkeleton,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { relTimeFromISO } from "~/lib/format";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/notifications")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.notifications_title(),
  }),
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData({
      queryKey: queryKeys.notificationChannels(),
      queryFn: fetchNotificationChannels,
    });
  },
  component: NotificationChannelsPage,
});

function NotificationChannelsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const adminID = auth?.user.id ?? "";
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<NotificationChannel | null>(null);
  const [deleting, setDeleting] = useState<NotificationChannel | null>(null);
  const [revealed, setRevealed] = useState<{ name: string; url: string } | null>(null);

  const channelsQuery = useQuery({
    queryKey: queryKeys.notificationChannels(),
    queryFn: fetchNotificationChannels,
  });
  const configQuery = useQuery({
    queryKey: queryKeys.config(),
    queryFn: fetchPanelConfigQuery,
    staleTime: Infinity,
  });
  const passkeysQuery = useQuery({
    queryKey: ["panel", "users", adminID, "passkeys"] as const,
    queryFn: () => listPasskeys(adminID),
    enabled: adminID !== "" && (configQuery.data?.passkeys_enabled ?? false),
  });

  const invalidate = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.notificationChannels() });

  const createMutation = useMutation({
    mutationFn: createNotificationChannel,
    onSuccess: () => {
      setCreateOpen(false);
      invalidate();
    },
  });
  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: string; body: NotificationChannelUpdateRequest }) =>
      updateNotificationChannel(id, body),
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
  });
  const testMutation = useMutation({
    mutationFn: testNotificationChannel,
    onSuccess: invalidate,
  });
  const deleteMutation = useMutation({
    mutationFn: deleteNotificationChannel,
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
  });
  const revealMutation = useMutation({
    mutationFn: revealNotificationChannelURL,
    onSuccess: (data, id) => {
      const channel = (channelsQuery.data ?? []).find((item) => item.id === id);
      setRevealed({ name: channel?.name ?? m.notifications_title(), url: data.url ?? "" });
    },
  });

  const channels = channelsQuery.data ?? [];
  const passkeysConfigured = configQuery.data?.passkeys_enabled ?? false;
  const canReveal = passkeysConfigured && (passkeysQuery.data?.length ?? 0) > 0;
  const showEnrollmentHint =
    passkeysConfigured && !passkeysQuery.isPending && (passkeysQuery.data?.length ?? 0) === 0;
  const loadError = channelsQuery.error ? queryErrorMessage(channelsQuery.error) : "";
  const actionError = [testMutation.error, revealMutation.error]
    .filter((error) => error && !isPasskeySoftError(error))
    .map((error) => queryErrorMessage(error, m.error_notification_channel_test_network()))[0];

  function clearRevealed() {
    setRevealed(null);
    revealMutation.reset();
  }

  function openCreate(open: boolean) {
    setCreateOpen(open);
    if (!open) createMutation.reset();
  }

  function closeEdit(open: boolean) {
    if (!open) {
      setEditing(null);
      updateMutation.reset();
    }
  }

  function closeDelete(open: boolean) {
    if (!open) {
      setDeleting(null);
      deleteMutation.reset();
    }
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-base font-semibold tracking-tight">{m.notifications_title()}</h1>
          <p className="mt-0.5 max-w-2xl text-[13px] text-(--muted)">{m.notifications_intro()}</p>
        </div>
        <a
          href="https://beszel.dev/guide/notifications/"
          target="_blank"
          rel="noreferrer"
          className="inline-flex shrink-0 items-center gap-1.5 text-[13px] font-medium text-(--accent) hover:opacity-80"
        >
          <LinkIcon className="size-3.5" aria-hidden />
          {m.notifications_docs()}
        </a>
      </div>

      <ErrorAlert message={loadError || actionError} icon className="mb-4" />

      {showEnrollmentHint && (
        <div className="mb-4 rounded-(--radius) border border-(--border) bg-(--surface-secondary) px-4 py-3 text-[13px] text-(--muted)">
          {m.notifications_passkey_required()}{" "}
          <Link to="/settings" className="font-medium text-(--accent) hover:opacity-80">
            {m.nav_settings()}
          </Link>
        </div>
      )}

      <ChannelFormModal
        isOpen={createOpen}
        onOpenChange={openCreate}
        pending={createMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(createMutation.error, m.error_notification_channel_save_network())
            : ""
        }
        onSubmit={(body) => createMutation.mutate(body as NotificationChannelCreateRequest)}
      />
      <ChannelFormModal
        channel={editing}
        isOpen={editing !== null}
        onOpenChange={closeEdit}
        pending={updateMutation.isPending}
        error={
          updateMutation.error
            ? queryErrorMessage(updateMutation.error, m.error_notification_channel_save_network())
            : ""
        }
        onSubmit={(body) => {
          if (editing?.id) {
            updateMutation.mutate({
              id: editing.id,
              body: body as NotificationChannelUpdateRequest,
            });
          }
        }}
      />
      <DestructiveConfirmModal
        isOpen={deleting !== null}
        title={m.notifications_delete_title()}
        body={m.notifications_delete_confirm({ name: deleting?.name ?? "" })}
        confirmLabel={m.common_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={
          deleteMutation.error
            ? queryErrorMessage(deleteMutation.error, m.error_notification_channel_delete_network())
            : ""
        }
        onOpenChange={closeDelete}
        onConfirm={() => deleting?.id && deleteMutation.mutate(deleting.id)}
      />
      <RevealModal revealed={revealed} onOpenChange={(open) => !open && clearRevealed()} />

      <Section
        className="mt-0"
        title={m.notifications_title()}
        meta={channels.length > 0 ? m.common_total() + `: ${channels.length}` : undefined}
        action={
          <Button size="sm" variant="secondary" onPress={() => setCreateOpen(true)}>
            <Plus className="size-3.5" aria-hidden />
            {m.notifications_new()}
          </Button>
        }
      >
        {channelsQuery.isPending ? (
          <TableSkeleton />
        ) : channels.length === 0 ? (
          <PanelMessage>
            <span className="block font-medium text-(--foreground)">
              {m.notifications_empty_title()}
            </span>
            <span className="mt-1 block">{m.notifications_empty_hint()}</span>
          </PanelMessage>
        ) : (
          <div className="divide-y divide-(--separator)">
            {channels.map((channel) => (
              <ChannelRow
                key={channel.id}
                channel={channel}
                canReveal={canReveal}
                testing={testMutation.isPending && testMutation.variables === channel.id}
                revealing={revealMutation.isPending && revealMutation.variables === channel.id}
                toggling={updateMutation.isPending && updateMutation.variables?.id === channel.id}
                onToggle={(enabled) =>
                  channel.id && updateMutation.mutate({ id: channel.id, body: { enabled } })
                }
                onTest={() => channel.id && testMutation.mutate(channel.id)}
                onEdit={() => {
                  clearRevealed();
                  setEditing(channel);
                }}
                onReveal={() => channel.id && revealMutation.mutate(channel.id)}
                onDelete={() => setDeleting(channel)}
              />
            ))}
          </div>
        )}
      </Section>
    </PageShell>
  );
}

function ChannelRow({
  channel,
  canReveal,
  testing,
  revealing,
  toggling,
  onToggle,
  onTest,
  onEdit,
  onReveal,
  onDelete,
}: {
  channel: NotificationChannel;
  canReveal: boolean;
  testing: boolean;
  revealing: boolean;
  toggling: boolean;
  onToggle: (enabled: boolean) => void;
  onTest: () => void;
  onEdit: () => void;
  onReveal: () => void;
  onDelete: () => void;
}) {
  const test = testLabel(channel);
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <Dot tone={test.tone} />
          <span className="truncate text-[13px] font-medium">{channel.name}</span>
          <span className="rounded border border-(--border) bg-(--surface-secondary) px-1.5 py-0.5 text-[11px] text-(--muted)">
            {serviceLabel(channel.service ?? "")}
          </span>
        </div>
        <p className="mt-1 text-xs text-(--muted)">
          {test.label}
          {channel.last_tested_at && (
            <>
              <span> · </span>
              <span className="font-mono tabular-nums">
                {relTimeFromISO(channel.last_tested_at, Date.now())}
              </span>
            </>
          )}
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <Button size="sm" variant="ghost" isDisabled={testing} isPending={testing} onPress={onTest}>
          {!testing && <Play className="size-3.5" aria-hidden />}
          {testing ? m.notifications_testing() : m.notifications_test()}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          isDisabled={!canReveal || revealing}
          isPending={revealing}
          onPress={onReveal}
        >
          {!revealing && <Eye className="size-3.5" aria-hidden />}
          {revealing ? m.notifications_revealing() : m.notifications_reveal()}
        </Button>
        <Button size="sm" variant="ghost" onPress={onEdit}>
          <Pencil className="size-3.5" aria-hidden />
          {m.notifications_edit()}
        </Button>
        <Switch
          aria-label={channel.enabled ? m.notifications_enabled() : m.notifications_disabled()}
          isSelected={channel.enabled ?? false}
          isDisabled={toggling}
          onChange={onToggle}
        >
          <Switch.Content>
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
            <span className="text-xs text-(--muted)">
              {channel.enabled ? m.notifications_enabled() : m.notifications_disabled()}
            </span>
          </Switch.Content>
        </Switch>
        <Button size="sm" variant="ghost" onPress={onDelete} aria-label={m.common_delete()}>
          <TrashBin className="size-3.5 text-(--danger)" aria-hidden />
        </Button>
      </div>
    </div>
  );
}

function ChannelFormModal({
  channel,
  isOpen,
  onOpenChange,
  pending,
  error,
  onSubmit,
}: {
  channel?: NotificationChannel | null;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  onSubmit: (body: NotificationChannelCreateRequest | NotificationChannelUpdateRequest) => void;
}) {
  const isNew = !channel;
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [enabled, setEnabled] = useState(false);

  useEffect(() => {
    if (!isOpen) {
      setName("");
      setURL("");
      setEnabled(false);
      return;
    }
    setName(channel?.name ?? "");
    setURL("");
    setEnabled(channel?.enabled ?? false);
  }, [channel, isOpen]);

  function submit(event: FormEvent) {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName || (isNew && !url.trim())) return;
    if (isNew) {
      onSubmit({ name: trimmedName, url: url.trim(), enabled });
      return;
    }
    onSubmit({ name: trimmedName, ...(url.trim() ? { url: url.trim() } : {}), enabled });
  }

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form onSubmit={submit}>
            <Modal.Header>
              <Modal.Heading>
                {isNew ? m.notifications_create_title() : m.notifications_edit_title()}
              </Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-(--muted)">
                {m.notifications_form_description()}
              </p>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <TextField value={name} onChange={setName} autoFocus>
                  <Label>{m.notifications_name_label()}</Label>
                  <Input autoComplete="off" maxLength={128} />
                </TextField>
                <TextField value={url} onChange={setURL}>
                  <Label>
                    {isNew ? m.notifications_url_label() : m.notifications_url_replace_label()}
                  </Label>
                  <Input autoComplete="off" spellCheck={false} inputMode="url" />
                </TextField>
                {!isNew && (
                  <p className="-mt-2 text-xs text-(--muted)">
                    {m.notifications_url_replace_hint()}
                  </p>
                )}
                <LabeledSwitch
                  label={m.notifications_enabled_label()}
                  description={m.notifications_enabled_desc()}
                  isSelected={enabled}
                  onChange={setEnabled}
                />
              </div>
              {error && (
                <p className="mt-4 text-[13px] text-(--danger)" role="alert">
                  {error}
                </p>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {m.common_cancel()}
              </Button>
              <Button
                type="submit"
                variant="primary"
                isPending={pending}
                isDisabled={pending || !name.trim() || (isNew && !url.trim())}
              >
                {pending
                  ? m.notifications_saving()
                  : isNew
                    ? m.notifications_create_submit()
                    : m.notifications_save()}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function RevealModal({
  revealed,
  onOpenChange,
}: {
  revealed: { name: string; url: string } | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Modal.Backdrop isOpen={revealed !== null} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{m.notifications_reveal_title()}</Modal.Heading>
            <p className="mt-1.5 text-sm leading-5 text-(--muted)">
              {m.notifications_reveal_description()}
            </p>
          </Modal.Header>
          <Modal.Body>
            <p className="mb-2 text-[13px] font-medium">{revealed?.name}</p>
            {revealed?.url && <CopyableCode value={revealed.url} label={m.common_copy_config()} />}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="primary" onPress={() => onOpenChange(false)}>
              {m.notifications_reveal_clear()}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function testLabel(channel: NotificationChannel): { label: string; tone: "ok" | "error" | "idle" } {
  switch (channel.last_test_status) {
    case "succeeded":
      return { label: m.notifications_test_succeeded(), tone: "ok" };
    case "failed":
      return {
        label:
          channel.last_test_error === "timed_out"
            ? m.notifications_test_timed_out()
            : channel.last_test_error === "delivery_failed"
              ? m.notifications_test_delivery_failed()
              : m.notifications_test_failed(),
        tone: "error",
      };
    default:
      return { label: m.notifications_test_never(), tone: "idle" };
  }
}

function serviceLabel(service: string): string {
  const labels: Record<string, () => string> = {
    generic: m.notifications_service_generic,
    bark: m.notifications_service_bark,
    discord: m.notifications_service_discord,
    gotify: m.notifications_service_gotify,
    googlechat: m.notifications_service_googlechat,
    ifttt: m.notifications_service_ifttt,
    join: m.notifications_service_join,
    lark: m.notifications_service_lark,
    mattermost: m.notifications_service_mattermost,
    matrix: m.notifications_service_matrix,
    mqtt: m.notifications_service_mqtt,
    ntfy: m.notifications_service_ntfy,
    opsgenie: m.notifications_service_opsgenie,
    pushbullet: m.notifications_service_pushbullet,
    pushover: m.notifications_service_pushover,
    rocketchat: m.notifications_service_rocketchat,
    signal: m.notifications_service_signal,
    slack: m.notifications_service_slack,
    teams: m.notifications_service_teams,
    telegram: m.notifications_service_telegram,
    twilio: m.notifications_service_twilio,
    wecom: m.notifications_service_wecom,
    zulip: m.notifications_service_zulip,
  };
  return labels[service]?.() ?? service;
}
