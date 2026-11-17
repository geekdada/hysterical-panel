import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import {
  announcementQueryOptions,
  isNotFoundError,
  panelConfigQueryOptions,
  queryErrorMessage,
  type Announcement,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { SetBreadcrumbTitle } from "~/components/breadcrumbs";
import { BrandLink, ErrorAlert, PageShell, Teaching } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatLocaleDateTime, relTimeFromISO } from "~/lib/format";
import { withPanelFileOrigin } from "~/lib/panel-files";
import { useActiveTimeZone } from "~/lib/use-timezone";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/announcements/$id")({
  staticData: breadcrumbStaticData({
    label: () => m.announcements_title(),
    dynamic: true,
  }),
  loader: async ({ context, params }) => {
    markResponsePrivate();
    // The body's file links depend on the API origin, so the server render
    // must resolve it the same way the browser will.
    await Promise.allSettled([
      context.queryClient.ensureQueryData(announcementQueryOptions(params.id)),
      context.queryClient.ensureQueryData(panelConfigQueryOptions()),
    ]);
  },
  component: AnnouncementPage,
});

function AnnouncementPage() {
  const { auth } = Route.useRouteContext();
  const { id } = Route.useParams();
  const query = useQuery(announcementQueryOptions(id));

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <SetBreadcrumbTitle title={query.data?.title} />
      {query.data ? (
        <AnnouncementArticle announcement={query.data} />
      ) : isNotFoundError(query.error) ? (
        <Teaching
          title={m.announcement_not_found_title()}
          hint={m.announcement_not_found_hint()}
          action={
            <Link to="/announcements" className="text-sm text-accent hover:underline">
              {m.announcements_view_all()}
            </Link>
          }
        />
      ) : query.error ? (
        <ErrorAlert message={queryErrorMessage(query.error)} />
      ) : null}
    </PageShell>
  );
}

function AnnouncementArticle({ announcement }: { announcement: Announcement }) {
  const origin = usePanelApiOrigin();
  const now = useHydratedNow();
  const timeZone = useActiveTimeZone();
  const published = Date.parse(announcement.created);

  return (
    <article>
      <h1 className="text-xl font-semibold tracking-tight text-balance">{announcement.title}</h1>
      <p className="mt-1.5 text-xs text-muted">
        <time
          dateTime={announcement.created}
          title={formatLocaleDateTime(published, undefined, timeZone)}
        >
          {m.announcement_published({ time: relTimeFromISO(announcement.created, now) })}
        </time>
      </p>
      <div
        className="announcement-body mt-6"
        // Sanitized by the backend allowlist on save (ADR-0015).
        dangerouslySetInnerHTML={{ __html: withPanelFileOrigin(announcement.html, origin) }}
      />
    </article>
  );
}
