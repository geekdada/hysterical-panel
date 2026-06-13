import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Description, Input, Label, NumberField, Switch, TextField } from "@heroui/react";
import { TrashBin } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  canQueryPanelApi,
  createInvitation,
  deleteInvitation,
  fetchInvitations,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  REFRESH_MS,
  type Invitation,
} from "~/api/queries";
import {
  BackLink,
  CopyButton,
  Dot,
  PageShell,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { relTimeFromISO } from "~/lib/format";

export const Route = createFileRoute("/invitations")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: InvitationsPage,
});

function InvitationsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const now = Date.now();

  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi(),
  });
  const invitationsQuery = useQuery({
    queryKey: queryKeys.invitations(),
    queryFn: fetchInvitations,
    enabled: canQueryPanelApi(),
    refetchInterval: REFRESH_MS,
  });

  const invalidate = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.invitations() });

  const createMutation = useMutation({ mutationFn: createInvitation, onSuccess: invalidate });
  const deleteMutation = useMutation({ mutationFn: deleteInvitation, onSuccess: invalidate });

  const invitations = invitationsQuery.data ?? [];
  const invitationsEnabled = settingsQuery.data?.invitations_enabled ?? false;
  const listError = invitationsQuery.error ? queryErrorMessage(invitationsQuery.error) : "";

  function handleDelete(id: string, code: string) {
    if (
      window.confirm(`Delete invitation ${code}? People who already have it can no longer use it.`)
    ) {
      deleteMutation.mutate(id);
    }
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={
        <div className="flex min-w-0 items-center gap-3">
          <BackLink />
          <span className="truncate text-[13px] font-semibold tracking-tight">Invitations</span>
        </div>
      }
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      {!settingsQuery.isPending && !invitationsEnabled && (
        <div className="mb-5 rounded-(--radius) border border-(--border) bg-(--surface-secondary) px-4 py-3 text-[13px] text-(--muted)">
          The invitation system is off, so new codes can't be created or used.{" "}
          <Link to="/settings" className="font-medium text-(--accent) hover:opacity-80">
            Enable it in Settings
          </Link>
          .
        </div>
      )}

      <CreateInvitationForm
        disabled={!invitationsEnabled}
        pending={createMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(
                createMutation.error,
                "Network error while creating the invitation."
              )
            : ""
        }
        created={createMutation.data ?? null}
        onSubmit={(body) => createMutation.mutate(body)}
      />

      <Section
        title="Invitations"
        meta={invitations.length > 0 ? `${invitations.length} total` : undefined}
      >
        {invitationsQuery.isPending ? (
          <TableSkeleton />
        ) : listError ? (
          <PanelMessage>{listError}</PanelMessage>
        ) : invitations.length === 0 ? (
          <PanelMessage>No invitations yet.</PanelMessage>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b border-(--separator) text-left">
                  <Th>Code</Th>
                  <Th>Email</Th>
                  <Th>Uses</Th>
                  <Th>Expires</Th>
                  <Th>Status</Th>
                  <Th className="text-right">Actions</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--separator)">
                {invitations.map((inv) => (
                  <InvitationRow
                    key={inv.id}
                    inv={inv}
                    now={now}
                    onDelete={() => handleDelete(inv.id ?? "", inv.code ?? "")}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </PageShell>
  );
}

function InvitationRow({
  inv,
  now,
  onDelete,
}: {
  inv: Invitation;
  now: number;
  onDelete: () => void;
}) {
  const used = inv.used_count ?? 0;
  const uses = inv.max_uses && inv.max_uses > 0 ? `${used} / ${inv.max_uses}` : `${used} / ∞`;
  const status = inv.valid
    ? { tone: "ok" as const, label: "Active" }
    : { tone: "idle" as const, label: inv.invalid_reason || "Inactive" };

  return (
    <tr className="hover:bg-(--surface-secondary)">
      <Td>
        <div className="group/key flex items-center gap-1.5">
          <span className="font-mono text-[12px]">{inv.code}</span>
          <CopyButton value={inv.code ?? ""} label="invite code" />
        </div>
      </Td>
      <Td className="text-(--muted)">{inv.email || "—"}</Td>
      <Td className="tabular-nums">{uses}</Td>
      <Td className="text-(--muted)">
        {inv.expires_at ? relTimeFromISO(inv.expires_at, now) : "Never"}
      </Td>
      <Td>
        <span className="inline-flex items-center gap-1.5">
          <Dot tone={status.tone} />
          <span className={inv.valid ? "" : "text-(--muted) capitalize"}>{status.label}</span>
        </span>
      </Td>
      <Td>
        <div className="flex items-center justify-end gap-1">
          <CopyButton value={inv.link ?? ""} label="invite link" />
          <button
            type="button"
            onClick={onDelete}
            title="Delete invitation"
            aria-label="Delete invitation"
            className="inline-grid size-6 place-items-center rounded text-(--muted) transition-colors duration-150 hover:text-(--danger) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
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
    <div className="rounded-(--radius) border border-(--border) bg-(--surface) p-5">
      <h2 className="text-[13px] font-semibold tracking-tight">Create invitation</h2>
      <p className="mt-0.5 text-[13px] text-(--muted)">
        Generates a shareable code. Set a usage limit and expiry, or leave them at 0 for unlimited /
        never.
      </p>

      <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField className="sm:col-span-2">
            <Label>Email (optional)</Label>
            <Input
              type="email"
              autoComplete="off"
              placeholder="person@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Description>Recorded for reference; the code works for any email.</Description>
          </TextField>

          <NumberField value={maxUses} onChange={setMaxUses} minValue={0} step={1}>
            <Label>Max uses</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
            <Description>0 = unlimited.</Description>
          </NumberField>

          <NumberField value={expiresInHours} onChange={setExpiresInHours} minValue={0} step={1}>
            <Label>Expires in (hours)</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input />
              <NumberField.IncrementButton />
            </NumberField.Group>
            <Description>0 = never expires.</Description>
          </NumberField>

          <TextField className="sm:col-span-2">
            <Label>Note (optional)</Label>
            <Input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="e.g. beta testers"
            />
          </TextField>
        </div>

        <Switch
          isSelected={sendEmail}
          onChange={setSendEmail}
          isDisabled={email.trim().length === 0}
          className="justify-between gap-4"
        >
          <Switch.Content>
            <Label>Email the invite</Label>
            <Description>
              Send the link to the address above (requires SMTP configured).
            </Description>
          </Switch.Content>
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
        </Switch>

        {error && (
          <p className="text-[13px] text-(--danger)" role="alert">
            {error}
          </p>
        )}

        <div className="flex justify-end border-t border-(--separator) pt-4">
          <Button type="submit" variant="primary" isDisabled={disabled || pending}>
            {pending ? "Creating…" : "Create invitation"}
          </Button>
        </div>
      </form>

      {created && (
        <div className="mt-4 rounded-(--radius) border border-(--border) bg-(--surface-secondary) p-3">
          <div className="flex items-center gap-2 text-[13px]">
            <Dot tone="ok" />
            <span className="font-medium">Invitation created</span>
            {created.email_sent === true && (
              <span className="text-xs text-(--muted)">· emailed to {created.email}</span>
            )}
            {created.email_sent === false && (
              <span className="text-xs text-(--warning)">
                · email not sent (SMTP not configured)
              </span>
            )}
          </div>
          <div className="group/key mt-2 flex items-center gap-1.5">
            <span className="min-w-0 truncate font-mono text-[12px] text-(--muted)">
              {created.link}
            </span>
            <CopyButton value={created.link ?? ""} label="invite link" />
          </div>
        </div>
      )}
    </div>
  );
}
