import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { Link, useMatches } from "@tanstack/react-router";
import { Breadcrumbs as HeroUIBreadcrumbs } from "@heroui/react";
import type { BreadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { cn } from "~/lib/cn";

type BreadcrumbTitleContextValue = {
  detailTitle: string | null;
  setDetailTitle: (title: string | null) => void;
};

const BreadcrumbTitleContext = createContext<BreadcrumbTitleContextValue | null>(null);

export function BreadcrumbTitleProvider({ children }: { children: ReactNode }) {
  const [detailTitle, setDetailTitle] = useState<string | null>(null);
  const value = useMemo(() => ({ detailTitle, setDetailTitle }), [detailTitle]);
  return (
    <BreadcrumbTitleContext.Provider value={value}>{children}</BreadcrumbTitleContext.Provider>
  );
}

/** Syncs async page title (email, node name) into the breadcrumb trail. */
export function SetBreadcrumbTitle({ title }: { title: string | null | undefined }) {
  const ctx = useContext(BreadcrumbTitleContext);
  useEffect(() => {
    if (!ctx) return;
    ctx.setDetailTitle(title?.trim() ? title.trim() : null);
    return () => ctx.setDetailTitle(null);
  }, [ctx, title]);
  return null;
}

type Crumb = {
  key: string;
  label: string;
  href?: string;
  loading?: boolean;
};

function usePanelBreadcrumbItems(): Crumb[] {
  const matches = useMatches();
  const ctx = useContext(BreadcrumbTitleContext);

  return useMemo(() => {
    const trail: Crumb[] = [];
    const segments = matches.filter((match) => {
      const data = match.staticData as BreadcrumbStaticData | undefined;
      const segment = data?.breadcrumb;
      return segment && segment.visible !== false;
    });
    const leafId = segments.at(-1)?.id;

    for (const match of segments) {
      const data = match.staticData as BreadcrumbStaticData | undefined;
      const segment = data!.breadcrumb!;
      const isLeaf = match.id === leafId;
      const dynamicTitle = isLeaf ? ctx?.detailTitle : null;
      // A dynamic leaf (detail page) whose async title hasn't arrived shows a
      // skeleton instead of flashing the placeholder label.
      const loading = isLeaf && segment.dynamic === true && !dynamicTitle;
      const label = dynamicTitle ?? segment.label();

      trail.push({
        key: match.id,
        label,
        href: isLeaf ? undefined : (segment.href ?? match.pathname),
        loading,
      });
    }

    return trail;
  }, [ctx?.detailTitle, matches]);
}

export function BreadcrumbBar({ className }: { className?: string }) {
  const crumbs = usePanelBreadcrumbItems();
  if (crumbs.length === 0) return null;

  return (
    <div className={cn("py-2", className)}>
      <HeroUIBreadcrumbs className="min-w-0 flex-wrap">
        {crumbs.map((crumb) =>
          crumb.href ? (
            // `render` replaces the whole <li>, so we rebuild it: a TanStack Link
            // wearing HeroUI's `breadcrumbs__link` styles (matching the leaf) plus
            // the chevron HeroUI would otherwise have appended for non-current items.
            <HeroUIBreadcrumbs.Item
              key={crumb.key}
              render={(props) => (
                <li {...(props as ComponentProps<"li">)}>
                  <Link to={crumb.href!} className="link breadcrumbs__link max-w-48 truncate">
                    {crumb.label}
                  </Link>
                  <svg
                    aria-hidden="true"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={2}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    className="breadcrumbs__separator"
                  >
                    <path d="m9 18 6-6-6-6" />
                  </svg>
                </li>
              )}
            />
          ) : crumb.loading ? (
            <HeroUIBreadcrumbs.Item key={crumb.key}>
              <span className="sr-only">{crumb.label}</span>
              <span
                aria-hidden="true"
                className="block h-3.5 w-24 animate-pulse rounded bg-(--surface-secondary)"
              />
            </HeroUIBreadcrumbs.Item>
          ) : (
            <HeroUIBreadcrumbs.Item key={crumb.key} className="max-w-xs truncate">
              {crumb.label}
            </HeroUIBreadcrumbs.Item>
          )
        )}
      </HeroUIBreadcrumbs>
    </div>
  );
}
