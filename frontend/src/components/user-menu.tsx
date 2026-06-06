import { useNavigate } from "@tanstack/react-router";
import { Button, Dropdown, Label, Separator } from "@heroui/react";
import type { Key } from "react";
import { clearAuth, type Auth } from "~/api/auth";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";

export function UserMenu({ auth }: { auth: Auth }) {
  const navigate = useNavigate();
  const origin = usePanelApiOrigin();
  const showPbAdmin = auth.user.role === "admin" && origin.length > 0;

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
        className="max-w-[180px] truncate"
      >
        <span className="sm:hidden">Account</span>
        <span className="hidden truncate sm:inline">{auth.user.email}</span>
      </Button>
      <Dropdown.Popover placement="bottom end">
        <Dropdown.Menu onAction={handleAction}>
          <Dropdown.Item id="account" textValue="Your account">
            <Label>Your account</Label>
          </Dropdown.Item>
          {showPbAdmin && (
            <Dropdown.Item id="pb-admin" textValue="PocketBase admin">
              <Label>PocketBase admin</Label>
            </Dropdown.Item>
          )}
          {auth.user.role === "admin" && (
            <Dropdown.Item id="database" textValue="Database management">
              <Label>Database management</Label>
            </Dropdown.Item>
          )}
          <Separator />
          <Dropdown.Item id="sign-out" variant="danger" textValue="Sign out">
            <Label>Sign out</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
