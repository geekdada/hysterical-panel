import { useRef, useState } from "react";
import { useForm } from "@tanstack/react-form";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Button, Input, Label, ListBox, Modal, TextField } from "@heroui/react";
import { Plus, TrashBin } from "@gravity-ui/icons";
import { DropIndicator, type DropPosition, type Key, useDragAndDrop } from "react-aria-components";
import { requireAdmin } from "~/api/guards";
import {
  createResourceLink,
  deleteResourceLink,
  fetchManagedResourceLinks,
  PanelApiError,
  queryErrorMessage,
  queryKeys,
  reorderResourceLinks,
  updateResourceLink,
  type ManagedResourceLink,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { ResourceLinkFavicon } from "~/components/resource-links";
import {
  BrandLink,
  DestructiveConfirmModal,
  ErrorAlert,
  PageShell,
  PanelMessage,
  TableSkeleton,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatLocaleCount, relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

const REORDER_SCOPE = "resource-links-reorder";

export const Route = createFileRoute("/settings/resource-links")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.resource_links_title(),
  }),
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData({
      queryKey: queryKeys.managedResourceLinks(),
      queryFn: fetchManagedResourceLinks,
    });
  },
  component: ResourceLinksPage,
});

type LinkSave = {
  id: string | null;
  url: string;
  title: string;
};

// Moves the dragged keys next to the target while keeping their relative order.
// Returns null when the drop would not change the list.
function moveLinkOrder(
  ids: string[],
  keys: Set<Key>,
  target: Key,
  dropPosition: DropPosition
): string[] | null {
  const moving = ids.filter((id) => keys.has(id));
  const targetId = String(target);
  if (moving.length === 0 || keys.has(targetId)) return null;

  const remaining = ids.filter((id) => !keys.has(id));
  const targetIndex = remaining.indexOf(targetId);
  if (targetIndex === -1) return null;

  const insertAt = dropPosition === "after" ? targetIndex + 1 : targetIndex;
  const next = [...remaining.slice(0, insertAt), ...moving, ...remaining.slice(insertAt)];
  return next.every((id, index) => id === ids[index]) ? null : next;
}

function linkHostname(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

function ResourceLinksPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const now = useHydratedNow();
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<ManagedResourceLink | null>(null);
  const [deleting, setDeleting] = useState<ManagedResourceLink | null>(null);

  const linksQuery = useQuery({
    queryKey: queryKeys.managedResourceLinks(),
    queryFn: fetchManagedResourceLinks,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.managedResourceLinks() });
    void queryClient.invalidateQueries({ queryKey: queryKeys.resourceLinks(), exact: true });
  };

  const saveMutation = useMutation({
    mutationFn: ({ id, url, title }: LinkSave) =>
      id ? updateResourceLink(id, { url, title }) : createResourceLink({ url, title }),
    onSuccess: () => {
      closeEditor();
      invalidate();
    },
  });

  // Drops run one at a time. A failed drop rolls back to the last order the
  // server confirmed, not to the optimistic order of an earlier queued drop.
  const confirmedOrder = useRef<ManagedResourceLink[] | undefined>(undefined);
  const pendingReorders = () =>
    queryClient.isMutating({
      predicate: (mutation) => mutation.options.scope?.id === REORDER_SCOPE,
    });

  const reorderMutation = useMutation({
    scope: { id: REORDER_SCOPE },
    mutationFn: async (ids: string[]) => {
      try {
        return await reorderResourceLinks(ids);
      } catch (error) {
        if (error instanceof PanelApiError && error.status === 409) throw error;
        throw new PanelApiError(m.error_resource_links_reorder());
      }
    },
    onMutate: async (ids) => {
      const startsChain = pendingReorders() === 1;
      await queryClient.cancelQueries({ queryKey: queryKeys.managedResourceLinks() });
      const previous = queryClient.getQueryData<ManagedResourceLink[]>(
        queryKeys.managedResourceLinks()
      );
      if (startsChain) confirmedOrder.current = previous;
      if (previous) {
        const byId = new Map(previous.map((link) => [link.id, link]));
        const ordered = ids.flatMap((id) => {
          const link = byId.get(id);
          return link ? [link] : [];
        });
        queryClient.setQueryData(queryKeys.managedResourceLinks(), ordered);
      }
    },
    onSuccess: (links) => {
      confirmedOrder.current = links;
      // A queued drop already shows a newer optimistic order.
      if (pendingReorders() === 1) {
        queryClient.setQueryData(queryKeys.managedResourceLinks(), links);
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.resourceLinks(), exact: true });
    },
    onError: (error) => {
      if (error instanceof PanelApiError && error.status === 409) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.managedResourceLinks() });
        return;
      }
      // A queued drop sends an order that already includes this move.
      if (pendingReorders() > 1) return;
      if (confirmedOrder.current) {
        queryClient.setQueryData(queryKeys.managedResourceLinks(), confirmedOrder.current);
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteResourceLink,
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
  });

  const links = linksQuery.data ?? [];
  const loadError = linksQuery.error ? queryErrorMessage(linksQuery.error) : "";
  const reorderError = reorderMutation.error ? queryErrorMessage(reorderMutation.error) : "";

  const { dragAndDropHooks } = useDragAndDrop({
    getItems: (keys) => [...keys].map((key) => ({ "text/plain": String(key) })),
    onReorder: (event) => {
      const ids = moveLinkOrder(
        links.map((link) => link.id),
        event.keys,
        event.target.key,
        event.target.dropPosition
      );
      if (ids) reorderMutation.mutate(ids);
    },
    renderDropIndicator: (target) => (
      <DropIndicator target={target} className="h-0.5 rounded-full bg-accent" />
    ),
  });

  function openCreate() {
    setEditing(null);
    setEditorOpen(true);
    saveMutation.reset();
  }

  function openEdit(link: ManagedResourceLink) {
    setEditing(link);
    setEditorOpen(true);
    saveMutation.reset();
  }

  function closeEditor() {
    setEditorOpen(false);
    setEditing(null);
    saveMutation.reset();
  }

  function requestDelete() {
    const link = editing;
    closeEditor();
    setDeleting(link);
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-base font-semibold tracking-tight">{m.resource_links_title()}</h1>
        <Button size="sm" variant="primary" onPress={openCreate}>
          <Plus className="size-3.5" aria-hidden />
          {m.resource_links_add()}
        </Button>
      </div>

      <ErrorAlert message={loadError || reorderError} className="mt-4" />

      <LinkFormModal
        key={editing ? `edit-${editing.id}` : editorOpen ? "create-open" : "create-closed"}
        link={editing}
        isOpen={editorOpen}
        onOpenChange={(open) => !open && closeEditor()}
        pending={saveMutation.isPending}
        error={
          saveMutation.error
            ? queryErrorMessage(saveMutation.error, m.error_resource_link_save_network())
            : ""
        }
        onSubmit={(input) => saveMutation.mutate({ id: editing?.id ?? null, ...input })}
        onDelete={editing ? requestDelete : undefined}
      />

      <DestructiveConfirmModal
        isOpen={deleting !== null}
        title={m.resource_links_delete_confirm_title()}
        body={m.resource_links_delete_confirm_body()}
        confirmLabel={m.resource_links_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={
          deleteMutation.error
            ? queryErrorMessage(deleteMutation.error, m.error_resource_link_delete_network())
            : ""
        }
        onOpenChange={(open) => {
          if (!open) {
            setDeleting(null);
            deleteMutation.reset();
          }
        }}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />

      <section className="mt-6 overflow-hidden rounded-lg border bg-surface">
        {linksQuery.isPending ? (
          <TableSkeleton />
        ) : links.length === 0 ? (
          <PanelMessage>
            <span className="font-medium text-foreground">{m.resource_links_empty()}</span>
          </PanelMessage>
        ) : (
          <ListBox
            aria-label={m.resource_links_title()}
            selectionMode="none"
            dragAndDropHooks={dragAndDropHooks}
            onAction={(key) => {
              const link = links.find((item) => item.id === String(key));
              if (link) openEdit(link);
            }}
          >
            {links.map((link) => (
              <ListBox.Item key={link.id} id={link.id} textValue={link.title} className="px-4 py-3">
                <ResourceLinkFavicon id={link.id} version={link.favicon_version} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-medium">{link.title}</span>
                  <span className="block truncate text-xs text-muted">
                    {linkHostname(link.url)}
                  </span>
                </span>
                <span className="shrink-0 text-right text-xs tabular-nums text-muted">
                  <span className="block">
                    {m.resource_links_visits({ count: formatLocaleCount(link.visit_count) })}
                  </span>
                  <span className="block">
                    {link.last_visited_at
                      ? m.resource_links_last_visited({
                          time: relTimeFromISO(link.last_visited_at, now),
                        })
                      : m.resource_links_never_visited()}
                  </span>
                </span>
              </ListBox.Item>
            ))}
          </ListBox>
        )}
      </section>

      {links.length > 1 && (
        <p className="mt-2 px-0.5 text-xs text-muted">{m.resource_links_reorder_hint()}</p>
      )}
    </PageShell>
  );
}

function LinkFormModal({
  link,
  isOpen,
  onOpenChange,
  pending,
  error,
  onSubmit,
  onDelete,
}: {
  link?: ManagedResourceLink | null;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  onSubmit: (input: { url: string; title: string }) => void;
  onDelete?: () => void;
}) {
  const isNew = !link;
  const titlePlaceholder = link?.fetched_title || m.resource_links_title_placeholder();
  const form = useForm({
    defaultValues: {
      url: link?.url ?? "",
      title: link?.manual_title ?? "",
    },
    onSubmit: ({ value }) => {
      const url = value.url.trim();
      if (!url) return;
      onSubmit({ url, title: value.title.trim() });
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
                {isNew ? m.resource_links_add() : m.resource_links_edit()}
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <form.Field name="url">
                  {(field) => (
                    <TextField
                      value={field.state.value}
                      onChange={field.handleChange}
                      onBlur={field.handleBlur}
                      isRequired
                      autoFocus
                    >
                      <Label>{m.resource_links_url_label()}</Label>
                      <Input
                        autoComplete="off"
                        inputMode="url"
                        spellCheck={false}
                        placeholder="https://"
                      />
                    </TextField>
                  )}
                </form.Field>
                <form.Field name="title">
                  {(field) => (
                    <TextField
                      value={field.state.value}
                      onChange={field.handleChange}
                      onBlur={field.handleBlur}
                    >
                      <Label>{m.resource_links_title_label()}</Label>
                      <Input autoComplete="off" maxLength={200} placeholder={titlePlaceholder} />
                    </TextField>
                  )}
                </form.Field>
                <p className="-mt-2 text-xs text-muted">{m.resource_links_saving_hint()}</p>
              </div>
              {error && (
                <p className="mt-4 text-[13px] text-danger" role="alert">
                  {error}
                </p>
              )}
            </Modal.Body>
            <Modal.Footer>
              {onDelete && (
                <Button variant="danger-soft" className="mr-auto" onPress={onDelete}>
                  <TrashBin className="size-3.5" aria-hidden />
                  {m.resource_links_delete()}
                </Button>
              )}
              <Button slot="close" variant="secondary">
                {m.common_cancel()}
              </Button>
              <form.Subscribe selector={(state) => state.values.url}>
                {(url) => (
                  <Button
                    type="submit"
                    variant="primary"
                    isPending={pending}
                    isDisabled={pending || !url.trim()}
                  >
                    {m.resource_links_save()}
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
