import { useNavigate } from "@tanstack/react-router";
import { Button, Dropdown, Label, Separator } from "@heroui/react";
import {
  ArrowRightFromSquare,
  ChartColumn,
  ChevronDown,
  Database,
  Person,
  ShieldCheck,
} from "@gravity-ui/icons";
import type { Key } from "react";
import { clearAuth, type Auth } from "~/api/auth";
import { ThemeToggle } from "~/components/theme-toggle";
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
        <ChevronDown className="size-3 shrink-0 text-(--muted)" aria-hidden />
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
            <Person className={iconClass} aria-hidden />
            <Label className="truncate">Your account</Label>
          </Dropdown.Item>
          {showPbAdmin && (
            <Dropdown.Item
              id="pb-admin"
              textValue="PocketBase admin"
              className={itemClass}
            >
              <ShieldCheck className={iconClass} aria-hidden />
              <Label className="truncate">PocketBase admin</Label>
            </Dropdown.Item>
          )}
          {auth.user.role === "admin" && (
            <Dropdown.Item
              id="analytics"
              textValue="Analytics"
              className={itemClass}
            >
              <ChartColumn className={iconClass} aria-hidden />
              <Label className="truncate">Analytics</Label>
            </Dropdown.Item>
          )}
          {auth.user.role === "admin" && (
            <Dropdown.Item
              id="database"
              textValue="Database management"
              className={itemClass}
            >
              <Database className={iconClass} aria-hidden />
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
            <ArrowRightFromSquare
              className="size-3.5 shrink-0 text-current"
              aria-hidden
            />
            <Label className="truncate">Sign out</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
        <Separator className="my-1 bg-(--separator)" />
        <div className="flex items-center justify-between gap-3 px-2 py-1">
          <span className="text-[13px] text-(--foreground)">Theme</span>
          <ThemeToggle />
        </div>
        <div className="mt-1 flex items-center justify-between gap-3 border-t border-(--separator) text-[10px] px-2 pt-1.5 pb-0.5 text-xs leading-4 text-muted/80">
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
