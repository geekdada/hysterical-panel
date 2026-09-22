import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Disclosure, FieldError, Label, Modal, NumberField } from "@heroui/react";
import {
  grantSubscription,
  queryErrorMessage,
  queryKeys,
  subscriptionTypesQueryOptions,
  terminateSubscription,
  topUpSubscription,
  userSubscriptionsQueryOptions,
  type SubscriptionType,
  type UserSubscription,
} from "~/api/queries";
import { DestructiveConfirmModal, Section, SelectField } from "~/components/ui";
import { formatBytes, formatLocaleDateTime } from "~/lib/format";
import { cn } from "~/lib/cn";
import { useActiveTimeZone } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

const GIB = 1024 ** 3;

function allowanceBytes(gib: number): number | null {
  if (!Number.isFinite(gib) || gib <= 0) return null;
  const bytes = gib * GIB;
  if (!Number.isSafeInteger(bytes)) return null;
  return bytes;
}

export function SubscriptionSection({
  userId,
  isAdmin,
  legacy,
}: {
  userId: string;
  isAdmin: boolean;
  legacy: boolean;
}) {
  const queryClient = useQueryClient();
  const tz = useActiveTimeZone();
  const grantsQuery = useQuery(userSubscriptionsQueryOptions(userId));
  const typesQuery = useQuery({
    ...subscriptionTypesQueryOptions(),
    enabled: isAdmin && typeof window !== "undefined",
  });
  const [grantOpen, setGrantOpen] = useState(false);
  const [selected, setSelected] = useState("");
  const [toTopUp, setToTopUp] = useState<UserSubscription | null>(null);
  const [gib, setGib] = useState(Number.NaN);
  const [gibTouched, setGibTouched] = useState(false);
  const [topUpFieldKey, setTopUpFieldKey] = useState(0);
  const [toTerminate, setToTerminate] = useState<UserSubscription | null>(null);
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.userSubscriptions(userId) });
    void queryClient.invalidateQueries({ queryKey: ["panel", "users", userId, "overview"] });
  };
  const grant = useMutation({
    mutationFn: (id: string) => grantSubscription(userId, id),
    onSuccess: () => {
      setGrantOpen(false);
      refresh();
    },
  });
  const topUp = useMutation({
    mutationFn: (input: { id: string; allowanceBytes: number }) =>
      topUpSubscription(userId, input.id, input.allowanceBytes),
    onSuccess: () => {
      setToTopUp(null);
      refresh();
    },
  });
  const terminate = useMutation({
    mutationFn: (id: string) => terminateSubscription(userId, id),
    onSuccess: () => {
      setToTerminate(null);
      refresh();
    },
  });
  const grants = grantsQuery.data ?? [];
  const active = grants.find((item) => item.status === "current");
  const queued = grants.find((item) => item.status === "queued");
  const history = grants.filter(
    (item) => item.status === "expired" || item.status === "terminated"
  );
  const availableTypes = (typesQuery.data ?? []).filter((item) => !item.hidden);
  const canGrant = isAdmin && !queued && availableTypes.length > 0;
  const loadError = [grantsQuery.error, isAdmin ? typesQuery.error : null]
    .filter(Boolean)
    .map((item) => queryErrorMessage(item, m.subscription_load_error()))
    .join(" ");
  const selectedId = selected || availableTypes[0]?.id || "";

  function openTopUp(item: UserSubscription) {
    const type = (typesQuery.data ?? []).find((entry) => entry.id === item.subscription_type);
    setGib(type ? type.allowance_bytes / GIB : Number.NaN);
    setGibTouched(false);
    setTopUpFieldKey((key) => key + 1);
    topUp.reset();
    setToTopUp(item);
  }

  function openGrant() {
    setSelected((current) =>
      availableTypes.some((item) => item.id === current) ? current : (availableTypes[0]?.id ?? "")
    );
    grant.reset();
    setGrantOpen(true);
  }

  const grantButton = canGrant ? (
    <Button size="sm" variant="secondary" onPress={openGrant}>
      {m.subscription_grant()}
    </Button>
  ) : null;
  const grantModal = (canGrant || grantOpen) && (
    <GrantModal
      isOpen={grantOpen}
      types={availableTypes}
      selected={selectedId}
      legacy={legacy}
      willQueue={Boolean(active)}
      pending={grant.isPending}
      error={grant.error ? queryErrorMessage(grant.error, m.subscription_action_error()) : ""}
      onSelectedChange={setSelected}
      onOpenChange={(open) => {
        if (!open) {
          setGrantOpen(false);
          grant.reset();
        }
      }}
      onConfirm={() => {
        if (selectedId) grant.mutate(selectedId);
      }}
    />
  );

  if (legacy) {
    if (!isAdmin) return null;
    return (
      <>
        <Section title={m.subscription_title()} action={grantButton}>
          <LoadError message={loadError} />
          <p className="px-4 py-3 text-[13px] text-muted">{m.subscription_legacy()}</p>
        </Section>
        {grantModal}
      </>
    );
  }

  const rows = [active, queued].filter((item): item is UserSubscription => Boolean(item));
  const unavailable = (!active || active.remaining_bytes <= 0) && !grantsQuery.isPending;

  return (
    <>
      <Section title={m.subscription_title()} action={grantButton}>
        <LoadError message={loadError} />
        {grantsQuery.isPending ? (
          <SubscriptionSkeleton />
        ) : (
          <>
            {unavailable && (
              <p
                className={cn(
                  "px-4 py-3 text-[13px] text-muted",
                  rows.length > 0 && "border-b border-border"
                )}
              >
                {m.subscription_none()}
              </p>
            )}
            <div className="divide-y divide-border">
              {rows.map((item) => (
                <GrantRow
                  key={item.id}
                  item={item}
                  isAdmin={isAdmin}
                  timeZone={tz}
                  onTopUp={() => openTopUp(item)}
                  onTerminate={() => {
                    terminate.reset();
                    setToTerminate(item);
                  }}
                />
              ))}
            </div>
            {history.length > 0 && (
              <GrantHistory
                items={history}
                timeZone={tz}
                bordered={rows.length > 0 || unavailable}
              />
            )}
          </>
        )}
      </Section>
      {grantModal}
      <TopUpModal
        isOpen={toTopUp !== null}
        fieldKey={topUpFieldKey}
        gib={gib}
        gibTouched={gibTouched}
        pending={topUp.isPending}
        error={topUp.error ? queryErrorMessage(topUp.error, m.subscription_action_error()) : ""}
        onGibChange={setGib}
        onTouch={() => setGibTouched(true)}
        onOpenChange={(open) => {
          if (!open) {
            setToTopUp(null);
            topUp.reset();
          }
        }}
        onConfirm={(bytes) => {
          if (toTopUp) topUp.mutate({ id: toTopUp.id, allowanceBytes: bytes });
        }}
      />
      <DestructiveConfirmModal
        isOpen={toTerminate !== null}
        title={m.subscription_terminate()}
        body={m.subscription_terminate_confirm()}
        confirmLabel={m.subscription_terminate()}
        pendingLabel={m.subscription_terminating()}
        pending={terminate.isPending}
        error={
          terminate.error ? queryErrorMessage(terminate.error, m.subscription_action_error()) : ""
        }
        onOpenChange={(open) => {
          if (!open) {
            setToTerminate(null);
            terminate.reset();
          }
        }}
        onConfirm={() => {
          if (toTerminate) terminate.mutate(toTerminate.id);
        }}
      />
    </>
  );
}

function GrantRow({
  item,
  isAdmin,
  timeZone,
  onTopUp,
  onTerminate,
}: {
  item: UserSubscription;
  isAdmin: boolean;
  timeZone: string;
  onTopUp: () => void;
  onTerminate: () => void;
}) {
  const current = item.status === "current";
  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
        <div className="min-w-0">
          <p className="truncate text-[13px] font-medium text-foreground">{item.type_name}</p>
          <p className="mt-0.5 text-xs text-muted">
            {current ? m.subscription_current() : m.subscription_queued()}
            {" · "}
            {m.subscription_starts()}{" "}
            {formatLocaleDateTime(Date.parse(item.starts_at), undefined, timeZone)}
            {" · "}
            {m.subscription_ends()}{" "}
            {formatLocaleDateTime(Date.parse(item.ends_at), undefined, timeZone)}
          </p>
        </div>
        {isAdmin && (
          <div className="flex shrink-0 items-center gap-3">
            {current && (
              <Button size="sm" variant="secondary" onPress={onTopUp}>
                {m.subscription_topup()}
              </Button>
            )}
            <Button size="sm" variant="danger-soft" onPress={onTerminate}>
              {m.subscription_terminate()}
            </Button>
          </div>
        )}
      </div>
      {current && (
        <div className="grid divide-y divide-border border-t border-border md:grid-cols-4 md:divide-x md:divide-y-0">
          <Meter label={m.subscription_allowance()}>
            <span className="font-mono text-[13px] tabular-nums">
              {formatBytes(item.allowance_bytes)}
            </span>
          </Meter>
          <Meter label={m.subscription_used()}>
            <span className="font-mono text-[13px] tabular-nums">
              {formatBytes(item.used_bytes)}
            </span>
            <p className="mt-0.5 text-xs text-muted">
              <DirectionUsage tx={item.used_tx_bytes} rx={item.used_rx_bytes} />
            </p>
          </Meter>
          <Meter label={m.subscription_remaining()}>
            <span className="inline-flex items-baseline gap-2">
              <span className="font-mono text-[13px] font-medium tabular-nums">
                {formatBytes(Math.max(0, item.remaining_bytes))}
              </span>
              {item.over_allowance && (
                <span className="text-xs text-danger">{m.subscription_over()}</span>
              )}
            </span>
          </Meter>
          <Meter label={m.subscription_resets()}>
            <span className="text-[13px] tabular-nums">
              {formatLocaleDateTime(Date.parse(item.window_ends_at), undefined, timeZone)}
            </span>
          </Meter>
        </div>
      )}
    </div>
  );
}

function DirectionUsage({ tx, rx }: { tx: number; rx: number }) {
  return (
    <span className="font-mono tabular-nums">
      {m.common_th_tx()} {formatBytes(tx)} · {m.common_th_rx()} {formatBytes(rx)}
    </span>
  );
}

// Past grants show Grant Usage: window usage only covers the last window written.
function GrantHistory({
  items,
  timeZone,
  bordered,
}: {
  items: UserSubscription[];
  timeZone: string;
  bordered: boolean;
}) {
  return (
    <Disclosure className={cn(bordered && "border-t border-border")}>
      <Disclosure.Heading>
        <Disclosure.Trigger className="flex w-full items-center justify-between px-4 py-2.5 text-left text-xs font-medium text-muted transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-focus">
          <span>
            {m.subscription_history()} <span className="tabular-nums">{items.length}</span>
          </span>
          <Disclosure.Indicator />
        </Disclosure.Trigger>
      </Disclosure.Heading>
      <Disclosure.Content>
        <div className="divide-y divide-border border-t border-border">
          {items.map((item) => {
            const terminated = item.status === "terminated";
            const endedAt = terminated ? item.terminated_at : item.ends_at;
            return (
              <div
                key={item.id}
                className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="truncate text-[13px] font-medium text-foreground">
                    {item.type_name}
                  </p>
                  <p className="mt-0.5 text-xs text-muted">
                    {terminated ? m.subscription_terminated() : m.subscription_expired()}
                    {" · "}
                    {m.subscription_starts()}{" "}
                    {formatLocaleDateTime(Date.parse(item.starts_at), undefined, timeZone)}
                    {" · "}
                    {m.subscription_ends()}{" "}
                    {formatLocaleDateTime(Date.parse(endedAt), undefined, timeZone)}
                  </p>
                </div>
                <p className="shrink-0 text-xs text-muted">
                  <DirectionUsage tx={item.grant_tx_bytes} rx={item.grant_rx_bytes} />
                </p>
              </div>
            );
          })}
        </div>
      </Disclosure.Content>
    </Disclosure>
  );
}

function Meter({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 px-4 py-3">
      <div className="text-[11px] font-medium uppercase tracking-wider text-muted">{label}</div>
      <div className="mt-1 min-w-0">{children}</div>
    </div>
  );
}

function TopUpModal({
  isOpen,
  fieldKey,
  gib,
  gibTouched,
  pending,
  error,
  onGibChange,
  onTouch,
  onOpenChange,
  onConfirm,
}: {
  isOpen: boolean;
  fieldKey: number;
  gib: number;
  gibTouched: boolean;
  pending: boolean;
  error: string;
  onGibChange: (value: number) => void;
  onTouch: () => void;
  onOpenChange: (open: boolean) => void;
  onConfirm: (bytes: number) => void;
}) {
  const bytes = allowanceBytes(gib);
  const invalid = gibTouched && bytes == null;
  const focusAmount = typeof window !== "undefined" && !("ontouchstart" in window);

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(event) => {
              event.preventDefault();
              onTouch();
              if (pending || bytes == null) return;
              onConfirm(bytes);
            }}
          >
            <Modal.Header>
              <Modal.Heading>{m.subscription_topup()}</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-3">
                <NumberField
                  key={fieldKey}
                  fullWidth
                  value={gib}
                  onChange={onGibChange}
                  onBlur={onTouch}
                  formatOptions={{ maximumFractionDigits: 6, useGrouping: false }}
                  commitBehavior="validate"
                  isRequired
                  isInvalid={invalid}
                  isDisabled={pending}
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
                      autoFocus={isOpen && focusAmount}
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
                  {invalid ? <FieldError>{m.subscriptions_allowance_invalid()}</FieldError> : null}
                </NumberField>
                {bytes != null ? (
                  <p className="text-xs leading-5 text-muted">
                    {m.subscription_topup_hint({ amount: formatBytes(bytes) })}
                  </p>
                ) : null}
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
                {pending ? m.subscription_topup_pending() : m.subscription_topup()}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function GrantModal({
  isOpen,
  types,
  selected,
  legacy,
  willQueue,
  pending,
  error,
  onSelectedChange,
  onOpenChange,
  onConfirm,
}: {
  isOpen: boolean;
  types: SubscriptionType[];
  selected: string;
  legacy: boolean;
  willQueue: boolean;
  pending: boolean;
  error: string;
  onSelectedChange: (value: string) => void;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  const name = types.find((item) => item.id === selected)?.name ?? "";
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!pending && selected) onConfirm();
            }}
          >
            <Modal.Header>
              <Modal.Heading>{m.subscription_grant()}</Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-muted">
                {grantConfirm(name, legacy, willQueue)}
              </p>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-3">
                <SelectField
                  fullWidth
                  label={m.subscription_select_type()}
                  value={selected}
                  onChange={onSelectedChange}
                  isDisabled={pending || types.length === 0}
                  options={types.map((item) => ({ value: item.id, label: item.name }))}
                />
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
              <Button
                type="submit"
                variant="primary"
                isDisabled={pending || !selected}
                isPending={pending}
              >
                {pending ? m.subscription_granting() : m.subscription_grant()}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function grantConfirm(name: string, legacy: boolean, willQueue: boolean) {
  if (legacy) return m.subscription_grant_confirm_legacy({ name });
  if (willQueue) return m.subscription_grant_confirm_queued({ name });
  return m.subscription_grant_confirm_now({ name });
}

function LoadError({ message }: { message: string }) {
  if (!message) return null;
  return (
    <div
      className="border-b border-border bg-danger-soft px-4 py-2 text-[13px] text-danger-soft-foreground"
      role="alert"
    >
      {message}
    </div>
  );
}

function SubscriptionSkeleton() {
  return (
    <div className="px-4 py-3">
      <div className="h-3 w-28 animate-pulse rounded bg-surface-secondary" />
      <div className="mt-2 h-3 w-56 animate-pulse rounded bg-surface-secondary" />
    </div>
  );
}
