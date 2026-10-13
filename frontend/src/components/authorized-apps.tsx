import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@heroui/react";
import {
  canQueryPanelApi,
  fetchSettings,
  PanelApiError,
  queryErrorMessage,
  queryKeys,
  revokeUserOIDCGrant,
  userOIDCGrantsQueryOptions,
  type OIDCGrant,
} from "~/api/queries";
import {
  DestructiveConfirmModal,
  ErrorAlert,
  Section,
  TableSkeleton,
  Teaching,
} from "~/components/ui";
import { relTimeFromISO } from "~/lib/format";
import { formatScopes } from "~/lib/oidc-scopes";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

// Loads a User's Authorized Apps and owns the revoke flow.
function useOIDCGrants(userId: string) {
  const queryClient = useQueryClient();
  const [pendingGrant, setPendingGrant] = useState<OIDCGrant | null>(null);
  const grantsQuery = useQuery(userOIDCGrantsQueryOptions(userId));
  const revokeMutation = useMutation({
    mutationFn: async (grantId: string) => {
      try {
        await revokeUserOIDCGrant(userId, grantId);
      } catch (error) {
        // Another tab or an admin already revoked it, which is the outcome the User asked for.
        if (!(error instanceof PanelApiError && error.status === 404)) throw error;
      }
    },
    onSuccess: () => {
      setPendingGrant(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.userOIDCGrants(userId) });
    },
  });

  return {
    grants: grantsQuery.data ?? [],
    loading: grantsQuery.isPending,
    isError: grantsQuery.isError,
    listError: grantsQuery.error ? queryErrorMessage(grantsQuery.error) : "",
    revoking: revokeMutation.isPending,
    requestRevoke: setPendingGrant,
    modal: {
      pendingGrant,
      pending: revokeMutation.isPending,
      error: revokeMutation.error
        ? queryErrorMessage(revokeMutation.error, m.error_oidc_grant_revoke())
        : "",
      confirm: () => pendingGrant && revokeMutation.mutate(pendingGrant.id),
      close: () => {
        setPendingGrant(null);
        revokeMutation.reset();
      },
    },
  };
}

type OIDCGrantsState = ReturnType<typeof useOIDCGrants>;

/** Settings page section listing the Connected Apps the User has authorized. */
export function AuthorizedAppsSection({ userId }: { userId: string }) {
  const state = useOIDCGrants(userId);

  // Users who never authorized an app have nothing to manage here.
  if (state.loading || (state.grants.length === 0 && !state.isError)) return null;

  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">
          {m.settings_authorized_apps_title()}
        </h1>
        <p className="mt-0.5 text-[13px] text-muted">{m.settings_authorized_apps_desc()}</p>
      </div>
      <div className="overflow-hidden rounded-lg border bg-surface">
        <ErrorAlert message={state.listError} className="m-3" />
        <GrantList state={state} />
      </div>
      <RevokeModal state={state} body={m.settings_authorized_apps_revoke_confirm} />
    </>
  );
}

/** User detail page section, where admins manage another user's Authorized Apps. */
export function UserAuthorizedAppsSection({ userId }: { userId: string }) {
  const state = useOIDCGrants(userId);
  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi(),
  });
  const oidcEnabled = settingsQuery.data?.oidc_enabled ?? false;

  // Grants survive turning the provider off, so they stay listed until revoked.
  if (!oidcEnabled && state.grants.length === 0 && !state.isError) return null;

  return (
    <Section
      title={m.settings_authorized_apps_title()}
      meta={
        state.loading
          ? undefined
          : m.settings_authorized_apps_meta({ count: String(state.grants.length) })
      }
    >
      <ErrorAlert message={state.listError} className="m-3" />
      {state.loading ? (
        <TableSkeleton rows={1} />
      ) : state.grants.length === 0 ? (
        <Teaching
          title={m.settings_authorized_apps_empty_title()}
          hint={m.settings_authorized_apps_empty_other()}
        />
      ) : (
        <GrantList state={state} />
      )}
      <RevokeModal state={state} body={m.settings_authorized_apps_revoke_confirm_other} />
    </Section>
  );
}

function GrantList({ state }: { state: OIDCGrantsState }) {
  const now = useHydratedNow();
  return (
    <ul className="divide-y divide-separator">
      {state.grants.map((grant) => (
        <li key={grant.id} className="flex items-center justify-between gap-3 px-5 py-3">
          <div className="min-w-0">
            <p className="truncate text-[13px] font-medium text-foreground">{grant.client_name}</p>
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
            isDisabled={state.revoking}
            onPress={() => state.requestRevoke(grant)}
            className="border-danger text-danger hover:bg-danger-soft"
          >
            {m.settings_authorized_apps_revoke()}
          </Button>
        </li>
      ))}
    </ul>
  );
}

function RevokeModal({
  state,
  body,
}: {
  state: OIDCGrantsState;
  body: (inputs: { app: string }) => string;
}) {
  const { modal } = state;
  const appName = modal.pendingGrant?.client_name ?? "";
  return (
    <DestructiveConfirmModal
      isOpen={modal.pendingGrant !== null}
      title={m.settings_authorized_apps_revoke_title({ app: appName })}
      body={body({ app: appName })}
      confirmLabel={m.settings_authorized_apps_revoke()}
      pendingLabel={m.settings_authorized_apps_revoking()}
      pending={modal.pending}
      error={modal.error}
      onOpenChange={(open) => {
        if (!open) modal.close();
      }}
      onConfirm={modal.confirm}
    />
  );
}
