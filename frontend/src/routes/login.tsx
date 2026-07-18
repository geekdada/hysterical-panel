import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, redirect } from "@tanstack/react-router";
import { Button, Card, Input, Label, TextField } from "@heroui/react";
import { login, loginWithPasskey } from "~/api/auth";
import { panelConfigQueryOptions } from "~/api/queries";
import { AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import { defaultDashboardSearch } from "~/lib/dashboard-search";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/login")({
  beforeLoad: ({ context }) => {
    if (context.auth) {
      throw redirect({ to: "/", search: defaultDashboardSearch() });
    }
  },
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(panelConfigQueryOptions()).catch(() => undefined);
  },
  component: LoginPage,
});

function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const loginMutation = useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      login(email, password),
    onSuccess: () => {
      window.location.href = "/";
    },
  });
  const passkeyMutation = useMutation({
    mutationFn: () => loginWithPasskey(false),
    onSuccess: () => {
      window.location.href = "/";
    },
  });
  const configQuery = useQuery(panelConfigQueryOptions());
  const canRegister = configQuery.data?.registration_open ?? false;
  const passkeysEnabled = configQuery.data?.passkeys_enabled ?? false;

  useEffect(() => {
    if (!passkeysEnabled) return;
    let cancelled = false;

    async function run() {
      if (typeof window === "undefined") return;
      try {
        const { browserSupportsWebAuthnAutofill } = await import("@simplewebauthn/browser");
        if (!(await browserSupportsWebAuthnAutofill())) return;
        await loginWithPasskey(true);
        if (!cancelled) {
          window.location.href = "/";
        }
      } catch {
        // Password login remains available; avoid surfacing background failures.
      }
    }

    void run();
    return () => {
      cancelled = true;
      void import("@simplewebauthn/browser").then(({ WebAuthnAbortService }) => {
        WebAuthnAbortService.cancelCeremony();
      });
    };
  }, [passkeysEnabled]);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    loginMutation.mutate({ email, password });
  }

  const error =
    loginMutation.error instanceof Error
      ? localizeApiError(loginMutation.error.message)
      : loginMutation.error
        ? m.auth_login_failed()
        : passkeyMutation.error instanceof Error
          ? localizeApiError(passkeyMutation.error.message)
          : passkeyMutation.error
            ? m.auth_passkey_login_failed()
            : "";

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-accent text-sm font-bold text-accent-foreground">
          H
        </span>
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.app_title()}
        </Card.Title>
        <Card.Description className="text-[13px] text-muted">
          {m.auth_sign_in_continue()}
        </Card.Description>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <TextField>
            <Label>{m.auth_email()}</Label>
            <Input
              type="email"
              autoComplete="username webauthn"
              required
              value={email}
              onChange={(e) => {
                loginMutation.reset();
                setEmail(e.target.value);
              }}
            />
          </TextField>
          <TextField>
            <Label>{m.auth_password()}</Label>
            <Input
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => {
                loginMutation.reset();
                setPassword(e.target.value);
              }}
            />
          </TextField>
          <div className="-mt-1 flex justify-end">
            <Link to="/forgot-password" className="text-[13px] text-muted hover:text-foreground">
              {m.auth_forgot_password()}
            </Link>
          </div>

          {error && (
            <p className="text-sm text-danger" role="alert">
              {error}
            </p>
          )}

          <Button
            type="submit"
            variant="primary"
            fullWidth
            isDisabled={loginMutation.isPending}
            className="mt-1"
          >
            {loginMutation.isPending ? m.auth_signing_in() : m.auth_sign_in()}
          </Button>
          {passkeysEnabled && (
            <>
              <div className="relative my-1 flex items-center justify-center">
                <span className="absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-separator" />
                <span className="relative bg-surface px-2 text-[12px] text-muted">
                  {m.auth_or()}
                </span>
              </div>
              <Button
                type="button"
                variant="secondary"
                fullWidth
                isDisabled={passkeyMutation.isPending || loginMutation.isPending}
                onPress={() => {
                  loginMutation.reset();
                  passkeyMutation.mutate();
                }}
              >
                {passkeyMutation.isPending ? m.auth_checking_passkeys() : m.auth_sign_in_passkey()}
              </Button>
            </>
          )}
          {canRegister && (
            <Link
              to="/register"
              className="text-center text-[13px] text-muted hover:text-foreground"
            >
              {m.auth_create_account_link()}
            </Link>
          )}
        </form>
      </Card.Content>
    </AuthShell>
  );
}
