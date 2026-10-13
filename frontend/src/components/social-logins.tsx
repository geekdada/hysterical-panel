import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Modal } from "@heroui/react";
import { startOAuthLink, type OAuthProvider } from "~/api/auth";
import {
  deleteUserOAuthLink,
  oauthProvidersQueryOptions,
  queryErrorMessage,
  queryKeys,
  userOAuthLinksQueryOptions,
  type OAuthLink,
} from "~/api/queries";
import { OAuthProviderIcon } from "~/components/oauth-provider-icon";
import {
  DestructiveConfirmModal,
  ErrorAlert,
  Section,
  TableSkeleton,
  Teaching,
} from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import { cn } from "~/lib/cn";
import { relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

// Loads a User's OAuth2 links and owns the unlink flow. The section stays
// hidden until PocketBase reports an enabled provider. A link to a disabled
// provider cannot sign in, so hiding it loses no access.
function useOAuthLinks(userId: string) {
  const queryClient = useQueryClient();
  const [pendingLink, setPendingLink] = useState<OAuthLink | null>(null);
  const providersQuery = useQuery(oauthProvidersQueryOptions());
  const hasProviders = (providersQuery.data ?? []).length > 0;
  const linksOptions = userOAuthLinksQueryOptions(userId);
  const linksQuery = useQuery({
    ...linksOptions,
    enabled: linksOptions.enabled !== false && hasProviders,
  });
  const unlinkMutation = useMutation({
    mutationFn: (linkId: string) => deleteUserOAuthLink(userId, linkId),
    onSuccess: () => {
      setPendingLink(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.userOAuthLinks(userId) });
    },
  });

  const providers = providersQuery.data ?? [];
  const links = linksQuery.data ?? [];
  const displayNames = new Map(providers.map((p) => [p.name, p.displayName]));
  // PocketBase allows one link per provider for each user.
  const linkedProviders = new Set(links.map((link) => link.provider));

  return {
    links,
    linkableProviders: providers.filter((p) => !linkedProviders.has(p.name)),
    loading: linksQuery.isPending,
    hidden: !hasProviders,
    listError: linksQuery.error ? queryErrorMessage(linksQuery.error) : "",
    providerLabel: (link: OAuthLink) => displayNames.get(link.provider) ?? link.provider,
    unlinkingId: unlinkMutation.isPending ? unlinkMutation.variables : undefined,
    requestUnlink: setPendingLink,
    modal: {
      pendingLink,
      pending: unlinkMutation.isPending,
      error: unlinkMutation.error
        ? queryErrorMessage(unlinkMutation.error, m.error_social_login_unlink())
        : "",
      confirm: () => pendingLink && unlinkMutation.mutate(pendingLink.id),
      close: () => {
        setPendingLink(null);
        unlinkMutation.reset();
      },
    },
  };
}

type OAuthLinksState = ReturnType<typeof useOAuthLinks>;

/** Settings page section for the signed-in user's own links. */
export function SocialLoginsSection({ userId }: { userId: string }) {
  const state = useOAuthLinks(userId);
  const [linkModalOpen, setLinkModalOpen] = useState(false);

  if (state.hidden) return null;
  if (state.loading) return <SocialLoginsSkeleton />;

  return (
    <>
      <SocialLoginsHeader />
      <div className="overflow-hidden rounded-lg border bg-surface">
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 px-5 py-3">
          <span className="text-[13px] text-muted">
            {m.settings_social_logins_meta({ count: String(state.links.length) })}
          </span>
          {state.linkableProviders.length > 0 && (
            <Button size="sm" variant="secondary" onPress={() => setLinkModalOpen(true)}>
              {m.settings_social_logins_link()}
            </Button>
          )}
        </div>
        <ErrorAlert message={state.listError} className="mx-5 mb-3" />
        {state.links.length === 0 ? (
          <p className="border-t border-border px-5 py-4 text-[13px] text-muted">
            {state.linkableProviders.length > 0
              ? m.settings_social_logins_empty_linkable()
              : m.settings_social_logins_empty()}
          </p>
        ) : (
          <OAuthLinkList state={state} className="border-t border-border" />
        )}
      </div>
      <UnlinkModal state={state} />
      <LinkProviderModal
        isOpen={linkModalOpen}
        userId={userId}
        providers={state.linkableProviders}
        onClose={() => setLinkModalOpen(false)}
      />
    </>
  );
}

/** User detail page section, where admins manage another user's links. */
export function UserSocialLoginsSection({ userId }: { userId: string }) {
  const state = useOAuthLinks(userId);
  if (state.hidden) return null;

  return (
    <Section
      title={m.settings_social_logins_title()}
      meta={
        state.loading
          ? undefined
          : m.settings_social_logins_meta({ count: String(state.links.length) })
      }
    >
      <ErrorAlert message={state.listError} className="m-3" />
      {state.loading ? (
        <TableSkeleton rows={1} />
      ) : state.links.length === 0 ? (
        <Teaching
          title={m.settings_social_logins_empty_title()}
          hint={m.settings_social_logins_empty_other()}
        />
      ) : (
        <OAuthLinkList state={state} />
      )}
      <UnlinkModal state={state} />
    </Section>
  );
}

function LinkProviderModal({
  isOpen,
  userId,
  providers,
  onClose,
}: {
  isOpen: boolean;
  userId: string;
  providers: OAuthProvider[];
  onClose: () => void;
}) {
  const linkMutation = useMutation({
    mutationFn: (providerName: string) => startOAuthLink(providerName, userId),
  });
  const pendingProvider = linkMutation.isPending ? linkMutation.variables : undefined;

  function handleOpenChange(open: boolean) {
    if (open) return;
    linkMutation.reset();
    onClose();
  }

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={handleOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{m.settings_social_logins_link()}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-2">
            {providers.map((provider) => (
              <Button
                key={provider.name}
                variant="secondary"
                fullWidth
                isDisabled={pendingProvider !== undefined}
                onPress={() => linkMutation.mutate(provider.name)}
              >
                <OAuthProviderIcon provider={provider.name} />
                {pendingProvider === provider.name ? m.auth_redirecting() : provider.displayName}
              </Button>
            ))}
            <ErrorAlert
              message={linkMutation.error ? localizeApiError(linkMutation.error.message) : ""}
            />
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function OAuthLinkList({ state, className }: { state: OAuthLinksState; className?: string }) {
  const now = useHydratedNow();
  return (
    <ul className={cn("divide-y divide-separator", className)}>
      {state.links.map((link) => {
        const unlinking = state.unlinkingId === link.id;
        return (
          <li key={link.id} className="flex items-center justify-between gap-3 px-5 py-3">
            <div className="min-w-0">
              <p className="flex items-center gap-2 text-[13px] font-medium text-foreground">
                <OAuthProviderIcon provider={link.provider} className="size-3.5" />
                <span className="truncate">{state.providerLabel(link)}</span>
              </p>
              <p className="mt-0.5 text-xs text-muted">
                {m.settings_social_logins_linked_at({ time: relTimeFromISO(link.created, now) })}
              </p>
            </div>
            <Button
              size="sm"
              variant="secondary"
              isDisabled={unlinking}
              onPress={() => state.requestUnlink(link)}
              className="border-danger text-danger hover:bg-danger-soft"
            >
              {unlinking ? m.settings_social_logins_unlinking() : m.settings_social_logins_unlink()}
            </Button>
          </li>
        );
      })}
    </ul>
  );
}

function UnlinkModal({ state }: { state: OAuthLinksState }) {
  const { pendingLink } = state.modal;
  const provider = pendingLink ? state.providerLabel(pendingLink) : "";
  return (
    <DestructiveConfirmModal
      isOpen={pendingLink !== null}
      title={m.settings_social_logins_unlink_title({ provider })}
      body={m.settings_social_logins_unlink_confirm({ provider })}
      confirmLabel={m.settings_social_logins_unlink()}
      pendingLabel={m.settings_social_logins_unlinking()}
      pending={state.modal.pending}
      error={state.modal.error}
      onOpenChange={(open) => !open && state.modal.close()}
      onConfirm={state.modal.confirm}
    />
  );
}

function SocialLoginsHeader() {
  return (
    <div className="mt-8 mb-5">
      <h1 className="text-base font-semibold tracking-tight">{m.settings_social_logins_title()}</h1>
      <p className="mt-0.5 text-[13px] text-muted">{m.settings_social_logins_desc()}</p>
    </div>
  );
}

function SocialLoginsSkeleton() {
  return (
    <>
      <SocialLoginsHeader />
      <div className="overflow-hidden rounded-lg border bg-surface" aria-hidden>
        <div className="px-5 py-3">
          <div className="h-3.5 w-20 animate-pulse rounded bg-surface-secondary" />
        </div>
        <div className="flex items-center justify-between gap-3 border-t border-border px-5 py-3">
          <div className="min-w-0 space-y-1.5">
            <div className="h-3.5 w-24 animate-pulse rounded bg-surface-secondary" />
            <div className="h-3 w-36 animate-pulse rounded bg-surface-secondary" />
          </div>
          <div className="h-8 w-16 shrink-0 animate-pulse rounded-full bg-surface-secondary" />
        </div>
      </div>
    </>
  );
}
