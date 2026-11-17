import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Chip } from "@heroui/react";
import { Plus } from "@gravity-ui/icons";
import {
  managedAnnouncementsQueryOptions,
  queryErrorMessage,
  type ManagedAnnouncement,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  ErrorAlert,
  PageShell,
  PanelMessage,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/announcements/")({
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient
      .ensureQueryData(managedAnnouncementsQueryOptions())
      .catch(() => undefined);
  },
  component: ManageAnnouncementsPage,
});

function ManageAnnouncementsPage() {
  const { auth } = Route.useRouteContext();
  const navigate = useNavigate();
  const now = useHydratedNow();
  const query = useQuery(managedAnnouncementsQueryOptions());
  const announcements = query.data ?? [];

  return (
    <PageShell
      width="medium"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-base font-semibold tracking-tight">
          {m.settings_announcements_manage()}
        </h1>
        <Button
          size="sm"
          variant="primary"
          onPress={() => void navigate({ to: "/settings/announcements/new" })}
        >
          <Plus className="size-3.5" aria-hidden />
          {m.announcements_new()}
        </Button>
      </div>

      <ErrorAlert message={query.error ? queryErrorMessage(query.error) : ""} className="mt-4" />

      <div className="mt-4 overflow-hidden rounded-lg border bg-surface">
        {query.isPending ? (
          <TableSkeleton />
        ) : query.error && announcements.length === 0 ? null : announcements.length === 0 ? (
          <PanelMessage>
            <span className="block font-medium text-foreground">{m.announcements_empty()}</span>
            <span className="mt-1 block">{m.announcements_empty_hint()}</span>
          </PanelMessage>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-[13px]">
              <thead>
                <tr className="border-b border-border bg-surface-secondary text-left">
                  <Th>{m.announcements_th_title()}</Th>
                  <Th className="text-right">{m.announcements_th_published()}</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-separator">
                {announcements.map((announcement) => (
                  <AnnouncementRow key={announcement.id} announcement={announcement} now={now} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </PageShell>
  );
}

function AnnouncementRow({
  announcement,
  now,
}: {
  announcement: ManagedAnnouncement;
  now: number | null;
}) {
  return (
    // The link's ::after overlay makes the whole row one click target.
    <tr className="relative cursor-pointer transition-colors duration-150 hover:bg-surface-secondary has-[a:focus-visible]:bg-surface-secondary">
      <Td>
        <span className="flex min-w-0 items-center gap-2">
          <Link
            to="/settings/announcements/$id"
            params={{ id: announcement.id }}
            className="truncate font-medium text-foreground after:absolute after:inset-0 focus-visible:underline focus-visible:outline-none"
          >
            {announcement.title}
          </Link>
          {announcement.pinned ? (
            <Chip size="sm" variant="soft" color="accent">
              {m.announcements_pinned()}
            </Chip>
          ) : null}
          {announcement.hidden ? (
            <Chip size="sm" variant="soft">
              {m.announcements_hidden()}
            </Chip>
          ) : null}
        </span>
      </Td>
      <Td className="whitespace-nowrap text-right text-muted">
        {relTimeFromISO(announcement.created, now)}
      </Td>
    </tr>
  );
}
