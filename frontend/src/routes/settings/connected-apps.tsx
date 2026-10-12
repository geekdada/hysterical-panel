import { useState, type Key } from "react";
import { useForm } from "@tanstack/react-form";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  Button,
  Dropdown,
  Input,
  Label,
  Modal,
  Separator,
  Switch,
  TextArea,
  TextField,
} from "@heroui/react";
import { ArrowRotateRight, Ellipsis, Pencil, Plus, TrashBin } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  canQueryPanelApi,
  createOIDCClient,
  deleteOIDCClient,
  fetchOIDCClients,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  rotateOIDCClientSecret,
  updateOIDCClient,
  type OIDCClient,
  type OIDCClientCreateRequest,
  type OIDCClientSecretResponse,
  type OIDCClientUpdateRequest,
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
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/connected-apps")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.oidc_clients_title(),
  }),
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData({
      queryKey: queryKeys.oidcClients(),
      queryFn: fetchOIDCClients,
    });
  },
  component: ConnectedAppsPage,
});

function ConnectedAppsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const now = useHydratedNow();
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<OIDCClient | null>(null);
  const [deleting, setDeleting] = useState<OIDCClient | null>(null);
  const [rotating, setRotating] = useState<OIDCClient | null>(null);
  const [credentials, setCredentials] = useState<OIDCClientSecretResponse | null>(null);

  const clientsQuery = useQuery({ queryKey: queryKeys.oidcClients(), queryFn: fetchOIDCClients });
  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi(),
  });

  const invalidate = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.oidcClients() });

  const createMutation = useMutation({
    mutationFn: createOIDCClient,
    onSuccess: (data) => {
      setCreateOpen(false);
      setCredentials(data);
      invalidate();
    },
  });
  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: string; body: OIDCClientUpdateRequest }) =>
      updateOIDCClient(id, body),
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
  });
  // Row switches get their own mutation so a toggle never shares pending
  // state, errors or the close-on-success with the edit modal.
  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      updateOIDCClient(id, { enabled }),
    onSuccess: invalidate,
  });
  const rotateMutation = useMutation({
    mutationFn: rotateOIDCClientSecret,
    onSuccess: (data) => {
      setRotating(null);
      setCredentials(data);
      invalidate();
    },
  });
  const deleteMutation = useMutation({
    mutationFn: deleteOIDCClient,
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
  });

  const clients = clientsQuery.data ?? [];
  const issuer = settingsQuery.data?.oidc_issuer ?? "";
  const providerOff = settingsQuery.data !== undefined && !settingsQuery.data.oidc_enabled;
  const loadError = clientsQuery.error ? queryErrorMessage(clientsQuery.error) : "";
  const toggleError = toggleMutation.error
    ? queryErrorMessage(toggleMutation.error, m.error_oidc_client_save_network())
    : "";

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

  function closeRotate(open: boolean) {
    if (!open) {
      setRotating(null);
      rotateMutation.reset();
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
      <div className="mb-6">
        <h1 className="text-base font-semibold tracking-tight">{m.settings_oidc()}</h1>
        <p className="mt-0.5 max-w-2xl text-[13px] text-muted">{m.settings_oidc_desc()}</p>
      </div>

      {providerOff && (
        <div className="mb-4 rounded-lg border bg-surface-secondary px-4 py-3 text-[13px] text-muted">
          {m.oidc_clients_provider_off()}{" "}
          <Link to="/settings" className="font-medium text-accent hover:opacity-80">
            {m.nav_settings()}
          </Link>
        </div>
      )}

      {issuer && (
        <div className="mb-6">
          <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
            {m.oidc_clients_discovery()}
          </p>
          <CopyableCode
            value={`${issuer}/.well-known/openid-configuration`}
            label={m.oidc_copy_discovery()}
          />
        </div>
      )}

      <ErrorAlert message={loadError || toggleError} className="mb-4" />

      <ClientFormModal
        key={createOpen ? "create-open" : "create-closed"}
        isOpen={createOpen}
        onOpenChange={openCreate}
        pending={createMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(createMutation.error, m.error_oidc_client_save_network())
            : ""
        }
        onSubmit={(body) => createMutation.mutate(body)}
      />
      <ClientFormModal
        key={editing ? `edit-${editing.id}` : "edit-closed"}
        client={editing}
        isOpen={editing !== null}
        onOpenChange={closeEdit}
        pending={updateMutation.isPending}
        error={
          updateMutation.error
            ? queryErrorMessage(updateMutation.error, m.error_oidc_client_save_network())
            : ""
        }
        onSubmit={(body) => editing && updateMutation.mutate({ id: editing.id, body })}
      />
      <DestructiveConfirmModal
        isOpen={rotating !== null}
        title={m.oidc_clients_rotate_title()}
        body={m.oidc_clients_rotate_body({ name: rotating?.name ?? "" })}
        confirmLabel={m.oidc_clients_rotate_secret()}
        pendingLabel={m.oidc_clients_rotating()}
        pending={rotateMutation.isPending}
        error={
          rotateMutation.error
            ? queryErrorMessage(rotateMutation.error, m.error_oidc_client_rotate_network())
            : ""
        }
        onOpenChange={closeRotate}
        onConfirm={() => rotating && rotateMutation.mutate(rotating.id)}
      />
      <DestructiveConfirmModal
        isOpen={deleting !== null}
        title={m.oidc_clients_delete_title()}
        body={m.oidc_clients_delete_confirm({ name: deleting?.name ?? "" })}
        confirmLabel={m.common_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={
          deleteMutation.error
            ? queryErrorMessage(deleteMutation.error, m.error_oidc_client_delete_network())
            : ""
        }
        onOpenChange={closeDelete}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
      <CredentialsModal credentials={credentials} onClose={() => setCredentials(null)} />

      <Section
        className="mt-0"
        title={m.oidc_clients_title()}
        action={
          <Button size="sm" variant="secondary" onPress={() => setCreateOpen(true)}>
            <Plus className="size-3.5" aria-hidden />
            {m.oidc_clients_new()}
          </Button>
        }
      >
        {clientsQuery.isPending ? (
          <TableSkeleton />
        ) : clients.length === 0 ? (
          <PanelMessage>
            <span className="block font-medium text-foreground">
              {m.oidc_clients_empty_title()}
            </span>
            <span className="mt-1 block">{m.oidc_clients_empty_hint()}</span>
          </PanelMessage>
        ) : (
          <div className="divide-y divide-separator">
            {clients.map((client) => (
              <ClientRow
                key={client.id}
                client={client}
                now={now}
                toggling={toggleMutation.isPending && toggleMutation.variables?.id === client.id}
                onToggle={(enabled) => toggleMutation.mutate({ id: client.id, enabled })}
                onEdit={() => setEditing(client)}
                onRotate={() => setRotating(client)}
                onDelete={() => setDeleting(client)}
              />
            ))}
          </div>
        )}
      </Section>
    </PageShell>
  );
}

const menuItemClass =
  "h-7 rounded-md px-2 text-[13px] text-foreground transition-colors duration-150 hover:bg-surface-secondary data-[focus=true]:bg-surface-secondary data-[hovered=true]:bg-surface-secondary";
const menuDangerItemClass =
  "h-7 rounded-md px-2 text-[13px] text-danger transition-colors duration-150 hover:bg-danger-soft data-[focus=true]:bg-danger-soft data-[hovered=true]:bg-danger-soft";
const menuIconClass = "size-3.5 shrink-0 text-muted";

function ClientRow({
  client,
  now,
  toggling,
  onToggle,
  onEdit,
  onRotate,
  onDelete,
}: {
  client: OIDCClient;
  now: number | null;
  toggling: boolean;
  onToggle: (enabled: boolean) => void;
  onEdit: () => void;
  onRotate: () => void;
  onDelete: () => void;
}) {
  const stateLabel = client.enabled ? m.oidc_clients_enabled() : m.oidc_clients_disabled();
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone={client.enabled ? "ok" : "idle"} />
          <span className="truncate text-[13px] font-medium">{client.name}</span>
        </div>
        <p className="mt-1 truncate font-mono text-xs text-muted">{client.client_id}</p>
        <p className="mt-0.5 text-xs text-muted">
          {m.oidc_clients_redirect_count({ count: client.redirect_uris.length })}
          {client.last_used_at && now !== null && (
            <>
              <span> · </span>
              <span className="tabular-nums">
                {m.oidc_clients_last_used({ time: relTimeFromISO(client.last_used_at, now) })}
              </span>
            </>
          )}
        </p>
      </div>
      <div className="flex items-center justify-between gap-1.5 sm:flex-shrink-0">
        <Switch
          aria-label={stateLabel}
          isSelected={client.enabled}
          isDisabled={toggling}
          onChange={onToggle}
        >
          <Switch.Content>
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
            <span className="text-xs text-muted">{stateLabel}</span>
          </Switch.Content>
        </Switch>
        <Dropdown>
          <Button
            size="sm"
            variant="ghost"
            aria-label={m.oidc_clients_more_actions()}
            className="px-1.5"
          >
            <Ellipsis className="size-4 text-muted" aria-hidden />
          </Button>
          <Dropdown.Popover
            placement="bottom end"
            className="min-w-40 rounded-lg border bg-surface p-1 shadow-none"
          >
            <Dropdown.Menu
              onAction={(key: Key) => {
                if (key === "edit") onEdit();
                else if (key === "rotate") onRotate();
                else if (key === "delete") onDelete();
              }}
            >
              <Dropdown.Item id="edit" textValue={m.oidc_clients_edit()} className={menuItemClass}>
                <Pencil className={menuIconClass} aria-hidden />
                <Label className="truncate">{m.oidc_clients_edit()}</Label>
              </Dropdown.Item>
              <Dropdown.Item
                id="rotate"
                textValue={m.oidc_clients_rotate_secret()}
                className={menuItemClass}
              >
                <ArrowRotateRight className={menuIconClass} aria-hidden />
                <Label className="truncate">{m.oidc_clients_rotate_secret()}</Label>
              </Dropdown.Item>
              <Separator className="my-1 bg-separator" />
              <Dropdown.Item
                id="delete"
                variant="danger"
                textValue={m.common_delete()}
                className={menuDangerItemClass}
              >
                <TrashBin className="size-3.5 shrink-0 text-current" aria-hidden />
                <Label className="truncate">{m.common_delete()}</Label>
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
      </div>
    </div>
  );
}

function splitLines(value: string): string[] {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function ClientFormModal({
  client,
  isOpen,
  onOpenChange,
  pending,
  error,
  onSubmit,
}: {
  client?: OIDCClient | null;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  onSubmit: (body: OIDCClientCreateRequest) => void;
}) {
  const isNew = !client;
  const form = useForm({
    defaultValues: {
      name: client?.name ?? "",
      redirectURIs: (client?.redirect_uris ?? []).join("\n"),
      shareEmail: client ? client.allowed_scopes.includes("email") : true,
      refreshEnabled: client?.refresh_enabled ?? false,
      skipConsent: client?.skip_consent ?? false,
      enabled: client?.enabled ?? true,
    },
    onSubmit: ({ value }) => {
      onSubmit({
        name: value.name.trim(),
        redirect_uris: splitLines(value.redirectURIs),
        allowed_scopes: value.shareEmail ? ["openid", "email"] : ["openid"],
        refresh_enabled: value.refreshEnabled,
        skip_consent: value.skipConsent,
        enabled: value.enabled,
      });
    },
  });

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="md" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(event) => {
              event.preventDefault();
              event.stopPropagation();
              void form.handleSubmit();
            }}
          >
            <Modal.Header>
              <Modal.Heading>
                {isNew ? m.oidc_clients_create_title() : m.oidc_clients_edit_title()}
              </Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-muted">
                {m.oidc_clients_form_description()}
              </p>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <form.Field name="name">
                  {(field) => (
                    <TextField
                      value={field.state.value}
                      onChange={field.handleChange}
                      onBlur={field.handleBlur}
                      autoFocus
                    >
                      <Label>{m.oidc_clients_name_label()}</Label>
                      <Input autoComplete="off" maxLength={128} />
                    </TextField>
                  )}
                </form.Field>
                <form.Field name="redirectURIs">
                  {(field) => (
                    <TextField
                      value={field.state.value}
                      onChange={field.handleChange}
                      onBlur={field.handleBlur}
                    >
                      <Label>{m.oidc_clients_redirect_label()}</Label>
                      <TextArea
                        rows={3}
                        spellCheck={false}
                        className="font-mono text-xs"
                        style={{ resize: "vertical" }}
                      />
                    </TextField>
                  )}
                </form.Field>
                <p className="-mt-2 text-xs text-muted">{m.oidc_clients_redirect_hint()}</p>
                <form.Field name="shareEmail">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_scope_email_label()}
                      description={m.oidc_clients_scope_email_desc()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
                <form.Field name="refreshEnabled">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_refresh_label()}
                      description={m.oidc_clients_refresh_desc()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
                <form.Field name="skipConsent">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_skip_consent_label()}
                      description={m.oidc_clients_skip_consent_desc()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
                <form.Field name="enabled">
                  {(field) => (
                    <LabeledSwitch
                      label={m.oidc_clients_enabled_label()}
                      isSelected={field.state.value}
                      onChange={field.handleChange}
                    />
                  )}
                </form.Field>
              </div>
              {error && (
                <p className="mt-4 text-[13px] text-danger" role="alert">
                  {error}
                </p>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {m.common_cancel()}
              </Button>
              <form.Subscribe
                selector={(state) => ({
                  name: state.values.name,
                  redirectURIs: state.values.redirectURIs,
                })}
              >
                {({ name, redirectURIs }) => (
                  <Button
                    type="submit"
                    variant="primary"
                    isPending={pending}
                    isDisabled={pending || !name.trim() || splitLines(redirectURIs).length === 0}
                  >
                    {pending
                      ? m.oidc_clients_saving()
                      : isNew
                        ? m.oidc_clients_create_submit()
                        : m.oidc_clients_save()}
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

// The secret exists only in this response, so only the Done button closes
// the modal. A stray click outside or Escape would lose the secret.
function CredentialsModal({
  credentials,
  onClose,
}: {
  credentials: OIDCClientSecretResponse | null;
  onClose: () => void;
}) {
  return (
    <Modal.Backdrop
      isOpen={credentials !== null}
      onOpenChange={(open) => !open && onClose()}
      isDismissable={false}
      isKeyboardDismissDisabled
    >
      <Modal.Container size="md" placement="auto">
        <Modal.Dialog>
          <Modal.Header>
            <Modal.Heading>{m.oidc_clients_credentials_title()}</Modal.Heading>
          </Modal.Header>
          <Modal.Body>
            <div className="flex flex-col gap-4">
              <p className="text-[13px] font-medium">{credentials?.client.name}</p>
              <div>
                <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
                  {m.oidc_clients_client_id()}
                </p>
                <CopyableCode
                  value={credentials?.client.client_id ?? ""}
                  label={m.oidc_copy_client_id()}
                />
              </div>
              <div>
                <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">
                  {m.oidc_clients_client_secret()}
                </p>
                <CopyableCode
                  value={credentials?.client_secret ?? ""}
                  label={m.oidc_copy_client_secret()}
                />
              </div>
              <div className="rounded-lg border border-warning bg-warning-soft px-4 py-3">
                <p className="text-[13px] font-semibold text-warning-soft-foreground">
                  {m.oidc_clients_secret_warning()}
                </p>
              </div>
            </div>
          </Modal.Body>
          <Modal.Footer>
            <Button variant="primary" onPress={onClose}>
              {m.oidc_clients_credentials_done()}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
