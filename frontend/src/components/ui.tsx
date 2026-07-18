import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import type { Column } from "@tanstack/react-table";
import { Check, Copy } from "@gravity-ui/icons";
import {
  Button,
  Card,
  Checkbox,
  CheckboxGroup,
  Description,
  FieldError,
  Label,
  ListBox,
  Modal,
  Select,
  Switch,
} from "@heroui/react";
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
      <div className="min-h-svh bg-background text-foreground">
        <header className="sticky top-0 z-20 border-b border-border bg-surface">
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
      <span className="grid size-5 place-items-center rounded-[5px] bg-accent text-[11px] font-bold text-accent-foreground">
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
      className="shrink-0 rounded-sm no-underline transition-opacity duration-150 hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
    >
      <Brand />
    </Link>
  );
}

// Centered single-card layout shared by the auth pages (login, register,
// verify, forgot-password). Pages pass their Card.Header/Content as children.
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh items-center justify-center bg-background p-4 text-foreground">
      <Card className="w-full max-w-sm border bg-surface">{children}</Card>
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
          <h2 className="shrink-0 text-[13px] font-semibold text-foreground">{title}</h2>
          {meta && <span className="min-w-0 truncate text-xs tabular-nums text-muted">{meta}</span>}
        </div>
        {action && <div className="min-w-0 w-full sm:w-auto">{action}</div>}
      </div>
      <div className="overflow-hidden rounded-lg border bg-surface">{children}</div>
    </section>
  );
}

/* ── Form controls ─────────────────────────────────────────────────────── */

// A boolean toggle laid out the HeroUI v3 way: the switch sits inline to the
// left of its label, with the description below. Label text lives inside
// Switch.Content (the clickable <label>, so it's the accessible name), and
// Description is a sibling that React Aria wires up as aria-describedby.
export function LabeledSwitch({
  label,
  description,
  isSelected,
  isDisabled = false,
  onChange,
}: {
  label: string;
  description?: string;
  isSelected: boolean;
  isDisabled?: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <Switch isSelected={isSelected} isDisabled={isDisabled} onChange={onChange}>
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
        {label}
      </Switch.Content>
      {description && <Description>{description}</Description>}
    </Switch>
  );
}

// A single-select field built on HeroUI Select so forms share one trigger,
// popover, focus ring, and disabled treatment instead of a hand-styled native
// <select>. value/onChange are plain option ids.
export function SelectField({
  variant,
  label,
  value,
  onChange,
  options,
  description,
}: {
  variant?: "secondary" | "primary";
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Array<{ value: string; label: string }>;
  description?: string;
}) {
  return (
    <Select
      variant={variant}
      value={value}
      onChange={(key) => onChange(String(Array.isArray(key) ? key[0] : key))}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item key={option.value} id={option.value} textValue={option.label}>
              {option.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
      {description ? <Description>{description}</Description> : null}
    </Select>
  );
}

// A multi-select relation picker built on HeroUI CheckboxGroup — the shared
// replacement for a native <select multiple>. Selection is visible at a glance
// and keyboard/screen-reader friendly. Options may carry a muted note (e.g. a
// disabled channel) and the list scrolls once it grows past a few rows.
export function CheckboxListField({
  variant,
  label,
  values,
  onChange,
  options,
  description,
  emptyLabel,
  isInvalid = false,
  errorMessage,
}: {
  variant?: "secondary" | "primary";
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  options: Array<{ id: string; name: string; disabled?: boolean; note?: string }>;
  description?: string;
  emptyLabel?: string;
  isInvalid?: boolean;
  errorMessage?: string;
}) {
  return (
    <CheckboxGroup variant={variant} value={values} onChange={onChange} isInvalid={isInvalid}>
      <Label>{label}</Label>
      {description ? <Description>{description}</Description> : null}

      {options.length === 0 ? (
        <p className="">{emptyLabel}</p>
      ) : (
        <div className="mt-1 flex flex-col gap-4 border border-field-border border-solid rounded-lg px-4 py-3 max-h-40 overflow-y-auto">
          {options.map((option) => (
            <Checkbox
              key={option.id}
              value={option.id}
              isDisabled={option.disabled}
              className="mt-0"
            >
              <Checkbox.Content className="">
                <Checkbox.Control className="">
                  <Checkbox.Indicator />
                </Checkbox.Control>
                {option.name}
              </Checkbox.Content>
              {option.note ? <Description>{option.note}</Description> : null}
            </Checkbox>
          ))}
        </div>
      )}
      {isInvalid && errorMessage ? <FieldError>{errorMessage}</FieldError> : null}
    </CheckboxGroup>
  );
}

/* ── Tables ────────────────────────────────────────────────────────────── */

export function Th({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return (
    <th
      className={cn(
        "px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-muted",
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
        "px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-muted",
        className
      )}
    >
      <button
        type="button"
        onClick={() => onSort(columnId)}
        className={`inline-flex items-center gap-1 rounded-sm uppercase transition-colors duration-150 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus ${
          align === "right" ? "ml-auto justify-end" : ""
        }`}
      >
        <span>{children}</span>
        <span
          aria-hidden
          className={`inline-block w-3 text-center font-mono text-[10px] ${
            sorted ? "text-foreground" : "text-muted"
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
        "px-3 py-2 text-[11px] font-medium uppercase tracking-wider text-muted",
        className
      )}
    >
      <button
        type="button"
        onClick={() => column.toggleSorting(sorted === "asc")}
        className={`inline-flex items-center gap-1 rounded-sm uppercase transition-colors duration-150 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus ${
          align === "right" ? "ml-auto justify-end" : ""
        }`}
      >
        <span>{children}</span>
        <span
          aria-hidden
          className={`inline-block w-3 text-center font-mono text-[10px] ${
            sorted ? "text-foreground" : "text-muted"
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
    ok: "bg-success",
    error: "bg-danger",
    warn: "bg-warning",
    idle: "border border-muted bg-transparent",
  }[tone];
  return (
    <span
      className={`inline-block size-2 shrink-0 rounded-full ${fill}`}
      title={title}
      aria-label={title}
    />
  );
}

export function SeverityBadge({
  severity,
  label,
  className = "",
}: {
  severity: "warning" | "critical";
  label: string;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-5 shrink-0 items-center rounded-[4px] border px-1.5 text-[10px] font-semibold leading-none",
        severity === "critical"
          ? "border-danger/30 bg-danger-soft text-danger-soft-foreground"
          : "border-warning/30 bg-warning-soft text-warning-soft-foreground",
        className
      )}
    >
      {label}
    </span>
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
      className={cn(
        "inline-grid size-5 shrink-0 place-items-center rounded transition-[opacity,color] duration-150 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus",
        copied
          ? "text-success opacity-100"
          : "text-muted opacity-0 hover:text-foreground group-hover:opacity-100"
      )}
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
        "flex items-center gap-2 rounded-lg border bg-danger-soft px-3 py-2 text-[13px] text-danger-soft-foreground",
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
      <pre className="overflow-x-auto rounded-lg border bg-surface-secondary p-3 pr-9 font-mono text-xs leading-relaxed text-foreground">
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
      <p className="text-[13px] font-medium text-foreground">{title}</p>
      <p className="mx-auto mt-1 max-w-sm text-xs text-muted">{hint}</p>
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}

export function PanelMessage({ children }: { children: ReactNode }) {
  return <div className="px-6 py-10 text-center text-[13px] text-muted">{children}</div>;
}

export function TableSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="divide-y divide-separator">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 px-3 py-[0.6875rem]">
          <span className="size-2 shrink-0 animate-pulse rounded-full bg-surface-secondary" />
          <span className="h-3 w-28 animate-pulse rounded bg-surface-secondary" />
          <span className="hidden h-3 w-44 animate-pulse rounded bg-surface-secondary sm:block" />
          <span className="ml-auto h-3 w-16 animate-pulse rounded bg-surface-secondary" />
        </div>
      ))}
    </div>
  );
}

export function DestructiveConfirmModal({
  isOpen,
  title,
  body,
  confirmLabel,
  pendingLabel,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: {
  isOpen: boolean;
  title: string;
  body: string;
  confirmLabel: string;
  pendingLabel: string;
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
            <p className="mt-1.5 text-sm leading-5 text-muted">{body}</p>
          </Modal.Header>
          {error ? (
            <Modal.Body>
              <p className="text-[13px] text-danger" role="alert">
                {error}
              </p>
            </Modal.Body>
          ) : null}
          <Modal.Footer>
            <Button size="sm" variant="secondary" onPress={() => onOpenChange(false)}>
              {m.common_cancel()}
            </Button>
            <Button
              size="sm"
              variant="primary"
              isPending={pending}
              onPress={onConfirm}
              className="bg-danger text-danger-foreground hover:opacity-90"
            >
              {pending ? pendingLabel : confirmLabel}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
