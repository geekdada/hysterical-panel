import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Description, Input, Label, NumberField, TextField } from "@heroui/react";
import { TrashBin } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  createInvitation,
  deleteInvitation,
  invitationsQueryOptions,
  queryErrorMessage,
  queryKeys,
  settingsQueryOptions,
  type Invitation,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  CopyButton,
  DestructiveConfirmModal,
  Dot,
  LabeledSwitch,
  PageShell,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatLocaleCount, relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/invitations")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.invitations_title(),
    href: "/invitations",
  }),
  loader: async ({ context }) => {
    markResponsePrivate();
    await Promise.allSettled([
      context.queryClient.ensureQueryData(settingsQueryOptions()),
      context.queryClient.ensureQueryData(invitationsQueryOptions()),
    ]);
  },
  component: InvitationsPage,
});

function InvitationsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const now = useHydratedNow(30_000);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [pendingInvitation, setPendingInvitation] = useState<{ id: string; code: string } | null>(
    null
  );

  const settingsQuery = useQuery(settingsQueryOptions());
  const invitationsQuery = useQuery(invitationsQueryOptions());

  const invalidate = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.invitations() });

  const createMutation = useMutation({ mutationFn: createInvitation, onSuccess: invalidate });
  const deleteMutation = useMutation({
    mutationFn: deleteInvitation,
    onSuccess: () => {
      setDeleteOpen(false);
      invalidate();
    },
  });

  const invitations = invitationsQuery.data ?? [];
  const invitationsEnabled = settingsQuery.data?.invitations_enabled ?? false;
  const listError = invitationsQuery.error ? queryErrorMessage(invitationsQuery.error) : "";
  const deleteError = deleteMutation.error
    ? queryErrorMessage(deleteMutation.error, m.error_invitation_delete_network())
    : "";

  function handleDeleteOpenChange(open: boolean) {
    setDeleteOpen(open);
    if (!open) {
      setPendingInvitation(null);
      deleteMutation.reset();
    }
  }

  function handleDeleteRequest(id: string, code: string) {
    setPendingInvitation({ id, code });
    setDeleteOpen(true);
  }

  function handleDeleteConfirm() {
    if (pendingInvitation?.id) deleteMutation.mutate(pendingInvitation.id);
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      {!settingsQuery.isPending && !invitationsEnabled && (
        <div className="mb-5 rounded-lg border bg-surface-secondary px-4 py-3 text-[13px] text-muted">
          {m.invitations_disabled_banner()}{" "}
          <Link to="/settings" className="font-medium text-accent hover:opacity-80">
            {m.invitations_enable_in_settings()}
          </Link>
          .
        </div>
      )}

      <CreateInvitationForm
        disabled={!invitationsEnabled}
        pending={createMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(createMutation.error, m.error_invitation_create_network())
            : ""
        }
        created={createMutation.data ?? null}
        onSubmit={(body) => createMutation.mutate(body)}
      />

      <Section
        title={m.invitations_section_title()}
        meta={
          invitations.length > 0
            ? m.invitations_total_meta({ count: formatLocaleCount(invitations.length) })
            : undefined
        }
      >
        {invitationsQuery.isPending ? (
          <TableSkeleton />
        ) : listError ? (
          <PanelMessage>{listError}</PanelMessage>
        ) : invitations.length === 0 ? (
          <PanelMessage>{m.invitations_no_invitations()}</PanelMessage>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b border-separator text-left">
                  <Th>{m.invitations_th_code()}</Th>
                  <Th>{m.common_email()}</Th>
                  <Th>{m.invitations_th_uses()}</Th>
                  <Th>{m.invitations_th_expires()}</Th>
                  <Th>{m.common_status()}</Th>
                  <Th className="text-right">{m.common_actions()}</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-separator">
                {invitations.map((inv) => (
                  <InvitationRow
                    key={inv.id}
                    inv={inv}
                    now={now}
                    onDelete={() => handleDeleteRequest(inv.id ?? "", inv.code ?? "")}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <DestructiveConfirmModal
        isOpen={deleteOpen}
        title={m.invitations_delete_title()}
        body={m.invitations_delete_confirm({
          code: pendingInvitation?.code ?? "",
        })}
        confirmLabel={m.common_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={deleteError}
        onOpenChange={handleDeleteOpenChange}
        onConfirm={handleDeleteConfirm}
      />
    </PageShell>
  );
}

function InvitationRow({
  inv,
  now,
  onDelete,
}: {
  inv: Invitation;
  now: number | null;
  onDelete: () => void;
}) {
  const used = inv.used_count ?? 0;
  const uses =
    inv.max_uses && inv.max_uses > 0
      ? m.invitations_uses_limited({ used: String(used), max: String(inv.max_uses) })
      : m.invitations_uses_unlimited({ used: String(used) });
  const status = inv.valid
    ? { tone: "ok" as const, label: m.common_active() }
    : { tone: "idle" as const, label: inv.invalid_reason || m.invitations_status_inactive() };

  return (
    <tr className="hover:bg-surface-secondary">
      <Td>
        <div className="group/key flex items-center gap-1.5">
          <span className="font-mono text-[12px]">{inv.code}</span>
          <CopyButton value={inv.code ?? ""} label={m.invitations_copy_invite_code()} />
        </div>
      </Td>
      <Td className="text-muted">{inv.email || m.common_em_dash()}</Td>
      <Td className="tabular-nums">{uses}</Td>
      <Td className="text-muted">
        {inv.expires_at ? relTimeFromISO(inv.expires_at, now) : m.common_never()}
      </Td>
      <Td>
        <span className="inline-flex items-center gap-1.5">
          <Dot tone={status.tone} />
          <span className={inv.valid ? "" : "text-muted capitalize"}>{status.label}</span>
        </span>
      </Td>
      <Td>
        <div className="flex items-center justify-end gap-1">
          <CopyButton value={inv.link ?? ""} label={m.invitations_copy_invite_link()} />
          <button
            type="button"
            onClick={onDelete}
            title={m.invitations_delete_title()}
            aria-label={m.invitations_delete_title()}
            className="inline-grid size-6 place-items-center rounded text-muted transition-colors duration-150 hover:text-danger focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
          >
            <TrashBin className="size-3.5" aria-hidden />
          </button>
        </div>
      </Td>
    </tr>
  );
}

function CreateInvitationForm({
  disabled,
  pending,
  error,
  created,
  onSubmit,
}: {
  disabled: boolean;
  pending: boolean;
  error: string;
  created: Invitation | null;
  onSubmit: (body: {
    email?: string;
    max_uses?: number;
    expires_in_hours?: number;
    note?: string;
    send_email?: boolean;
  }) => void;
}) {
  const [email, setEmail] = useState("");
  const [maxUses, setMaxUses] = useState(1);
  const [expiresInHours, setExpiresInHours] = useState(168);
  const [note, setNote] = useState("");
  const [sendEmail, setSendEmail] = useState(false);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    onSubmit({
      email: email.trim() || undefined,
      max_uses: maxUses,
      expires_in_hours: expiresInHours,
      note: note.trim() || undefined,
      send_email: sendEmail && email.trim().length > 0,
    });
  }

  return (
    <div className="rounded-lg border bg-surface p-5">
      <h2 className="text-[13px] font-semibold tracking-tight">{m.invitations_create_title()}</h2>
      <p className="mt-0.5 text-[13px] text-muted">{m.invitations_create_description()}</p>

      <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField className="sm:col-span-2">
            <Label>{m.invitations_label_email_optional()}</Label>
            <Input
              type="email"
              autoComplete="off"
              placeholder={m.invitations_placeholder_email()}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Description>{m.invitations_email_description()}</Description>
          </TextField>

          <NumberField value={maxUses} onChange={setMaxUses} minValue={0} step={1}>
            <Label>{m.invitations_label_max_uses()}</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
            <Description>{m.invitations_max_uses_description()}</Description>
          </NumberField>

          <NumberField value={expiresInHours} onChange={setExpiresInHours} minValue={0} step={1}>
            <Label>{m.invitations_label_expires_hours()}</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
            <Description>{m.invitations_expires_description()}</Description>
          </NumberField>

          <TextField className="sm:col-span-2">
            <Label>{m.invitations_label_note_optional()}</Label>
            <Input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder={m.invitations_placeholder_note()}
            />
          </TextField>
        </div>

        <LabeledSwitch
          label={m.invitations_label_email_invite()}
          description={m.invitations_email_invite_description()}
          isSelected={sendEmail}
          isDisabled={email.trim().length === 0}
          onChange={setSendEmail}
        />

        {error && (
          <p className="text-[13px] text-danger" role="alert">
            {error}
          </p>
        )}

        <div className="flex justify-end border-t border-separator pt-4">
          <Button type="submit" variant="primary" isDisabled={disabled || pending}>
            {pending ? m.invitations_creating() : m.invitations_create_button()}
          </Button>
        </div>
      </form>

      {created && (
        <div className="mt-4 rounded-lg border bg-surface-secondary p-3">
          <div className="flex items-center gap-2 text-[13px]">
            <Dot tone="ok" />
            <span className="font-medium">{m.invitations_created()}</span>
            {created.email_sent === true && (
              <span className="text-xs text-muted">
                {m.invitations_emailed_to({ email: created.email ?? "" })}
              </span>
            )}
            {created.email_sent === false && (
              <span className="text-xs text-warning">{m.invitations_email_not_sent()}</span>
            )}
          </div>
          <div className="group/key mt-2 flex items-center gap-1.5">
            <span className="min-w-0 truncate font-mono text-[12px] text-muted">
              {created.link}
            </span>
            <CopyButton value={created.link ?? ""} label={m.invitations_copy_invite_link()} />
          </div>
        </div>
      )}
    </div>
  );
}
