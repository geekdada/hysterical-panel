import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import { TrashBin } from "@gravity-ui/icons";
import {
  deleteAnnouncement,
  managedAnnouncementQueryOptions,
  queryErrorMessage,
  queryKeys,
  updateAnnouncement,
  type ManagedAnnouncementDetail,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { AnnouncementForm, type SubmittedAnnouncement } from "~/announcements/announcement-form";
import { SetBreadcrumbTitle } from "~/components/breadcrumbs";
import {
  BrandLink,
  DestructiveConfirmModal,
  ErrorAlert,
  PageShell,
  PanelMessage,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/announcements/$id")({
  staticData: breadcrumbStaticData({
    label: () => m.announcements_edit(),
    dynamic: true,
  }),
  loader: ({ context, params }) => {
    markResponsePrivate();
    return context.queryClient
      .ensureQueryData(managedAnnouncementQueryOptions(params.id))
      .catch(() => undefined);
  },
  component: EditAnnouncementPage,
});

function EditAnnouncementPage() {
  const { auth } = Route.useRouteContext();
  const { id } = Route.useParams();
  const query = useQuery(managedAnnouncementQueryOptions(id));
  const announcement = query.data;

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <SetBreadcrumbTitle title={announcement?.title} />
      <h1 className="mb-6 text-base font-semibold tracking-tight">{m.announcements_edit()}</h1>
      {announcement ? (
        // Remount when another announcement loads so the editor starts from its content.
        <EditAnnouncement key={announcement.id} announcement={announcement} />
      ) : query.error ? (
        <ErrorAlert message={queryErrorMessage(query.error)} />
      ) : (
        <PanelMessage>{m.common_loading()}</PanelMessage>
      )}
    </PageShell>
  );
}

function EditAnnouncement({ announcement }: { announcement: ManagedAnnouncementDetail }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const saveMutation = useMutation({
    mutationFn: (body: SubmittedAnnouncement) => updateAnnouncement(announcement.id, body),
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.managedAnnouncement(announcement.id), saved);
      void queryClient.invalidateQueries({ queryKey: queryKeys.announcements() });
    },
  });
  const deleteMutation = useMutation({
    mutationFn: () => deleteAnnouncement(announcement.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.managedAnnouncement(announcement.id) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.announcements() });
      void navigate({ to: "/settings/announcements" });
    },
  });

  return (
    <>
      <AnnouncementForm
        initial={{
          title: announcement.title,
          pinned: announcement.pinned,
          hidden: announcement.hidden,
        }}
        initialContent={announcement.content}
        showHidden
        submitLabel={m.announcement_form_save()}
        pending={saveMutation.isPending}
        footer={
          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              className="text-danger"
              onPress={() => {
                deleteMutation.reset();
                setConfirmingDelete(true);
              }}
            >
              <TrashBin className="size-4" aria-hidden />
              {m.announcements_delete()}
            </Button>
            {saveMutation.isSuccess ? (
              <span className="text-xs text-muted" role="status">
                {m.announcement_form_saved()}
              </span>
            ) : null}
          </div>
        }
        onSubmit={(body) => saveMutation.mutate(body)}
      />
      <ErrorAlert
        message={saveMutation.error ? queryErrorMessage(saveMutation.error) : ""}
        className="mt-4"
      />
      <DestructiveConfirmModal
        isOpen={confirmingDelete}
        title={m.announcements_delete_confirm_title()}
        body={m.announcements_delete_confirm_body()}
        confirmLabel={m.announcements_delete()}
        pendingLabel={m.common_deleting()}
        pending={deleteMutation.isPending}
        error={deleteMutation.error ? queryErrorMessage(deleteMutation.error) : ""}
        onOpenChange={setConfirmingDelete}
        onConfirm={() => deleteMutation.mutate()}
      />
    </>
  );
}
