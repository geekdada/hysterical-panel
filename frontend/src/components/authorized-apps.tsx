import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@heroui/react";
import {
  queryErrorMessage,
  queryKeys,
  revokeUserOIDCGrant,
  userOIDCGrantsQueryOptions,
  type OIDCGrant,
} from "~/api/queries";
import { DestructiveConfirmModal, ErrorAlert } from "~/components/ui";
import { relTimeFromISO } from "~/lib/format";
import { formatScopes } from "~/lib/oidc-scopes";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

/** Settings page section listing the Connected Apps the User has authorized. */
export function AuthorizedAppsSection({ userId }: { userId: string }) {
  const queryClient = useQueryClient();
  const now = useHydratedNow();
  const [pendingGrant, setPendingGrant] = useState<OIDCGrant | null>(null);
  const grantsQuery = useQuery(userOIDCGrantsQueryOptions(userId));
  const revokeMutation = useMutation({
    mutationFn: (grantId: string) => revokeUserOIDCGrant(userId, grantId),
    onSuccess: () => {
      setPendingGrant(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.userOIDCGrants(userId) });
    },
  });
  const grants = grantsQuery.data ?? [];

  // Users who never authorized an app have nothing to manage here.
  if (grantsQuery.isPending || (grants.length === 0 && !grantsQuery.isError)) return null;

  const appName = pendingGrant?.client_name ?? "";
  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">
          {m.settings_authorized_apps_title()}
        </h1>
        <p className="mt-0.5 text-[13px] text-muted">{m.settings_authorized_apps_desc()}</p>
      </div>
      <div className="overflow-hidden rounded-lg border bg-surface">
        <ErrorAlert
          message={grantsQuery.error ? queryErrorMessage(grantsQuery.error) : ""}
          className="m-3"
        />
        <ul className="divide-y divide-separator">
          {grants.map((grant) => (
            <li key={grant.id} className="flex items-center justify-between gap-3 px-5 py-3">
              <div className="min-w-0">
                <p className="truncate text-[13px] font-medium text-foreground">
                  {grant.client_name}
                </p>
                <p className="mt-0.5 text-xs text-muted">{formatScopes(grant.scopes)}</p>
                <p className="mt-0.5 text-xs text-muted">
                  {grant.last_used_at
                    ? m.settings_authorized_apps_last_used({
                        time: relTimeFromISO(grant.last_used_at, now),
                      })
                    : m.settings_authorized_apps_authorized_at({
                        time: relTimeFromISO(grant.created, now),
                      })}
                </p>
              </div>
              <Button
                size="sm"
                variant="secondary"
                isDisabled={revokeMutation.isPending}
                onPress={() => setPendingGrant(grant)}
                className="border-danger text-danger hover:bg-danger-soft"
              >
                {m.settings_authorized_apps_revoke()}
              </Button>
            </li>
          ))}
        </ul>
      </div>
      <DestructiveConfirmModal
        isOpen={pendingGrant !== null}
        title={m.settings_authorized_apps_revoke_title({ app: appName })}
        body={m.settings_authorized_apps_revoke_confirm({ app: appName })}
        confirmLabel={m.settings_authorized_apps_revoke()}
        pendingLabel={m.settings_authorized_apps_revoking()}
        pending={revokeMutation.isPending}
        error={
          revokeMutation.error
            ? queryErrorMessage(revokeMutation.error, m.error_oidc_grant_revoke())
            : ""
        }
        onOpenChange={(open) => {
          if (open) return;
          setPendingGrant(null);
          revokeMutation.reset();
        }}
        onConfirm={() => pendingGrant && revokeMutation.mutate(pendingGrant.id)}
      />
    </>
  );
}
