import { useState, type FormEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, redirect } from "@tanstack/react-router";
import { Button, Card, Input, Label, TextField } from "@heroui/react";
import { login, loginWithPasskey, startOAuthLogin } from "~/api/auth";
import { loginMethodsQueryOptions, panelConfigQueryOptions } from "~/api/queries";
import { EmailCodeLoginForm, useEmailCodeLogin } from "~/components/email-code-login";
import { OAuthProviderIcon } from "~/components/oauth-provider-icon";
import { AuthBrandMark, AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import { defaultDashboardSearch } from "~/lib/dashboard-search";
import { safeRedirectPath } from "~/lib/safe-redirect";
import { useAppName } from "~/lib/use-app-name";
import * as m from "~/paraglide/messages.js";

interface LoginSearch {
  redirect?: string;
  reauth?: true;
}

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: safeRedirectPath(search.redirect),
    reauth: search.reauth === true || search.reauth === "true" ? true : undefined,
  }),
  beforeLoad: ({ context, search }) => {
    // A Connected App that asked for a fresh sign-in needs the form even
    // when a panel session exists.
    if (context.auth && !search.reauth) {
      if (search.redirect) throw redirect({ href: search.redirect });
      throw redirect({ to: "/", search: defaultDashboardSearch() });
    }
  },
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(panelConfigQueryOptions()).catch(() => undefined);
  },
  component: LoginPage,
});

type LoginMethod = "password" | "email-code";

function LoginPage() {
  const appName = useAppName();
  const search = Route.useSearch();
  const destination = search.redirect ?? "/";
  const [method, setMethod] = useState<LoginMethod>("password");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const goToDestination = () => {
    window.location.href = destination;
  };
  const loginMutation = useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      login(email, password),
    onSuccess: goToDestination,
  });
  const passkeyMutation = useMutation({
    mutationFn: () => loginWithPasskey(),
    onSuccess: goToDestination,
  });
  const oauthMutation = useMutation({
    mutationFn: (providerName: string) => startOAuthLogin(providerName, search.redirect),
  });
  const emailCode = useEmailCodeLogin(goToDestination);
  const configQuery = useQuery(panelConfigQueryOptions());
  const loginMethodsQuery = useQuery(loginMethodsQueryOptions());
  const canRegister = configQuery.data?.registration_open ?? false;
  const passkeysEnabled = configQuery.data?.passkeys_enabled ?? false;
  const emailCodeEnabled = loginMethodsQuery.data?.emailCode.enabled ?? false;
  const oauthProviders = loginMethodsQuery.data?.oauthProviders ?? [];
  const hasAlternativeLogins = passkeysEnabled || emailCodeEnabled || oauthProviders.length > 0;
  const anyLoginPending =
    loginMutation.isPending ||
    passkeyMutation.isPending ||
    oauthMutation.isPending ||
    emailCode.isPending;

  function resetLoginErrors() {
    loginMutation.reset();
    passkeyMutation.reset();
    oauthMutation.reset();
    emailCode.resetErrors();
  }

  function switchMethod(next: LoginMethod) {
    resetLoginErrors();
    setMethod(next);
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    loginMutation.mutate({ email, password });
  }

  const passwordError =
    loginMutation.error instanceof Error
      ? localizeApiError(loginMutation.error.message)
      : loginMutation.error
        ? m.auth_login_failed()
        : "";
  const alternativeError =
    passkeyMutation.error instanceof Error
      ? localizeApiError(passkeyMutation.error.message)
      : passkeyMutation.error
        ? m.auth_passkey_login_failed()
        : oauthMutation.error instanceof Error
          ? localizeApiError(oauthMutation.error.message)
          : oauthMutation.error
            ? m.auth_oauth_login_failed()
            : "";

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {appName}
        </Card.Title>
        <Card.Description className="text-[13px] text-muted">
          {search.reauth ? m.auth_reauth_desc() : m.auth_sign_in_continue()}
        </Card.Description>
      </Card.Header>
      <Card.Content className="flex flex-col gap-4 px-6 pt-4 pb-6">
        {method === "email-code" && emailCodeEnabled ? (
          <EmailCodeLoginForm
            state={emailCode}
            initialEmail={email}
            durationSeconds={loginMethodsQuery.data?.emailCode.durationSeconds ?? 0}
            disabled={anyLoginPending}
          />
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <TextField>
              <Label>{m.auth_email()}</Label>
              <Input
                type="email"
                autoComplete="username"
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

            {passwordError && (
              <p className="text-sm text-danger" role="alert">
                {passwordError}
              </p>
            )}

            <Button
              type="submit"
              variant="primary"
              fullWidth
              isDisabled={anyLoginPending}
              className="mt-1"
            >
              {loginMutation.isPending ? m.auth_signing_in() : m.auth_sign_in()}
            </Button>
          </form>
        )}

        {alternativeError && (
          <p className="text-sm text-danger" role="alert">
            {alternativeError}
          </p>
        )}
        {hasAlternativeLogins && (
          <div className="relative my-1 flex items-center justify-center">
            <span className="absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-separator" />
            <span className="relative bg-surface px-2 text-[12px] text-muted">{m.auth_or()}</span>
          </div>
        )}
        {passkeysEnabled && (
          <Button
            type="button"
            variant="secondary"
            fullWidth
            isDisabled={anyLoginPending}
            onPress={() => {
              resetLoginErrors();
              passkeyMutation.mutate();
            }}
          >
            {passkeyMutation.isPending ? m.auth_checking_passkeys() : m.auth_sign_in_passkey()}
          </Button>
        )}
        {emailCodeEnabled && (
          <Button
            type="button"
            variant="secondary"
            fullWidth
            isDisabled={anyLoginPending}
            onPress={() => switchMethod(method === "password" ? "email-code" : "password")}
          >
            {method === "password" ? m.auth_sign_in_email_code() : m.auth_sign_in_with_password()}
          </Button>
        )}
        {oauthProviders.map((provider) => (
          <Button
            key={provider.name}
            type="button"
            variant="secondary"
            fullWidth
            isDisabled={anyLoginPending}
            onPress={() => {
              resetLoginErrors();
              oauthMutation.mutate(provider.name);
            }}
          >
            <OAuthProviderIcon provider={provider.name} />
            {oauthMutation.isPending && oauthMutation.variables === provider.name
              ? m.auth_redirecting()
              : m.auth_sign_in_oauth({ provider: provider.displayName })}
          </Button>
        ))}
        {canRegister && (
          <Link to="/register" className="text-center text-[13px] text-muted hover:text-foreground">
            {m.auth_create_account_link()}
          </Link>
        )}
      </Card.Content>
    </AuthShell>
  );
}
