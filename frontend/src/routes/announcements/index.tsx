import { useQuery } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import { announcementPageQueryOptions, queryErrorMessage } from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { AnnouncementRows } from "~/components/announcements";
import { BrandLink, ErrorAlert, PageShell, PanelMessage, TableSkeleton } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import * as m from "~/paraglide/messages.js";

type AnnouncementsSearch = { page?: number };

// The router merges this result over the raw query, so an invalid page must
// be overwritten with undefined rather than left out.
function parseSearch(search: Record<string, unknown>): AnnouncementsSearch {
  const page = Number(search.page);
  return { page: Number.isInteger(page) && page > 1 ? page : undefined };
}

export const Route = createFileRoute("/announcements/")({
  validateSearch: parseSearch,
  loaderDeps: ({ search }) => ({ page: search.page ?? 1 }),
  loader: ({ context, deps }) => {
    markResponsePrivate();
    return context.queryClient
      .ensureQueryData(announcementPageQueryOptions(deps.page))
      .catch(() => undefined);
  },
  component: AnnouncementsPage,
});

function AnnouncementsPage() {
  const { auth } = Route.useRouteContext();
  const page = Route.useSearch().page ?? 1;
  const query = useQuery(announcementPageQueryOptions(page));
  const items = query.data?.items ?? [];

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <h1 className="text-base font-semibold tracking-tight">{m.announcements_title()}</h1>
      <ErrorAlert message={query.error ? queryErrorMessage(query.error) : ""} className="mt-4" />

      <div className="mt-4 overflow-hidden rounded-lg border bg-surface">
        {query.isPending ? (
          <TableSkeleton />
        ) : items.length > 0 ? (
          <AnnouncementRows items={items} />
        ) : query.error || (query.data?.total ?? 0) > 0 ? null : (
          <PanelMessage>
            <span className="block font-medium text-foreground">{m.announcements_empty()}</span>
            <span className="mt-1 block">{m.announcements_reader_empty_hint()}</span>
          </PanelMessage>
        )}
        {query.data ? (
          <Pager page={page} total={query.data.total} perPage={query.data.per_page} />
        ) : null}
      </div>
    </PageShell>
  );
}

function Pager({ page, total, perPage }: { page: number; total: number; perPage: number }) {
  const navigate = useNavigate({ from: "/announcements/" });
  const pageCount = Math.ceil(total / perPage);
  // An edited URL can point past the last page, so the way back stays visible.
  if (pageCount <= 1 && page <= 1) return null;

  const goTo = (next: number) =>
    void navigate({ search: next > 1 ? { page: next } : {}, resetScroll: true });

  return (
    <div className="flex items-center justify-end gap-2 border-t border-separator px-3 py-2">
      <span className="text-xs tabular-nums text-muted">
        {m.users_pagination_page({ page: String(page), pageCount: String(pageCount) })}
      </span>
      <Button
        size="sm"
        variant="secondary"
        isDisabled={page <= 1}
        onPress={() => goTo(Math.min(page - 1, Math.max(pageCount, 1)))}
      >
        {m.users_pagination_previous()}
      </Button>
      <Button
        size="sm"
        variant="secondary"
        isDisabled={page >= pageCount}
        onPress={() => goTo(page + 1)}
      >
        {m.users_pagination_next()}
      </Button>
    </div>
  );
}
