import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Description, Label, Switch } from "@heroui/react";
import { ChevronLeft, ChevronRight, Database } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  canQueryPanelApi,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  updateSettings,
  type AppSettings,
  type SettingsUpdateRequest,
} from "~/api/queries";
import { UserMenu } from "~/components/user-menu";

export const Route = createFileRoute("/settings")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: SettingsPage,
});

function SettingsPage() {
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();

  const settingsQuery = useQuery({
    queryKey: queryKeys.settings(),
    queryFn: fetchSettings,
    enabled: canQueryPanelApi(),
  });

  const mutation = useMutation({
    mutationFn: (patch: SettingsUpdateRequest) => updateSettings(patch),
    onSuccess: (data) => {
      queryClient.setQueryData(queryKeys.settings(), data);
      void queryClient.invalidateQueries({ queryKey: queryKeys.config() });
    },
  });

  const settings = settingsQuery.data;
  const loadError = settingsQuery.error ? queryErrorMessage(settingsQuery.error) : "";
  const saveError = mutation.error
    ? queryErrorMessage(mutation.error, "Network error while updating settings.")
    : "";

  function patch(field: keyof AppSettings, value: boolean) {
    mutation.mutate({ [field]: value });
  }

  return (
    <div className="min-h-svh bg-(--background) text-(--foreground)">
      <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
        <div className="mx-auto flex h-12 max-w-3xl items-center justify-between px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-2.5">
            <Link
              to="/"
              aria-label="Back to dashboard"
              className="grid size-6 shrink-0 place-items-center rounded text-(--muted) transition-colors duration-150 hover:bg-(--surface-secondary) hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
            >
              <ChevronLeft className="size-3.5" aria-hidden />
            </Link>
            <span className="truncate text-[13px] font-semibold tracking-tight">Settings</span>
          </div>
          {auth && <UserMenu auth={auth} />}
        </div>
      </header>

      <main className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
        <div className="mb-5">
          <h1 className="text-base font-semibold tracking-tight">Registration</h1>
          <p className="mt-0.5 text-[13px] text-(--muted)">
            Control who can create accounts. Changes take effect immediately.
          </p>
        </div>

        {loadError && (
          <div
            className="mb-4 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            {loadError}
          </div>
        )}

        <div className="flex flex-col gap-1 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
          <SettingSwitch
            label="Invitation system"
            description="Let admins generate invite codes that allow people to register."
            isSelected={settings?.invitations_enabled ?? false}
            isDisabled={!settings || mutation.isPending}
            onChange={(v) => patch("invitations_enabled", v)}
          />
          <div className="my-1 h-px bg-(--separator)" />
          <SettingSwitch
            label="Open registration"
            description="Let anyone create an account without an invite code."
            isSelected={settings?.open_registration ?? false}
            isDisabled={!settings || mutation.isPending}
            onChange={(v) => patch("open_registration", v)}
          />
          <div className="my-1 h-px bg-(--separator)" />
          <SettingSwitch
            label="Require invite code for open registration"
            description="When open registration is on, still require a valid invite code. Needs the invitation system enabled."
            isSelected={settings?.require_invite_for_open ?? false}
            isDisabled={
              !settings || mutation.isPending || !(settings?.invitations_enabled ?? false)
            }
            onChange={(v) => patch("require_invite_for_open", v)}
          />
        </div>

        {settings?.open_registration && !settings.require_invite_for_open && (
          <p className="mt-3 text-xs text-(--muted)">
            Open registration without an invite code requires email verification, which needs SMTP
            configured in the PocketBase admin. Until SMTP is set up, those sign-ups are rejected.
          </p>
        )}

        {saveError && (
          <div
            className="mt-4 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            {saveError}
          </div>
        )}

        <div className="mt-8 mb-5">
          <h1 className="text-base font-semibold tracking-tight">Database</h1>
          <p className="mt-0.5 text-[13px] text-(--muted)">
            Inspect storage usage and prune old traffic data.
          </p>
        </div>

        <Link
          to="/database"
          className="group flex items-center gap-3 rounded-(--radius) border border-(--border) bg-(--surface) px-4 py-3.5 transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
        >
          <span className="grid size-8 shrink-0 place-items-center rounded-(--radius) border border-(--border) bg-(--surface-secondary) text-(--muted)">
            <Database className="size-4" aria-hidden />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-[13px] font-medium text-(--foreground)">
              Database management
            </span>
            <span className="block text-xs text-(--muted)">
              Storage footprint, traffic data points, and maintenance.
            </span>
          </span>
          <ChevronRight
            className="size-4 shrink-0 text-(--muted) transition-colors duration-150 group-hover:text-(--foreground)"
            aria-hidden
          />
        </Link>
      </main>
    </div>
  );
}

function SettingSwitch({
  label,
  description,
  isSelected,
  isDisabled,
  onChange,
}: {
  label: string;
  description: string;
  isSelected: boolean;
  isDisabled: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <Switch
      isSelected={isSelected}
      isDisabled={isDisabled}
      onChange={onChange}
      className="justify-between gap-4 py-1.5"
    >
      <Switch.Content>
        <Label>{label}</Label>
        <Description>{description}</Description>
      </Switch.Content>
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  );
}
