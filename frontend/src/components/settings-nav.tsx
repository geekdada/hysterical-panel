import { useEffect, useMemo, useRef, useState, type MouseEvent, type RefObject } from "react";
import { useActiveSection } from "~/lib/use-active-section";
import { cn } from "~/lib/cn";
import * as m from "~/paraglide/messages.js";

export type SettingsNavItem = {
  id: string;
  label: string;
  /** The section renders nothing in some states, so the link waits until its anchor exists. */
  optional?: boolean;
};

export type SettingsNavGroup = {
  label: string;
  items: SettingsNavItem[];
};

// Below lg the nav is a sticky bar under the page header, so anchors clear both.
// The bar is py-2 around h-7 items plus a 1px border, about 3rem, plus 1rem of air.
const SECTION_SCROLL_MARGIN =
  "scroll-mt-[calc(var(--page-header-h,3rem)+4rem)] lg:scroll-mt-[calc(var(--page-header-h,3rem)+1.5rem)]";

/** Heading block for one settings section. `id` is the anchor the nav links to. */
export function SettingsSectionHeader({
  id,
  title,
  description,
  className = "mt-8 mb-5",
}: {
  id: string;
  title: string;
  description?: string;
  className?: string;
}) {
  return (
    <div id={id} className={cn(SECTION_SCROLL_MARGIN, className)}>
      <h3 className="text-base font-semibold tracking-tight">{title}</h3>
      {description && <p className="mt-0.5 text-[13px] text-muted">{description}</p>}
    </div>
  );
}

/**
 * In-page section nav. On lg and up it is a sticky column beside the content.
 * Below lg it becomes a horizontally scrolling bar pinned under the page header.
 */
export function SettingsNav({
  groups,
  contentRef,
  className,
}: {
  groups: SettingsNavGroup[];
  contentRef: RefObject<HTMLElement | null>;
  className?: string;
}) {
  const visibleGroups = useVisibleGroups(groups, contentRef);
  const ids = useMemo(
    () => visibleGroups.flatMap((group) => group.items.map((item) => item.id)),
    [visibleGroups]
  );
  const { activeId, pin } = useActiveSection(ids);
  const scrollerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (activeId) revealInScroller(scrollerRef.current, activeId);
  }, [activeId]);

  function jumpTo(event: MouseEvent<HTMLAnchorElement>, id: string) {
    const target = document.getElementById(id);
    if (!target) return;
    event.preventDefault();
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    target.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "start" });
    history.replaceState(history.state, "", `#${id}`);
    pin(id);
  }

  return (
    <nav
      aria-label={m.settings_nav_label()}
      className={cn(
        "sticky top-[var(--page-header-h,3rem)] z-10 -mx-4 -mt-6 mb-6 border-b border-border bg-surface sm:-mx-6",
        "lg:top-[calc(var(--page-header-h,3rem)+1.5rem)] lg:mx-0 lg:mt-0 lg:mb-0 lg:self-start lg:border-0 lg:bg-transparent",
        className
      )}
    >
      <div
        ref={scrollerRef}
        className={cn(
          "relative flex gap-1 overflow-x-auto px-4 py-2 [scrollbar-width:none] sm:px-6 [&::-webkit-scrollbar]:hidden",
          "lg:max-h-[calc(100dvh-var(--page-header-h,3rem)-3rem)] lg:flex-col lg:gap-5 lg:overflow-x-visible lg:overflow-y-auto lg:p-0"
        )}
      >
        {visibleGroups.map((group) => (
          <div key={group.label} className="flex shrink-0 lg:block">
            <p className="hidden px-2 pb-1.5 text-xs font-semibold text-foreground lg:block">
              {group.label}
            </p>
            <ul className="flex gap-1 lg:flex-col lg:gap-0.5">
              {group.items.map((item) => (
                <li key={item.id} className="shrink-0">
                  <NavLink
                    item={item}
                    active={item.id === activeId}
                    onClick={(event) => jumpTo(event, item.id)}
                  />
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </nav>
  );
}

function NavLink({
  item,
  active,
  onClick,
}: {
  item: SettingsNavItem;
  active: boolean;
  onClick: (event: MouseEvent<HTMLAnchorElement>) => void;
}) {
  return (
    <a
      href={`#${item.id}`}
      data-nav-id={item.id}
      aria-current={active ? "location" : undefined}
      onClick={onClick}
      className={cn(
        "flex h-7 items-center whitespace-nowrap rounded-md px-2.5 text-[13px] transition-colors duration-150",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus lg:px-2",
        active
          ? "bg-surface-secondary text-foreground"
          : "text-muted hover:bg-surface-secondary hover:text-foreground"
      )}
    >
      {item.label}
    </a>
  );
}

// Optional items appear once their section mounts, and leave when it unmounts.
function useVisibleGroups(
  groups: SettingsNavGroup[],
  contentRef: RefObject<HTMLElement | null>
): SettingsNavGroup[] {
  const optionalIds = useMemo(
    () => groups.flatMap((group) => group.items.filter((i) => i.optional).map((i) => i.id)),
    [groups]
  );
  const [mounted, setMounted] = useState<ReadonlySet<string>>(() => new Set());

  useEffect(() => {
    const content = contentRef.current;
    if (!content || optionalIds.length === 0) return;
    const sync = () => {
      const present = optionalIds.filter((id) => document.getElementById(id));
      setMounted((prev) => (sameMembers(prev, present) ? prev : new Set(present)));
    };
    sync();
    const observer = new MutationObserver(sync);
    observer.observe(content, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [contentRef, optionalIds]);

  return useMemo(
    () =>
      groups
        .map((group) => ({
          ...group,
          items: group.items.filter((item) => !item.optional || mounted.has(item.id)),
        }))
        .filter((group) => group.items.length > 0),
    [groups, mounted]
  );
}

function sameMembers(set: ReadonlySet<string>, list: string[]): boolean {
  return set.size === list.length && list.every((id) => set.has(id));
}

// Keeps the active item in view on the mobile bar without moving the page.
function revealInScroller(scroller: HTMLElement | null, id: string) {
  if (!scroller || scroller.scrollWidth <= scroller.clientWidth) return;
  const link = scroller.querySelector<HTMLElement>(`[data-nav-id="${CSS.escape(id)}"]`);
  if (!link) return;
  const gutter = 16;
  const left = link.offsetLeft - gutter;
  const right = link.offsetLeft + link.offsetWidth + gutter - scroller.clientWidth;
  if (scroller.scrollLeft > left) scroller.scrollTo({ left, behavior: "smooth" });
  else if (scroller.scrollLeft < right) scroller.scrollTo({ left: right, behavior: "smooth" });
}
