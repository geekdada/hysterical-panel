import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import type { Column } from "@tanstack/react-table";
import { Check, Copy } from "@gravity-ui/icons";
import { Card } from "@heroui/react";
import { BreadcrumbTitleProvider, BreadcrumbBar } from "~/components/breadcrumbs";
import { cn } from "~/lib/cn";
import * as m from "~/paraglide/messages.js";

/* ── Layout ────────────────────────────────────────────────────────────── */

// Page chrome shared by every signed-in page: the full-height background, the
// sticky header bar, and the content <main>. `width` is the single source of
// truth for content width so the header and main always line up — "wide" for
// data-dense pages (dashboards, detail views), "narrow" for focused forms and
// settings. headerLeft/headerRight fill the standard justify-between header row.
export function PageShell({
  width = "wide",
  headerLeft,
  headerRight,
  children,
}: {
  width?: "wide" | "narrow";
  headerLeft: ReactNode;
  headerRight?: ReactNode;
  children: ReactNode;
}) {
  const max = width === "narrow" ? "max-w-3xl" : "max-w-7xl";
  return (
    <BreadcrumbTitleProvider>
      <div className="min-h-svh bg-(--background) text-(--foreground)">
        <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
          <div className={`mx-auto flex h-12 ${max} items-center justify-between px-4 sm:px-6`}>
            {headerLeft}
            {headerRight}
          </div>
          <BreadcrumbBar className={`mx-auto ${max} px-4 sm:px-6`} />
        </header>
        <main className={`mx-auto ${max} px-4 py-6 sm:px-6`}>{children}</main>
      </div>
    </BreadcrumbTitleProvider>
  );
}

// The "H" badge + wordmark shown on the left of top-level pages.
export function Brand() {
  return (
    <div className="flex items-center gap-2">
      <span className="grid size-5 place-items-center rounded-[5px] bg-(--accent) text-[11px] font-bold text-(--accent-foreground)">
        H
      </span>
      <span className="text-[13px] font-semibold tracking-tight">{m.app_title()}</span>
    </div>
  );
}

// Brand linked to the dashboard home — sits in the header on secondary pages,
// where the breadcrumb trail handles upward navigation.
export function BrandLink() {
  return (
    <Link
      to="/"
      className="shrink-0 rounded-sm no-underline transition-opacity duration-150 hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
    >
      <Brand />
    </Link>
  );
}

// Centered single-card layout shared by the auth pages (login, register,
// verify, forgot-password). Pages pass their Card.Header/Content as children.
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh items-center justify-center bg-(--background) p-4 text-(--foreground)">
      <Card className="w-full max-w-sm border border-(--border) bg-(--surface)">{children}</Card>
    </div>
  );
}

export function Section({
  title,
  meta,
  action,
  className,
  children,
}: {
  title: string;
  meta?: ReactNode;
  action?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section className={cn("mt-6", className)}>
      <div className="mb-2 flex flex-col gap-2 px-0.5 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <h2 className="shrink-0 text-[13px] font-semibold text-(--foreground)">{title}</h2>
          {meta && (
            <span className="min-w-0 truncate text-xs tabular-nums text-(--muted)">{meta}</span>
          )}
        </div>
        {action && <div className="min-w-0 w-full sm:w-auto">{action}</div>}
      </div>
      <div className="overflow-hidden rounded-(--radius) border border-(--border) bg-(--surface)">
        {children}
      </div>
    </section>
  );
}

/* ── Tables ────────────────────────────────────────────────────────────── */

export function Th({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return (
    <th
      className={cn(
        "px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-(--muted)",
        className
      )}
    >
      {children}
    </th>
  );
}

export function ServerSortableTh({
  columnId,
  sort,
  onSort,
  children,
  align = "left",
  className = "",
}: {
  columnId: string;
  sort: string;
  onSort: (columnId: string) => void;
  children: ReactNode;
  align?: "left" | "right";
  className?: string;
}) {
  const activeId = sort.startsWith("-") ? sort.slice(1) : sort;
  const desc = sort.startsWith("-");
  const sorted = activeId === columnId ? (desc ? "desc" : "asc") : false;
  const ariaSort = sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : "none";

  return (
    <th
      aria-sort={ariaSort}
      className={cn(
        "px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-(--muted)",
        className
      )}
    >
      <button
        type="button"
        onClick={() => onSort(columnId)}
        className={`inline-flex items-center gap-1 rounded-sm uppercase transition-colors duration-150 hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
          align === "right" ? "ml-auto justify-end" : ""
        }`}
      >
        <span>{children}</span>
        <span
          aria-hidden
          className={`inline-block w-3 text-center font-mono text-[10px] ${
            sorted ? "text-(--foreground)" : "text-(--muted)"
          }`}
        >
          {sorted === "asc" ? "↑" : sorted === "desc" ? "↓" : "↕"}
        </span>
      </button>
    </th>
  );
}

export function SortableTh<TData>({
  column,
  children,
  align = "left",
  className = "",
}: {
  column: Column<TData, unknown>;
  children: ReactNode;
  align?: "left" | "right";
  className?: string;
}) {
  const sorted = column.getIsSorted();
  const ariaSort = sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : "none";

  return (
    <th
      aria-sort={ariaSort}
      className={cn(
        "px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-(--muted)",
        className
      )}
    >
      <button
        type="button"
        onClick={() => column.toggleSorting(sorted === "asc")}
        className={`inline-flex items-center gap-1 rounded-sm uppercase transition-colors duration-150 hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
          align === "right" ? "ml-auto justify-end" : ""
        }`}
      >
        <span>{children}</span>
        <span
          aria-hidden
          className={`inline-block w-3 text-center font-mono text-[10px] ${
            sorted ? "text-(--foreground)" : "text-(--muted)"
          }`}
        >
          {sorted === "asc" ? "↑" : sorted === "desc" ? "↓" : "↕"}
        </span>
      </button>
    </th>
  );
}

export function Td({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return <td className={cn("px-3 py-2 align-middle", className)}>{children}</td>;
}

/* ── State / feedback ──────────────────────────────────────────────────── */

export function Dot({ tone, title }: { tone: "ok" | "error" | "warn" | "idle"; title?: string }) {
  const fill = {
    ok: "bg-(--success)",
    error: "bg-(--danger)",
    warn: "bg-(--warning)",
    idle: "border border-(--muted) bg-transparent",
  }[tone];
  return (
    <span
      className={`inline-block size-2 shrink-0 rounded-full ${fill}`}
      title={title}
      aria-label={title}
    />
  );
}

export function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    []
  );

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard unavailable (insecure context); nothing actionable to do.
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      title={copied ? m.common_copy_copied() : m.common_copy_copy({ label })}
      aria-label={copied ? m.common_copy_copied() : m.common_copy_copy({ label })}
      className={`inline-grid size-5 shrink-0 place-items-center rounded transition-[opacity,color] duration-150 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
        copied
          ? "text-(--success) opacity-100"
          : "text-(--muted) opacity-0 hover:text-(--foreground) group-hover/key:opacity-100"
      }`}
    >
      {copied ? (
        <Check className="size-3.5" aria-hidden />
      ) : (
        <Copy className="size-3.5" aria-hidden />
      )}
    </button>
  );
}

// ErrorAlert renders a danger banner, or nothing when there is no message. The
// caller supplies the margin (mb-4 / mt-4) via className — tailwind-merge does
// not dedupe mt vs mb, so no margin is baked in.
export function ErrorAlert({
  message,
  icon = false,
  className,
}: {
  message: string | null | undefined;
  icon?: boolean;
  className?: string;
}) {
  if (!message) return null;
  return (
    <div
      className={cn(
        "flex items-center gap-2 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)",
        className
      )}
      role="alert"
    >
      {icon && <Dot tone="error" />}
      <span>{message}</span>
    </div>
  );
}

// CopyableCode is a multi-line monospace block with a copy button in the corner.
export function CopyableCode({
  value,
  label,
  className,
}: {
  value: string;
  label: string;
  className?: string;
}) {
  return (
    <div className={cn("group/key relative", className)}>
      <pre className="overflow-x-auto rounded-(--radius) border border-(--border) bg-(--surface-secondary) p-3 pr-9 font-mono text-xs leading-relaxed text-(--foreground)">
        {value}
      </pre>
      <div className="absolute right-2 top-2">
        <CopyButton value={value} label={label} />
      </div>
    </div>
  );
}

export function Teaching({
  title,
  hint,
  action,
}: {
  title: string;
  hint: string;
  action?: ReactNode;
}) {
  return (
    <div className="px-6 py-12 text-center">
      <p className="text-[13px] font-medium text-(--foreground)">{title}</p>
      <p className="mx-auto mt-1 max-w-sm text-xs text-(--muted)">{hint}</p>
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}

export function PanelMessage({ children }: { children: ReactNode }) {
  return <div className="px-6 py-10 text-center text-[13px] text-(--muted)">{children}</div>;
}

export function TableSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="divide-y divide-(--separator)">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 px-3 py-[0.6875rem]">
          <span className="size-2 shrink-0 animate-pulse rounded-full bg-(--surface-secondary)" />
          <span className="h-3 w-28 animate-pulse rounded bg-(--surface-secondary)" />
          <span className="hidden h-3 w-44 animate-pulse rounded bg-(--surface-secondary) sm:block" />
          <span className="ml-auto h-3 w-16 animate-pulse rounded bg-(--surface-secondary)" />
        </div>
      ))}
    </div>
  );
}
