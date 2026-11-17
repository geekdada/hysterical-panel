import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ChevronRight } from "@gravity-ui/icons";
import { pinnedAnnouncementsQueryOptions, type AnnouncementSummary } from "~/api/queries";
import { Section } from "~/components/ui";
import { relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

const ROW_CLASS =
  "flex min-w-0 items-center gap-3 px-4 py-2.5 text-[13px] transition-colors duration-150 hover:bg-surface-secondary focus-visible:bg-surface-secondary focus-visible:outline-none";

/** Title-and-time rows shared by the user page and the announcements list. */
export function AnnouncementRows({ items }: { items: AnnouncementSummary[] }) {
  const now = useHydratedNow();
  return (
    <ul className="divide-y divide-separator">
      {items.map((item) => (
        <li key={item.id}>
          <Link to="/announcements/$id" params={{ id: item.id }} className={ROW_CLASS}>
            <span className="min-w-0 flex-1 truncate font-medium text-foreground">
              {item.title}
            </span>
            <span className="shrink-0 tabular-nums text-muted">
              {relTimeFromISO(item.created, now)}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/**
 * Pinned announcements on the user page. The section stays when nothing is
 * pinned but announcements exist, so the full list remains reachable.
 */
export function PinnedAnnouncements() {
  const query = useQuery(pinnedAnnouncementsQueryOptions());
  if (!query.data?.has_published) return null;

  const items = query.data.items;
  return (
    <Section title={m.announcements_title()}>
      {items.length > 0 ? <AnnouncementRows items={items} /> : null}
      <Link
        to="/announcements"
        className={`${ROW_CLASS} text-muted hover:text-foreground ${items.length > 0 ? "border-t border-separator" : ""}`}
      >
        <span className="flex-1">{m.announcements_view_all()}</span>
        <ChevronRight className="size-3.5 shrink-0" aria-hidden />
      </Link>
    </Section>
  );
}
