import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { createAnnouncement, queryErrorMessage, queryKeys } from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { AnnouncementForm } from "~/announcements/announcement-form";
import { BrandLink, ErrorAlert, PageShell } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/announcements/new")({
  staticData: breadcrumbStaticData({ label: () => m.announcements_new() }),
  loader: () => markResponsePrivate(),
  component: NewAnnouncementPage,
});

function NewAnnouncementPage() {
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: createAnnouncement,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.announcements() });
      void navigate({ to: "/settings/announcements" });
    },
  });

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <h1 className="mb-6 text-base font-semibold tracking-tight">{m.announcements_new()}</h1>
      <AnnouncementForm
        initial={{ title: "", pinned: false, hidden: false }}
        showHidden={false}
        submitLabel={m.announcement_form_publish()}
        pending={mutation.isPending}
        onSubmit={(announcement) => mutation.mutate(announcement)}
      />
      <ErrorAlert
        message={mutation.error ? queryErrorMessage(mutation.error) : ""}
        className="mt-4"
      />
    </PageShell>
  );
}
