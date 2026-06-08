import { useNavigate } from "@tanstack/react-router";
import { Button, Dropdown, Label, Separator } from "@heroui/react";
import type { Key } from "react";
import { clearAuth, type Auth } from "~/api/auth";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";

export function UserMenu({ auth }: { auth: Auth }) {
  const navigate = useNavigate();
  const origin = usePanelApiOrigin();
  const showPbAdmin = auth.user.role === "admin" && origin.length > 0;
  const initial = auth.user.email.trim().charAt(0).toUpperCase() || "U";
  const roleLabel = auth.user.role === "admin" ? "Admin" : "User";
  const statusLabel = auth.user.status === "active" ? "Active" : "Disabled";
  const itemClass =
    "h-7 rounded-md px-2 text-[13px] text-(--foreground) transition-colors duration-150 hover:bg-(--surface-secondary) data-[focus=true]:bg-(--surface-secondary) data-[hovered=true]:bg-(--surface-secondary)";
  const dangerItemClass =
    "h-7 rounded-md px-2 text-[13px] text-(--danger) transition-colors duration-150 hover:bg-(--danger-soft) data-[focus=true]:bg-(--danger-soft) data-[hovered=true]:bg-(--danger-soft)";
  const iconClass = "size-3.5 shrink-0 text-(--muted)";

  function handleAction(key: Key) {
    if (key === "account") {
      navigate({ to: "/users/$userId", params: { userId: auth.user.id } });
      return;
    }
    if (key === "pb-admin") {
      window.open(`${origin}/_/`, "_blank", "noopener,noreferrer");
      return;
    }
    if (key === "database") {
      navigate({ to: "/database" });
      return;
    }
    if (key === "analytics") {
      navigate({ to: "/analytics" });
      return;
    }
    if (key === "sign-out") {
      clearAuth();
      window.location.href = "/login";
    }
  }

  return (
    <Dropdown>
      <Button
        variant="ghost"
        size="sm"
        aria-label="Account menu"
        className="h-8 max-w-[220px] gap-1.5 rounded-(--radius) border border-transparent px-1.5 pr-2 text-[13px] font-medium text-(--foreground) transition-colors duration-150 hover:border-(--border) hover:bg-(--surface-secondary)"
      >
        <AvatarInitial value={initial} />
        <span className="min-w-0 truncate sm:hidden">Account</span>
        <span className="hidden min-w-0 truncate sm:inline">
          {auth.user.email}
        </span>
        <ChevronDownIcon className="size-3 shrink-0 text-(--muted)" />
      </Button>
      <Dropdown.Popover
        placement="bottom end"
        className="min-w-64 rounded-(--radius) border border-(--border) bg-(--surface) p-1 shadow-none"
      >
        <div className="px-2 py-2">
          <div className="flex min-w-0 items-center gap-2">
            <AvatarInitial value={initial} />
            <div className="min-w-0">
              <div className="truncate text-[13px] font-medium text-(--foreground)">
                {auth.user.email}
              </div>
              <div className="mt-0.5 flex items-center gap-1.5 text-[11px] leading-none text-(--muted)">
                <span>{roleLabel}</span>
                <span className="size-1 rounded-full bg-(--separator)" />
                <span className="inline-flex items-center gap-1">
                  <span
                    className={`size-1.5 rounded-full ${
                      auth.user.status === "active"
                        ? "bg-(--success)"
                        : "bg-(--muted)"
                    }`}
                  />
                  {statusLabel}
                </span>
              </div>
            </div>
          </div>
        </div>
        <Separator className="my-1 bg-(--separator)" />
        <Dropdown.Menu onAction={handleAction}>
          <Dropdown.Item
            id="account"
            textValue="Your account"
            className={itemClass}
          >
            <AccountIcon className={iconClass} />
            <Label className="truncate">Your account</Label>
          </Dropdown.Item>
          {showPbAdmin && (
            <Dropdown.Item
              id="pb-admin"
              textValue="PocketBase admin"
              className={itemClass}
            >
              <AdminIcon className={iconClass} />
              <Label className="truncate">PocketBase admin</Label>
            </Dropdown.Item>
          )}
          {auth.user.role === "admin" && (
            <Dropdown.Item
              id="analytics"
              textValue="Analytics"
              className={itemClass}
            >
              <AnalyticsIcon className={iconClass} />
              <Label className="truncate">Analytics</Label>
            </Dropdown.Item>
          )}
          {auth.user.role === "admin" && (
            <Dropdown.Item
              id="database"
              textValue="Database management"
              className={itemClass}
            >
              <DatabaseIcon className={iconClass} />
              <Label className="truncate">Database management</Label>
            </Dropdown.Item>
          )}
          <Separator className="my-1 bg-(--separator)" />
          <Dropdown.Item
            id="sign-out"
            variant="danger"
            textValue="Sign out"
            className={dangerItemClass}
          >
            <SignOutIcon className="size-3.5 shrink-0 text-current" />
            <Label className="truncate">Sign out</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
        <div className="mt-1 flex items-center justify-between gap-3 border-t border-(--separator) px-2 pt-1.5 pb-0.5 text-xs leading-4 text-muted">
          <span className="font-mono tabular-nums">v{__APP_VERSION__}</span>
        </div>
      </Dropdown.Popover>
    </Dropdown>
  );
}

function AvatarInitial({ value }: { value: string }) {
  return (
    <span className="grid size-5 shrink-0 place-items-center rounded-[5px] border border-(--border) bg-(--surface-secondary) text-[11px] font-semibold uppercase text-(--foreground)">
      {value}
    </span>
  );
}

function ChevronDownIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m4 6 4 4 4-4" />
    </svg>
  );
}

function AccountIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M8 8.25a2.75 2.75 0 1 0 0-5.5 2.75 2.75 0 0 0 0 5.5Z" />
      <path d="M3.25 13.25c.72-1.86 2.34-3 4.75-3s4.03 1.14 4.75 3" />
    </svg>
  );
}

function AdminIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M8 2.25 12.25 4v3.35c0 2.7-1.67 5.1-4.25 6.4-2.58-1.3-4.25-3.7-4.25-6.4V4L8 2.25Z" />
      <path d="m6.4 8 1.1 1.1 2.25-2.35" />
    </svg>
  );
}

function AnalyticsIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M3 12.75h10" />
      <path d="M4.25 10.5V7.75" />
      <path d="M8 10.5V4.25" />
      <path d="M11.75 10.5V6" />
    </svg>
  );
}

function DatabaseIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M3 4.25c0-1.1 2.24-2 5-2s5 .9 5 2-2.24 2-5 2-5-.9-5-2Z" />
      <path d="M3 4.25v3.5c0 1.1 2.24 2 5 2s5-.9 5-2v-3.5" />
      <path d="M3 7.75v3.75c0 1.1 2.24 2 5 2s5-.9 5-2V7.75" />
    </svg>
  );
}

function SignOutIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M6.5 13.25h-2.5A1.25 1.25 0 0 1 2.75 12V4A1.25 1.25 0 0 1 4 2.75h2.5" />
      <path d="M10.25 11.25 13.5 8l-3.25-3.25" />
      <path d="M13.25 8H6.5" />
    </svg>
  );
}
