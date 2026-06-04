import { useEffect, useRef, useState, type ReactNode } from "react";

/* ── Layout ────────────────────────────────────────────────────────────── */

export function Section({
  title,
  meta,
  action,
  children,
}: {
  title: string;
  meta?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="mt-6">
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
      className={`px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-(--muted) ${className}`}
    >
      {children}
    </th>
  );
}

export function Td({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return <td className={`px-3 py-2 align-middle ${className}`}>{children}</td>;
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
    [],
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
      title={copied ? "Copied" : `Copy ${label}`}
      aria-label={copied ? "Copied" : `Copy ${label}`}
      className={`inline-grid size-5 shrink-0 place-items-center rounded transition-[opacity,color] duration-150 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
        copied
          ? "text-(--success) opacity-100"
          : "text-(--muted) opacity-0 hover:text-(--foreground) group-hover/key:opacity-100"
      }`}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
    </button>
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
  return (
    <div className="px-6 py-10 text-center text-[13px] text-(--muted)">{children}</div>
  );
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

/* ── Icons ─────────────────────────────────────────────────────────────── */

export function CopyIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V5a2 2 0 0 1 2-2h10" />
    </svg>
  );
}

export function CheckIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}
