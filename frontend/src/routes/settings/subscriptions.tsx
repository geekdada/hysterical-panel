import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Button, Input, Label, TextField } from "@heroui/react";
import { Pencil, Plus, TrashBin } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  createSubscriptionType,
  deleteSubscriptionType,
  queryErrorMessage,
  queryKeys,
  subscriptionTypesQueryOptions,
  updateSubscriptionType,
  type SubscriptionType,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  DestructiveConfirmModal,
  ErrorAlert,
  PageShell,
  SelectField,
  TableSkeleton,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatBytes } from "~/lib/format";
import * as m from "~/paraglide/messages.js";

const GIB = 1024 ** 3;

export const Route = createFileRoute("/settings/subscriptions")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({ label: () => m.subscriptions_title() }),
  loader: async ({ context }) => {
    markResponsePrivate();
    await context.queryClient.ensureQueryData(subscriptionTypesQueryOptions());
  },
  component: SubscriptionTypesPage,
});

function SubscriptionTypesPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const typesQuery = useQuery(subscriptionTypesQueryOptions());
  const [editing, setEditing] = useState<SubscriptionType | null>(null);
  const [name, setName] = useState("");
  const [gib, setGib] = useState("");
  const [resetDays, setResetDays] = useState("30");
  const [toDelete, setToDelete] = useState<SubscriptionType | null>(null);
  const [validation, setValidation] = useState("");

  const refresh = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.subscriptionTypes() });
  const save = useMutation({
    mutationFn: async () => {
      const allowanceBytes = Number(gib) * GIB;
      const allowanceUnchanged = editing && gib === String(editing.allowance_bytes / GIB);
      if (
        !name.trim() ||
        (!allowanceUnchanged && (!Number.isSafeInteger(allowanceBytes) || allowanceBytes <= 0))
      ) {
        throw new Error(m.subscriptions_action_error());
      }
      if (editing) {
        return updateSubscriptionType(editing.id, {
          name: name.trim(),
          ...(!allowanceUnchanged && { allowance_bytes: allowanceBytes }),
          reset_days: Number(resetDays),
        });
      }
      return createSubscriptionType({
        name: name.trim(),
        allowance_bytes: allowanceBytes,
        reset_days: Number(resetDays),
      });
    },
    onSuccess: () => {
      resetForm();
      refresh();
    },
  });
  const toggle = useMutation({
    mutationFn: (item: SubscriptionType) =>
      updateSubscriptionType(item.id, { hidden: !item.hidden }),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (item: SubscriptionType) => deleteSubscriptionType(item.id),
    onSuccess: () => {
      setToDelete(null);
      refresh();
    },
  });

  function resetForm() {
    setEditing(null);
    setName("");
    setGib("");
    setResetDays("30");
    setValidation("");
  }

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const bytes = Number(gib) * GIB;
    const allowanceUnchanged = editing && gib === String(editing.allowance_bytes / GIB);
    if (!name.trim() || (!allowanceUnchanged && (!Number.isSafeInteger(bytes) || bytes <= 0))) {
      setValidation(m.subscriptions_action_error());
      return;
    }
    setValidation("");
    save.mutate();
  }

  return (
    <PageShell headerLeft={<BrandLink />} headerRight={auth ? <UserMenu auth={auth} /> : undefined}>
      <div className="mb-6">
        <h1 className="text-lg font-semibold">{m.subscriptions_title()}</h1>
        <p className="mt-1 text-[13px] text-muted">{m.subscriptions_description()}</p>
      </div>
      <ErrorAlert
        message={
          validation ||
          (save.error || toggle.error || remove.error
            ? queryErrorMessage(
                save.error || toggle.error || remove.error,
                m.subscriptions_action_error()
              )
            : "") ||
          (typesQuery.error
            ? queryErrorMessage(typesQuery.error, m.subscriptions_load_error())
            : "")
        }
        className="mb-4"
      />
      <form
        onSubmit={submit}
        className="grid gap-4 border-y border-border py-5 md:grid-cols-[minmax(0,1fr)_10rem_11rem_auto] md:items-end"
      >
        <TextField value={name} onChange={setName} isRequired>
          <Label>{m.subscriptions_name()}</Label>
          <Input maxLength={128} />
        </TextField>
        <TextField>
          <Label>{m.subscriptions_allowance()}</Label>
          <Input
            type="number"
            min="0"
            step="any"
            value={gib}
            onChange={(e) => setGib(e.target.value)}
            required
          />
        </TextField>
        {editing?.assigned ? (
          <div className="text-[13px] text-muted">
            <span className="block text-xs">{m.subscriptions_reset()}</span>
            {resetDays === "30" ? m.subscriptions_monthly() : m.subscriptions_annual()}
          </div>
        ) : (
          <SelectField
            label={m.subscriptions_reset()}
            value={resetDays}
            onChange={setResetDays}
            options={[
              { value: "30", label: m.subscriptions_monthly() },
              { value: "360", label: m.subscriptions_annual() },
            ]}
          />
        )}
        <div className="flex items-center gap-2">
          <Button type="submit" size="sm" variant="primary" isPending={save.isPending}>
            {!editing && <Plus className="size-4" aria-hidden />}
            {editing ? m.subscriptions_save() : m.subscriptions_add()}
          </Button>
          {editing && (
            <Button size="sm" variant="tertiary" onPress={resetForm}>
              {m.common_cancel()}
            </Button>
          )}
        </div>
      </form>
      <div className="mt-5 divide-y divide-border border-y border-border">
        {typesQuery.isPending ? (
          <TableSkeleton />
        ) : (typesQuery.data ?? []).length === 0 ? (
          <p className="py-8 text-center text-[13px] text-muted">{m.subscriptions_empty()}</p>
        ) : (
          (typesQuery.data ?? []).map((item) => (
            <div key={item.id} className="flex flex-wrap items-center gap-3 py-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium">
                  {item.name}{" "}
                  {item.hidden && <span className="text-muted">({m.subscriptions_hidden()})</span>}
                </p>
                <p className="text-xs text-muted">
                  {formatBytes(item.allowance_bytes)} ·{" "}
                  {item.reset_days === 30 ? m.subscriptions_monthly() : m.subscriptions_annual()}
                </p>
              </div>
              <Button
                size="sm"
                variant="tertiary"
                onPress={() => {
                  setEditing(item);
                  setName(item.name);
                  setGib(String(item.allowance_bytes / GIB));
                  setResetDays(String(item.reset_days));
                  setValidation("");
                }}
                aria-label={m.subscriptions_edit({ name: item.name })}
              >
                <Pencil className="size-4" aria-hidden />
              </Button>
              <Button
                size="sm"
                variant="secondary"
                isDisabled={toggle.isPending}
                onPress={() => toggle.mutate(item)}
              >
                {item.hidden ? m.subscriptions_show() : m.subscriptions_hide()}
              </Button>
              {!item.assigned && (
                <Button
                  size="sm"
                  variant="tertiary"
                  onPress={() => setToDelete(item)}
                  aria-label={`${m.common_delete()} ${item.name}`}
                >
                  <TrashBin className="size-4" aria-hidden />
                </Button>
              )}
            </div>
          ))
        )}
      </div>
      <DestructiveConfirmModal
        isOpen={toDelete !== null}
        title={m.common_delete()}
        body={m.subscriptions_delete_confirm()}
        confirmLabel={m.common_delete()}
        pendingLabel={m.common_deleting()}
        pending={remove.isPending}
        error={remove.error ? queryErrorMessage(remove.error) : ""}
        onOpenChange={(open) => {
          if (!open) setToDelete(null);
        }}
        onConfirm={() => {
          if (toDelete) remove.mutate(toDelete);
        }}
      />
    </PageShell>
  );
}
