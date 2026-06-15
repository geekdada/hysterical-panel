import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Description, Label, Switch } from "@heroui/react";
import { ChevronRight, Code, Database } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import {
  canQueryPanelApi,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  rotateManagementApiToken,
  updateSettings,
  type AppSettings,
  type SettingsUpdateRequest,
} from "~/api/queries";
import { BackLink, CopyableCode, ErrorAlert, PageShell } from "~/components/ui";
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
    <PageShell
      width="narrow"
      headerLeft={
        <div className="flex min-w-0 items-center gap-3">
          <BackLink />
          <span className="truncate text-[13px] font-semibold tracking-tight">Settings</span>
        </div>
      }
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-5">
        <h1 className="text-base font-semibold tracking-tight">Registration</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">Control who can create accounts.</p>
      </div>

      <ErrorAlert message={loadError} className="mb-4" />

      <div className="flex flex-col gap-1 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
        <SettingSwitch
          label="Invitation system"
          description="Generate invite codes that let people sign up."
          isSelected={settings?.invitations_enabled ?? false}
          isDisabled={!settings || mutation.isPending}
          onChange={(v) => patch("invitations_enabled", v)}
        />
        <div className="my-1 h-px bg-(--separator)" />
        <SettingSwitch
          label="Open registration"
          description="Anyone can sign up without an invite code."
          isSelected={settings?.open_registration ?? false}
          isDisabled={!settings || mutation.isPending}
          onChange={(v) => patch("open_registration", v)}
        />
        <div className="my-1 h-px bg-(--separator)" />
        <SettingSwitch
          label="Require invite code"
          description="Open sign-ups still need a valid code. Requires the invitation system."
          isSelected={settings?.require_invite_for_open ?? false}
          isDisabled={!settings || mutation.isPending || !(settings?.invitations_enabled ?? false)}
          onChange={(v) => patch("require_invite_for_open", v)}
        />
      </div>

      {settings?.open_registration && !settings.require_invite_for_open && (
        <p className="mt-3 text-xs text-(--muted)">
          Sign-ups without a code must verify their email, which needs SMTP set up in the PocketBase
          admin. Until then, those sign-ups are rejected.
        </p>
      )}

      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">Database</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">
          Keep traffic data from piling up over time.
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
            View storage usage and prune old data points.
          </span>
        </span>
        <ChevronRight
          className="size-4 shrink-0 text-(--muted) transition-colors duration-150 group-hover:text-(--foreground)"
          aria-hidden
        />
      </Link>

      <ManagementApiSection
        settings={settings}
        pending={mutation.isPending}
        onSave={(patch) => mutation.mutate(patch)}
        error={saveError}
        newToken={mutation.data?.management_api_token}
      />
    </PageShell>
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

function ManagementApiSection({
  settings,
  pending,
  error,
  onSave,
  newToken,
}: {
  settings: AppSettings | undefined;
  pending: boolean;
  error: string;
  onSave: (patch: SettingsUpdateRequest) => void;
  newToken?: string;
}) {
  const queryClient = useQueryClient();
  const [revealedToken, setRevealedToken] = useState<string | null>(null);

  const enabled = settings?.management_api_enabled ?? false;
  const tokenSet = settings?.management_api_token_set ?? false;

  // Surface a freshly generated/rotated token exactly once.
  useEffect(() => {
    if (newToken) setRevealedToken(newToken);
  }, [newToken]);

  const rotateMutation = useMutation({
    mutationFn: () => rotateManagementApiToken(),
    onSuccess: (data) => {
      if (data.management_api_token) setRevealedToken(data.management_api_token);
      void queryClient.invalidateQueries({ queryKey: queryKeys.settings() });
    },
  });

  function toggleEnabled(v: boolean) {
    onSave({ management_api_enabled: v });
  }

  return (
    <>
      <div className="mt-8 mb-5">
        <h1 className="text-base font-semibold tracking-tight">Management API</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">
          Let external services manage resources on this platform.
        </p>
      </div>

      <div className="flex flex-col gap-4 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
        <SettingSwitch
          label="Enable management API"
          description={
            enabled
              ? "The /api/mgmt/* endpoints are served, authenticated by a server-generated token."
              : "Turning this on generates a token shown once. Copy it before leaving this page."
          }
          isSelected={enabled}
          isDisabled={!settings || pending}
          onChange={toggleEnabled}
        />

        {tokenSet && (
          <>
            <div className="h-px bg-(--separator)" />

            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 text-[13px]">
                <span className="text-success font-semibold">Configured</span>
              </div>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={pending || rotateMutation.isPending}
                onPress={() => rotateMutation.mutate()}
              >
                {rotateMutation.isPending ? "Rotating…" : "Rotate token"}
              </Button>
            </div>
          </>
        )}
      </div>

      {revealedToken && (
        <div className="mt-4 rounded-(--radius) border border-(--warning) bg-(--warning-soft) px-4 py-3">
          <p className="text-[13px] font-semibold text-(--warning-soft-foreground)">
            Copy your token now. It won't be shown again.
          </p>
          <CopyableCode value={revealedToken} label="token" className="mt-2" />
        </div>
      )}

      <Link
        to="/management-api"
        className="group mt-3 flex items-center gap-3 rounded-(--radius) border border-(--border) bg-(--surface) px-4 py-3.5 transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-(--radius) border border-(--border) bg-(--surface-secondary) text-(--muted)">
          <Code className="size-4" aria-hidden />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[13px] font-medium text-(--foreground)">API reference</span>
          <span className="block text-xs text-(--muted)">
            Endpoints with request and response examples.
          </span>
        </span>
        <ChevronRight
          className="size-4 shrink-0 text-(--muted) transition-colors duration-150 group-hover:text-(--foreground)"
          aria-hidden
        />
      </Link>

      <ErrorAlert message={error} className="mt-4" />

      <ErrorAlert
        className="mt-4"
        message={
          rotateMutation.error
            ? queryErrorMessage(
                rotateMutation.error,
                "Network error while rotating the management API token."
              )
            : ""
        }
      />
    </>
  );
}
