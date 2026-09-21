import { useState } from "react";
import { useForm } from "@tanstack/react-form";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Button, FieldError, Input, Label, Modal, NumberField, TextField } from "@heroui/react";
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

function allowanceBytes(gib: number): number | null {
  if (!Number.isFinite(gib) || gib <= 0) return null;
  const bytes = gib * GIB;
  if (!Number.isSafeInteger(bytes)) return null;
  return bytes;
}

function allowanceUnchanged(editing: SubscriptionType | null, gib: number) {
  return editing != null && Number.isFinite(gib) && gib === editing.allowance_bytes / GIB;
}

function nameError(name: string) {
  return name.trim() ? undefined : m.subscriptions_name_required();
}

function gibError(gib: number, editing: SubscriptionType | null) {
  if (allowanceUnchanged(editing, gib) || allowanceBytes(gib) != null) return undefined;
  return m.subscriptions_allowance_invalid();
}

type SubscriptionTypeSave = {
  name: string;
  allowanceBytes?: number;
  resetDays: number;
};

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
  const [draft, setDraft] = useState<SubscriptionType | "new" | null>(null);
  const [toDelete, setToDelete] = useState<SubscriptionType | null>(null);
  const editing = draft && draft !== "new" ? draft : null;

  const refresh = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.subscriptionTypes() });
  const save = useMutation({
    mutationFn: (input: SubscriptionTypeSave) => {
      if (editing) {
        return updateSubscriptionType(editing.id, {
          name: input.name,
          ...(input.allowanceBytes != null && { allowance_bytes: input.allowanceBytes }),
          reset_days: input.resetDays,
        });
      }
      if (input.allowanceBytes == null) throw new Error(m.subscriptions_allowance_invalid());
      return createSubscriptionType({
        name: input.name,
        allowance_bytes: input.allowanceBytes,
        reset_days: input.resetDays,
      });
    },
    onSuccess: () => {
      setDraft(null);
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

  return (
    <PageShell headerLeft={<BrandLink />} headerRight={auth ? <UserMenu auth={auth} /> : undefined}>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold">{m.subscriptions_title()}</h1>
          <p className="mt-1 text-[13px] text-muted">{m.subscriptions_description()}</p>
        </div>
        <Button size="sm" variant="primary" onPress={() => setDraft("new")}>
          <Plus className="size-3.5" aria-hidden />
          {m.subscriptions_add()}
        </Button>
      </div>
      <ErrorAlert
        message={
          (toggle.error || remove.error
            ? queryErrorMessage(toggle.error || remove.error, m.subscriptions_action_error())
            : "") ||
          (typesQuery.error
            ? queryErrorMessage(typesQuery.error, m.subscriptions_load_error())
            : "")
        }
        className="mt-4"
      />
      {draft !== null && (
        <SubscriptionTypeForm
          key={editing?.id ?? "new"}
          editing={editing}
          pending={save.isPending}
          error={save.error ? queryErrorMessage(save.error, m.subscriptions_action_error()) : ""}
          onOpenChange={(open) => {
            if (!open) {
              setDraft(null);
              save.reset();
            }
          }}
          onSave={(input) => save.mutateAsync(input)}
        />
      )}
      <div className="mt-6 divide-y divide-border border-y border-border">
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
                onPress={() => setDraft(item)}
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

function SubscriptionTypeForm({
  editing,
  pending,
  error,
  onOpenChange,
  onSave,
}: {
  editing: SubscriptionType | null;
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
  onSave: (input: SubscriptionTypeSave) => Promise<unknown>;
}) {
  const form = useForm({
    defaultValues: {
      name: editing?.name ?? "",
      gib: editing ? editing.allowance_bytes / GIB : Number.NaN,
      resetDays: String(editing?.reset_days ?? 30),
    },
    onSubmit: async ({ value }) => {
      const unchanged = allowanceUnchanged(editing, value.gib);
      const bytes = unchanged && editing ? editing.allowance_bytes : allowanceBytes(value.gib);
      if (!value.name.trim() || bytes == null) return;
      await onSave({
        name: value.name.trim(),
        allowanceBytes: unchanged ? undefined : bytes,
        resetDays: Number(value.resetDays),
      });
    },
  });
  const submitLabel = pending
    ? editing
      ? m.subscriptions_saving()
      : m.subscriptions_adding()
    : editing
      ? m.subscriptions_save()
      : m.subscriptions_add();

  return (
    <Modal.Backdrop isOpen onOpenChange={onOpenChange}>
      <Modal.Container size="md" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              event.stopPropagation();
              void form.handleSubmit();
            }}
          >
            <Modal.Header>
              <Modal.Heading>
                {editing ? m.subscriptions_edit({ name: editing.name }) : m.subscriptions_add()}
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <form.Field
                  name="name"
                  validators={{
                    onChange: ({ value }) => nameError(value),
                    onSubmit: ({ value }) => nameError(value),
                  }}
                >
                  {(field) => {
                    const invalid =
                      field.state.meta.isTouched && field.state.meta.errors.length > 0;
                    return (
                      <TextField
                        fullWidth
                        value={field.state.value}
                        onChange={field.handleChange}
                        onBlur={field.handleBlur}
                        isRequired
                        isInvalid={invalid}
                      >
                        <Label>{m.subscriptions_name()}</Label>
                        <Input
                          maxLength={128}
                          autoFocus={typeof window !== "undefined" && !("ontouchstart" in window)}
                          spellCheck={false}
                          autoComplete="off"
                          data-1p-ignore
                          data-lpignore="true"
                        />
                        {invalid ? (
                          <FieldError>{m.subscriptions_name_required()}</FieldError>
                        ) : null}
                      </TextField>
                    );
                  }}
                </form.Field>
                <form.Field
                  name="gib"
                  validators={{
                    onChange: ({ value }) => gibError(value, editing),
                    onSubmit: ({ value }) => gibError(value, editing),
                  }}
                >
                  {(field) => {
                    const invalid =
                      field.state.meta.isTouched && field.state.meta.errors.length > 0;
                    return (
                      <NumberField
                        fullWidth
                        value={field.state.value}
                        onChange={field.handleChange}
                        onBlur={field.handleBlur}
                        formatOptions={{ maximumFractionDigits: 6, useGrouping: false }}
                        commitBehavior="validate"
                        isRequired
                        isInvalid={invalid}
                      >
                        <Label>
                          {m.subscriptions_allowance()}
                          <span className="sr-only"> ({m.subscriptions_unit_gib()})</span>
                        </Label>
                        <NumberField.Group
                          className="relative"
                          style={{ gridTemplateColumns: "minmax(0, 1fr)", height: "auto" }}
                        >
                          <NumberField.Input
                            autoComplete="off"
                            style={{ paddingInlineEnd: "2.75rem" }}
                          />
                          <span
                            aria-hidden
                            className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-muted"
                          >
                            {m.subscriptions_unit_gib()}
                          </span>
                        </NumberField.Group>
                        {invalid ? (
                          <FieldError>{m.subscriptions_allowance_invalid()}</FieldError>
                        ) : null}
                      </NumberField>
                    );
                  }}
                </form.Field>
                <form.Field name="resetDays">
                  {(field) => (
                    <SelectField
                      fullWidth
                      label={m.subscriptions_reset()}
                      value={field.state.value}
                      onChange={field.handleChange}
                      isDisabled={Boolean(editing?.assigned)}
                      description={m.subscriptions_reset_hint()}
                      options={[
                        { value: "30", label: m.subscriptions_monthly() },
                        { value: "360", label: m.subscriptions_annual() },
                      ]}
                    />
                  )}
                </form.Field>
                {error ? (
                  <p className="text-[13px] text-danger" role="alert">
                    {error}
                  </p>
                ) : null}
              </div>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary" isDisabled={pending}>
                {m.common_cancel()}
              </Button>
              <Button type="submit" variant="primary" isDisabled={pending} isPending={pending}>
                {submitLabel}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
