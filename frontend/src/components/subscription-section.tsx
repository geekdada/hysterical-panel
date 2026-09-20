import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@heroui/react";
import {
  grantSubscription,
  queryErrorMessage,
  queryKeys,
  subscriptionTypesQueryOptions,
  terminateSubscription,
  topUpSubscription,
  userSubscriptionsQueryOptions,
  type UserSubscription,
} from "~/api/queries";
import { DestructiveConfirmModal, ErrorAlert, Section, SelectField } from "~/components/ui";
import { formatBytes, formatLocaleDateTime } from "~/lib/format";
import { useActiveTimeZone } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

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
  const [selected, setSelected] = useState("");
  const [toTerminate, setToTerminate] = useState<UserSubscription | null>(null);
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.userSubscriptions(userId) });
    void queryClient.invalidateQueries({ queryKey: ["panel", "users", userId, "overview"] });
  };
  const grant = useMutation({
    mutationFn: (id: string) => grantSubscription(userId, id),
    onSuccess: refresh,
  });
  const topUp = useMutation({
    mutationFn: (id: string) => topUpSubscription(userId, id),
    onSuccess: refresh,
  });
  const terminate = useMutation({
    mutationFn: (id: string) => terminateSubscription(userId, id),
    onSuccess: () => {
      setToTerminate(null);
      refresh();
    },
  });
  const active = (grantsQuery.data ?? []).find((item) => item.status === "current");
  const queued = (grantsQuery.data ?? []).find((item) => item.status === "queued");
  const availableTypes = (typesQuery.data ?? []).filter((item) => !item.hidden);
  const selectedType = selected || availableTypes[0]?.id || "";
  const error = [grantsQuery.error, typesQuery.error, grant.error, topUp.error, terminate.error]
    .filter(Boolean)
    .map((item) => queryErrorMessage(item, m.subscription_action_error()))
    .join(" ");
  const grantControl = isAdmin && !queued && availableTypes.length > 0 && (
    <div className="flex flex-wrap items-end gap-3">
      <div className="min-w-44 flex-1 sm:max-w-64">
        <SelectField
          label={m.subscription_select_type()}
          value={selectedType}
          onChange={setSelected}
          options={availableTypes.map((item) => ({ value: item.id, label: item.name }))}
        />
      </div>
      <Button
        size="sm"
        variant="secondary"
        isPending={grant.isPending}
        onPress={() => grant.mutate(selectedType)}
      >
        {m.subscription_grant()}
      </Button>
    </div>
  );

  if (legacy)
    return isAdmin ? (
      <div className="mt-4">
        <ErrorAlert message={error} />
        {grantControl}
      </div>
    ) : null;

  return (
    <Section title={m.subscription_title()}>
      <ErrorAlert message={error} className="mb-3" />
      {(!active || active.remaining_bytes <= 0) && !grantsQuery.isPending && (
        <p className="text-[13px] text-muted">{m.subscription_none()}</p>
      )}
      <div className="divide-y divide-border">
        {[active, queued]
          .filter((item): item is UserSubscription => Boolean(item))
          .map((item) => (
            <div key={item.id} className="py-3 first:pt-0">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p className="text-[13px] font-medium">
                    {item.type_name} ·{" "}
                    {item.status === "current" ? m.subscription_current() : m.subscription_queued()}
                  </p>
                  <p className="mt-1 text-xs text-muted">
                    {m.subscription_starts()}{" "}
                    {formatLocaleDateTime(Date.parse(item.starts_at), undefined, tz)} ·{" "}
                    {m.subscription_ends()}{" "}
                    {formatLocaleDateTime(Date.parse(item.ends_at), undefined, tz)}
                  </p>
                </div>
                {isAdmin && (
                  <div className="flex gap-2">
                    {item.status === "current" && (
                      <Button
                        size="sm"
                        variant="secondary"
                        isPending={topUp.isPending}
                        onPress={() => topUp.mutate(item.id)}
                      >
                        {m.subscription_topup()}
                      </Button>
                    )}
                    <Button size="sm" variant="tertiary" onPress={() => setToTerminate(item)}>
                      {m.subscription_terminate()}
                    </Button>
                  </div>
                )}
              </div>
              {item.status === "current" && (
                <div className="mt-3 grid gap-2 text-[13px] sm:grid-cols-2 lg:grid-cols-4">
                  <p>
                    {m.subscription_allowance()}{" "}
                    <span className="font-mono tabular-nums">
                      {formatBytes(item.allowance_bytes)}
                    </span>
                  </p>
                  <p>
                    {m.subscription_used()}{" "}
                    <span className="font-mono tabular-nums">{formatBytes(item.used_bytes)}</span>
                  </p>
                  <p>
                    {m.subscription_remaining()}{" "}
                    <span className="font-mono tabular-nums">
                      {formatBytes(Math.max(0, item.remaining_bytes))}
                    </span>{" "}
                    {item.over_allowance && (
                      <span className="text-danger">{m.subscription_over()}</span>
                    )}
                  </p>
                  <p>
                    {m.subscription_resets()}{" "}
                    <span className="tabular-nums">
                      {formatLocaleDateTime(Date.parse(item.window_ends_at), undefined, tz)}
                    </span>
                  </p>
                </div>
              )}
            </div>
          ))}
      </div>
      {grantControl && <div className="mt-4 border-t border-border pt-4">{grantControl}</div>}
      <DestructiveConfirmModal
        isOpen={toTerminate !== null}
        title={m.subscription_terminate()}
        body={m.subscription_terminate_confirm()}
        confirmLabel={m.subscription_terminate()}
        pendingLabel={m.common_deleting()}
        pending={terminate.isPending}
        error={terminate.error ? queryErrorMessage(terminate.error) : ""}
        onOpenChange={(open) => {
          if (!open) setToTerminate(null);
        }}
        onConfirm={() => {
          if (toTerminate) terminate.mutate(toTerminate.id);
        }}
      />
    </Section>
  );
}
