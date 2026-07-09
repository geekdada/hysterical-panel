import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Label, ListBox, Select } from "@heroui/react";
import { Bell, ChevronRight, Code, Database, Xmark } from "@gravity-ui/icons";
import {
  deletePasskey,
  isPasskeySoftError,
  listPasskeys,
  registerPasskey,
  type Passkey,
} from "~/api/auth";
import {
  canQueryPanelApi,
  deleteIgnoredConnectionIP,
  fetchIgnoredConnectionIPs,
  fetchPanelConfigQuery,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  rotateManagementApiToken,
  updateSettings,
  type AppSettings,
  type SettingsUpdateRequest,
} from "~/api/queries";
import {
  BrandLink,
  CopyableCode,
  DestructiveConfirmModal,
  ErrorAlert,
  LabeledSwitch,
  PageShell,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { relTimeFromISO } from "~/lib/format";
import { offsetLabel, SYSTEM_TIMEZONE_ID, TIMEZONE_OPTIONS } from "~/lib/timezone";
import { cn } from "~/lib/cn";
import { useTimezonePreference } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/")({
  component: SettingsPage,
});

function SettingsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const isAdmin = auth?.user.role === "admin";

  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi() && isAdmin,
  });

  const mutation = useMutation({
    mutationFn: (patch: SettingsUpdateRequest) => updateSettings(patch),
    onSuccess: (data) => {
      queryClient.setQueryData(queryKeys.settings(), data);
      void queryClient.invalidateQueries({ queryKey: queryKeys.config() });
    },
  });

  const settings = settingsQuery.data;
  const loadError = settingsQuery.error ? queryErrorMessage(settingsQuery.error) : "";
  const saveError = mutation.error
    ? queryErrorMessage(mutation.error, m.error_settings_update_network())
    : "";

  function patch(field: keyof AppSettings, value: boolean) {
    mutation.mutate({ [field]: value });
  }

  // The three registration flags form a strict hierarchy (open_registration >
  // invitations_enabled > require_invite_for_open). Turning a parent off must
  // cascade its descendants off in the same request so the backend never sees
  // an invalid nested state.
  function patchOpenRegistration(value: boolean) {
    mutation.mutate(
      value
        ? { open_registration: true }
        : { open_registration: false, invitations_enabled: false, require_invite_for_open: false }
    );
  }

  function patchInvitations(value: boolean) {
    mutation.mutate(
      value
        ? { invitations_enabled: true }
        : { invitations_enabled: false, require_invite_for_open: false }
    );
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-5">
        <h1 className="text-base font-semibold tracking-tight">{m.settings_timezone()}</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">{m.settings_timezone_desc()}</p>
      </div>

      <div className="rounded-(--radius) border border-(--border) bg-(--surface) p-5">
        <TimezoneSetting />
      </div>

      {auth && <PasskeysSection userId={auth.user.id} />}

      {isAdmin && (
        <>
          <div className="mt-8 mb-5">
            <h1 className="text-base font-semibold tracking-tight">{m.settings_registration()}</h1>
            <p className="mt-0.5 text-[13px] text-(--muted)">{m.settings_registration_desc()}</p>
          </div>

          <ErrorAlert message={loadError} className="mb-4" />

          <div className="flex flex-col gap-5 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
            <LabeledSwitch
              label={m.settings_open_registration()}
              description={m.settings_open_registration_desc()}
              isSelected={settings?.open_registration ?? false}
              isDisabled={!settings || mutation.isPending}
              onChange={patchOpenRegistration}
            />
            <LabeledSwitch
              label={m.settings_invitation_system()}
              description={m.settings_invitation_system_desc()}
              isSelected={settings?.invitations_enabled ?? false}
              isDisabled={
                !settings || mutation.isPending || !(settings?.open_registration ?? false)
              }
              onChange={patchInvitations}
            />
            <LabeledSwitch
              label={m.settings_require_invite()}
              description={m.settings_require_invite_desc()}
              isSelected={settings?.require_invite_for_open ?? false}
              isDisabled={
                !settings || mutation.isPending || !(settings?.invitations_enabled ?? false)
              }
              onChange={(v) => patch("require_invite_for_open", v)}
            />
          </div>

          {settings?.open_registration && !settings.require_invite_for_open && (
            <p className="mt-3 text-xs text-(--muted)">{m.settings_smtp_note()}</p>
          )}

          <IgnoredConnectionIPsSection />

          <div className="mt-8 mb-5">
            <h1 className="text-base font-semibold tracking-tight">
              {m.settings_notification_channels()}
            </h1>
            <p className="mt-0.5 text-[13px] text-(--muted)">
              {m.settings_notification_channels_desc()}
            </p>
          </div>

          <Link
            to="/settings/notifications"
            className="group flex items-center gap-3 rounded-(--radius) border border-(--border) bg-(--surface) px-4 py-3.5 transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
          >
            <span className="grid size-8 shrink-0 place-items-center rounded-(--radius) border border-(--border) bg-(--surface-secondary) text-(--muted)">
              <Bell className="size-4" aria-hidden />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-medium text-(--foreground)">
                {m.settings_notification_channels_manage()}
              </span>
              <span className="block text-xs text-(--muted)">
                {m.settings_notification_channels_manage_desc()}
              </span>
            </span>
            <ChevronRight
              className="size-4 shrink-0 text-(--muted) transition-colors duration-150 group-hover:text-(--foreground)"
              aria-hidden
            />
          </Link>

          <div className="mt-8 mb-5">
            <h1 className="text-base font-semibold tracking-tight">{m.settings_database()}</h1>
            <p className="mt-0.5 text-[13px] text-(--muted)">{m.settings_database_desc()}</p>
          </div>

          <Link
            to="/settings/database"
            className="group flex items-center gap-3 rounded-(--radius) border border-(--border) bg-(--surface) px-4 py-3.5 transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
          >
            <span className="grid size-8 shrink-0 place-items-center rounded-(--radius) border border-(--border) bg-(--surface-secondary) text-(--muted)">
              <Database className="size-4" aria-hidden />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-medium text-(--foreground)">
                {m.settings_database_management()}
              </span>
              <span className="block text-xs text-(--muted)">
                {m.settings_database_management_desc()}
              </span>
            </span>
            <ChevronRight
              className="size-4 shrink-0 text-(--muted) transition-colors duration-150 group-hover:text-(--foreground)"
              aria-hidden
            />
          </Link>

          <ManagementApiSection
            settings={settings}
            pending={mutation.isPending}
            onSave={(patch) => mutation.mutate(patch)}
            error={saveError}
            newToken={mutation.data?.management_api_token}
          />
        </>
      )}
    </PageShell>
  );
}

// Self-service passkey management for the signed-in user. Adding a passkey runs
// WebAuthn registration in this browser, so it's inherently scoped to the
// current account — admins manage their own keys here like anyone else, and
// revoking another user's key is left to the PocketBase admin. Hidden entirely
// when passkeys are disabled platform-wide.
function PasskeysSection({ userId }: { userId: string }) {
  const queryClient = useQueryClient();
  const passkeysKey = ["panel", "users", userId, "passkeys"] as const;
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [pendingPasskey, setPendingPasskey] = useState<Passkey | null>(null);

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
    mutationFn: (passkeyId: string) => deletePasskey(userId, passkeyId),
    onSuccess: () => {
      setDeleteOpen(false);
      void queryClient.invalidateQueries({ queryKey: passkeysKey });
    },
  });

  function handleDeleteOpenChange(open: boolean) {
    setDeleteOpen(open);
    if (!open) {
      setPendingPasskey(null);
      deleteMutation.reset();
    }
  }

  function handleDeleteRequest(passkey: Passkey) {
    setPendingPasskey(passkey);
    setDeleteOpen(true);
  }

  function handleDeleteConfirm() {
    if (pendingPasskey?.id) deleteMutation.mutate(pendingPasskey.id);
  }

  if (configQuery.isPending) {
    return <PasskeysSectionSkeleton />;
  }

  if (!enabled) return null;

  const rows = passkeysQuery.data ?? [];
  const loading = passkeysQuery.isPending;
  // A cancelled WebAuthn prompt is a normal outcome, not an error to surface.
  const addError =
    addMutation.error && !isPasskeySoftError(addMutation.error)
      ? queryErrorMessage(addMutation.error, m.error_passkey_add())
      : "";
  const deleteError = deleteMutation.error
    ? queryErrorMessage(deleteMutation.error, m.error_passkey_delete())
    : "";
  const error = passkeysQuery.error ? queryErrorMessage(passkeysQuery.error) : addError;
  const deletingId =
    deleteMutation.isPending && deleteMutation.variables ? deleteMutation.variables : undefined;
  const pendingPasskeyName = pendingPasskey?.name || m.user_passkeys_default_name();

  return (
    <>
      <PasskeysSectionHeader />

      <div className="overflow-hidden rounded-(--radius) border border-(--border) bg-(--surface)">
        <div className="flex items-center justify-between gap-3 px-5 py-3">
          {loading ? (
            <span
              className="inline-block h-3.5 w-20 animate-pulse rounded bg-(--surface-secondary)"
              aria-hidden
            />
          ) : (
            <span className="text-[13px] text-(--muted)">
              {m.user_passkeys_meta({ count: String(rows.length) })}
            </span>
          )}
          <Button
            size="sm"
            variant="secondary"
            isDisabled={addMutation.isPending || loading}
            onPress={() => addMutation.mutate()}
          >
            {addMutation.isPending ? m.user_passkeys_adding() : m.user_passkeys_add()}
          </Button>
        </div>

        {error && (
          <div
            className="border-t border-(--border) bg-(--danger-soft) px-5 py-2.5 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            {error}
          </div>
        )}

        {loading ? (
          <PasskeysListSkeleton />
        ) : rows.length === 0 ? (
          <p className="border-t border-(--border) px-5 py-4 text-[13px] text-(--muted)">
            {m.user_passkeys_empty_hint_self()}
          </p>
        ) : (
          <ul className="divide-y divide-(--separator) border-t border-(--border)">
            {rows.map((passkey) => (
              <PasskeyRow
                key={passkey.id}
                passkey={passkey}
                deleting={deletingId === passkey.id}
                onDelete={() => handleDeleteRequest(passkey)}
              />
            ))}
          </ul>
        )}
      </div>

      <DestructiveConfirmModal
        isOpen={deleteOpen}
        title={m.user_passkeys_delete_title()}
        body={m.user_passkeys_delete_confirm({ name: pendingPasskeyName })}
        confirmLabel={m.common_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={deleteError}
        onOpenChange={handleDeleteOpenChange}
        onConfirm={handleDeleteConfirm}
      />
    </>
  );
}

function PasskeysSectionHeader() {
  return (
    <div className="mt-8 mb-5">
      <h1 className="text-base font-semibold tracking-tight">{m.user_passkeys_title()}</h1>
      <p className="mt-0.5 text-[13px] text-(--muted)">{m.settings_passkeys_desc()}</p>
    </div>
  );
}

function PasskeysSectionSkeleton() {
  return (
    <>
      <PasskeysSectionHeader />
      <PasskeysCardSkeleton />
    </>
  );
}

function PasskeysCardSkeleton() {
  return (
    <div
      className="overflow-hidden rounded-(--radius) border border-(--border) bg-(--surface)"
      aria-hidden
    >
      <div className="flex items-center justify-between gap-3 px-5 py-3">
        <div className="h-3.5 w-20 animate-pulse rounded bg-(--surface-secondary)" />
        <div className="h-8 w-28 shrink-0 animate-pulse rounded-full bg-(--surface-secondary)" />
      </div>
      <PasskeysListSkeleton />
    </div>
  );
}

function PasskeysListSkeleton() {
  return (
    <div className="border-t border-(--border)" aria-hidden>
      <div className="flex items-center justify-between gap-3 px-5 py-3">
        <div className="min-w-0 space-y-1.5">
          <div className="h-3.5 w-24 animate-pulse rounded bg-(--surface-secondary)" />
          <div className="h-3 w-36 animate-pulse rounded bg-(--surface-secondary)" />
        </div>
        <div className="h-8 w-14 shrink-0 animate-pulse rounded-full bg-(--surface-secondary)" />
      </div>
    </div>
  );
}

function PasskeyRow({
  passkey,
  deleting,
  onDelete,
}: {
  passkey: Passkey;
  deleting: boolean;
  onDelete: () => void;
}) {
  const backup = passkey.backup_state
    ? m.user_passkeys_backup_synced()
    : passkey.backup_eligible
      ? m.user_passkeys_backup_eligible()
      : m.user_passkeys_backup_device_bound();
  const lastUsed = passkey.last_used_at
    ? relTimeFromISO(passkey.last_used_at, Date.now())
    : m.common_never();

  return (
    <li className="flex items-center justify-between gap-3 px-5 py-3">
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-(--foreground)">
          {passkey.name || m.user_passkeys_default_name()}
        </p>
        <p className="mt-0.5 text-xs text-(--muted)">
          {backup} · {lastUsed}
        </p>
      </div>
      <Button
        size="sm"
        variant="secondary"
        isDisabled={deleting}
        onPress={onDelete}
        className="border-(--danger) text-(--danger) hover:bg-(--danger-soft)"
      >
        {deleting ? m.common_deleting() : m.common_delete()}
      </Button>
    </li>
  );
}

// Available to every signed-in user (the page guard is requireAuth, not requireAdmin).
// Stored preference is null for "follow system"; the Select uses a sentinel id instead.
function TimezoneSetting() {
  const [pref, setPref] = useTimezonePreference();
  const value = pref ?? SYSTEM_TIMEZONE_ID;

  return (
    <Select
      className="w-full sm:max-w-xs"
      value={value}
      onChange={(key) => {
        const next = Array.isArray(key) ? key[0] : key;
        setPref(next == null || next === SYSTEM_TIMEZONE_ID ? null : String(next));
      }}
    >
      <Label>{m.settings_timezone_label()}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox className="max-h-72 overflow-y-auto">
          {TIMEZONE_OPTIONS.map((option) => {
            const text =
              option.offset === null
                ? m.settings_timezone_follow_system()
                : offsetLabel(option.offset);
            return (
              <ListBox.Item key={option.id} id={option.id} textValue={text}>
                {text}
                <ListBox.ItemIndicator />
              </ListBox.Item>
            );
          })}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}

function IgnoredConnectionIPsSection() {
  const queryClient = useQueryClient();
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const ignoredQuery = useQuery({
    queryKey: queryKeys.ignoredConnectionIPs(),
    queryFn: fetchIgnoredConnectionIPs,
    enabled: canQueryPanelApi(),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteIgnoredConnectionIP(id),
    onMutate: (id) => setDeletingId(id),
    onSettled: () => setDeletingId(null),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ignoredConnectionIPs() });
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.all, "users"] });
    },
  });

  const loadError = ignoredQuery.error ? queryErrorMessage(ignoredQuery.error) : "";
  const deleteError = deleteMutation.error
    ? queryErrorMessage(deleteMutation.error, m.error_ignored_ip_delete_network())
    : "";
  const items = ignoredQuery.data ?? [];

  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">{m.settings_ignored_ips()}</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">{m.settings_ignored_ips_desc()}</p>
      </div>

      <div className="rounded-(--radius) border border-(--border) bg-(--surface) p-5">
        {ignoredQuery.isPending ? (
          <IgnoredIPsSkeleton />
        ) : items.length === 0 ? (
          <p className="text-[13px] text-(--muted)">{m.settings_ignored_ips_empty()}</p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {items.map((item) => {
              if (!item.id) return null;
              const pending = deletingId === item.id;
              return (
                <Button
                  key={item.id}
                  variant="tertiary"
                  size="sm"
                  isDisabled={pending || deleteMutation.isPending}
                  aria-label={item.ip}
                  className="font-mono text-xs"
                  onPress={() => deleteMutation.mutate(item.id!)}
                >
                  {item.ip}
                  <Xmark className="size-3.5 shrink-0" aria-hidden />
                </Button>
              );
            })}
          </div>
        )}
      </div>

      <ErrorAlert message={loadError} className="mt-4" />
      <ErrorAlert message={deleteError} className="mt-4" />
    </>
  );
}

function IgnoredIPsSkeleton() {
  const widths = ["w-24", "w-28", "w-20"] as const;

  return (
    <div className="flex flex-wrap gap-2" aria-hidden>
      {widths.map((width) => (
        <div
          key={width}
          className={cn("h-8 animate-pulse rounded-full bg-(--surface-secondary)", width)}
        />
      ))}
    </div>
  );
}

function ManagementApiSection({
  settings,
  pending,
  error,
  onSave,
  newToken,
}: {
  settings: AppSettings | undefined;
  pending: boolean;
  error: string;
  onSave: (patch: SettingsUpdateRequest) => void;
  newToken?: string;
}) {
  const queryClient = useQueryClient();
  const [revealedToken, setRevealedToken] = useState<string | null>(null);
  const [rotateOpen, setRotateOpen] = useState(false);

  const enabled = settings?.management_api_enabled ?? false;
  const tokenSet = settings?.management_api_token_set ?? false;

  // Surface a freshly generated/rotated token exactly once.
  useEffect(() => {
    if (newToken) setRevealedToken(newToken);
  }, [newToken]);

  const rotateMutation = useMutation({
    mutationFn: () => rotateManagementApiToken(),
    onSuccess: (data) => {
      if (data.management_api_token) setRevealedToken(data.management_api_token);
      setRotateOpen(false);
      void queryClient.invalidateQueries({ queryKey: queryKeys.settings() });
    },
  });

  function toggleEnabled(v: boolean) {
    onSave({ management_api_enabled: v });
  }

  function handleRotateOpenChange(open: boolean) {
    setRotateOpen(open);
    if (!open) rotateMutation.reset();
  }

  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">{m.settings_management_api()}</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">{m.settings_management_api_desc()}</p>
      </div>

      <div className="flex flex-col gap-4 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
        <LabeledSwitch
          label={m.settings_enable_mgmt_api()}
          description={
            enabled ? m.settings_mgmt_api_enabled_desc() : m.settings_mgmt_api_disabled_desc()
          }
          isSelected={enabled}
          isDisabled={!settings || pending}
          onChange={toggleEnabled}
        />

        {tokenSet && (
          <>
            <div className="h-px bg-(--separator)" />

            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 text-[13px]">
                <span className="text-success font-semibold">{m.settings_configured()}</span>
              </div>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={pending || rotateMutation.isPending}
                onPress={() => setRotateOpen(true)}
                className="border-(--danger) text-(--danger) hover:bg-(--danger-soft)"
              >
                {rotateMutation.isPending ? m.settings_rotating() : m.settings_rotate_token()}
              </Button>
            </div>
          </>
        )}
      </div>

      <DestructiveConfirmModal
        isOpen={rotateOpen}
        title={m.settings_rotate_token_confirm_title()}
        body={m.settings_rotate_token_confirm_body()}
        confirmLabel={m.settings_rotate_token()}
        pendingLabel={m.settings_rotating()}
        pending={rotateMutation.isPending}
        error={
          rotateMutation.error
            ? queryErrorMessage(rotateMutation.error, m.error_mgmt_token_rotate_network())
            : ""
        }
        onOpenChange={handleRotateOpenChange}
        onConfirm={() => rotateMutation.mutate()}
      />

      {revealedToken && (
        <div className="mt-4 rounded-(--radius) border border-(--warning) bg-(--warning-soft) px-4 py-3">
          <p className="text-[13px] font-semibold text-(--warning-soft-foreground)">
            {m.settings_copy_token_now()}
          </p>
          <CopyableCode value={revealedToken} label="token" className="mt-2" />
        </div>
      )}

      <Link
        to="/settings/management-api"
        className="group mt-3 flex items-center gap-3 rounded-(--radius) border border-(--border) bg-(--surface) px-4 py-3.5 transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-(--radius) border border-(--border) bg-(--surface-secondary) text-(--muted)">
          <Code className="size-4" aria-hidden />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[13px] font-medium text-(--foreground)">
            {m.settings_api_reference()}
          </span>
          <span className="block text-xs text-(--muted)">{m.settings_api_reference_desc()}</span>
        </span>
        <ChevronRight
          className="size-4 shrink-0 text-(--muted) transition-colors duration-150 group-hover:text-(--foreground)"
          aria-hidden
        />
      </Link>

      <ErrorAlert message={error} className="mt-4" />
    </>
  );
}
