import { useEffect, useRef } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { Button, Card } from "@heroui/react";
import { clearAuth } from "~/api/auth";
import {
  PanelApiError,
  abortOIDCAuthorizationRequest,
  approveOIDCAuthorizationRequest,
  denyOIDCAuthorizationRequest,
  oidcAuthorizationRequestQueryOptions,
  type OIDCAuthorizationRequest,
} from "~/api/queries";
import { AuthBrandMark, AuthShell } from "~/components/ui";
import { scopeLabel } from "~/lib/oidc-scopes";
import { useAppName } from "~/lib/use-app-name";
import * as m from "~/paraglide/messages.js";

interface ConsentSearch {
  request: string;
  // Set by /api/oidc/authorize for prompt=none. It only chooses between an
  // immediate abort and the sign-in form. The backend picks the error.
  prompt?: "none";
}

export const Route = createFileRoute("/oidc/authorize")({
  validateSearch: (search: Record<string, unknown>): ConsentSearch => ({
    request: typeof search.request === "string" ? search.request : "",
    prompt: search.prompt === "none" ? "none" : undefined,
  }),
  // The page grants access to the account, so no other site may frame it,
  // and the request handle in its URL stays out of Referer headers.
  headers: () => ({
    "Content-Security-Policy": "frame-ancestors 'none'",
    "X-Frame-Options": "DENY",
    "Referrer-Policy": "no-referrer",
  }),
  beforeLoad: ({ context, search, location }) => {
    if (!context.auth && search.request && search.prompt !== "none") {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: ConsentPage,
});

function ConsentPage() {
  const { auth } = Route.useRouteContext();
  const { request, prompt } = Route.useSearch();
  if (!request) return <ConsentMessage message={m.oidc_consent_expired()} />;
  if (!auth) return <SilentAbort handle={request} />;
  return (
    <SignedInConsent handle={request} email={auth.user.email} promptNone={prompt === "none"} />
  );
}

function consentPath(handle: string): string {
  return `/oidc/authorize?request=${encodeURIComponent(handle)}`;
}

function errorStatus(error: unknown): number | undefined {
  return error instanceof PanelApiError ? error.status : undefined;
}

function decisionErrorMessage(error: unknown): string {
  switch (errorStatus(error)) {
    case 404:
      return m.oidc_consent_expired();
    case 403:
      return m.oidc_consent_not_allowed();
    case 401:
      return m.oidc_consent_session_expired();
  }
  return m.oidc_consent_failed();
}

// A prompt=none request from a browser without a panel session goes back to
// the app with login_required without showing anything.
function SilentAbort({ handle }: { handle: string }) {
  const abort = useMutation({
    mutationFn: () => abortOIDCAuthorizationRequest(handle),
    onSuccess: (decision) => window.location.replace(decision.redirect_to),
  });
  const { mutate } = abort;
  const ran = useRef(false);

  useEffect(() => {
    // The request is single-use, so StrictMode's second run must not resend it.
    if (ran.current) return;
    ran.current = true;
    mutate();
  }, [mutate]);

  return (
    <ConsentMessage
      message={abort.isError ? decisionErrorMessage(abort.error) : m.oidc_consent_redirecting()}
    />
  );
}

function SignedInConsent({
  handle,
  email,
  promptNone,
}: {
  handle: string;
  email: string;
  promptNone: boolean;
}) {
  const navigate = useNavigate();
  const requestQuery = useQuery(oidcAuthorizationRequestQueryOptions(handle));
  const decision = useMutation({
    mutationFn: (allow: boolean) =>
      allow ? approveOIDCAuthorizationRequest(handle) : denyOIDCAuthorizationRequest(handle),
    // Replacing the history entry keeps Back from restoring a used consent
    // page out of the back-forward cache.
    onSuccess: (result) => window.location.replace(result.redirect_to),
  });
  const view = requestQuery.data;
  // Remembered consent and prompt=none skip the page. The backend turns a
  // prompt=none request it cannot answer into an error for the app.
  const automatic =
    view !== undefined &&
    (view.prompt_none || (!view.consent_required && !view.reauthentication_required));
  const { mutate } = decision;
  const started = useRef(false);

  useEffect(() => {
    if (!automatic || started.current) return;
    started.current = true;
    mutate(true);
  }, [automatic, mutate]);

  function signInAgain() {
    void navigate({ to: "/login", search: { reauth: true, redirect: consentPath(handle) } });
  }

  function switchAccount() {
    clearAuth();
    void navigate({ to: "/login", search: { redirect: consentPath(handle) } });
  }

  const failure = requestQuery.error ?? decision.error;
  const sessionRejected = errorStatus(failure) === 401 || errorStatus(failure) === 403;
  if (failure && promptNone && sessionRejected) {
    // A panel cookie the backend no longer accepts must still end a
    // prompt=none request at once instead of asking the User to sign in.
    return <SilentAbort handle={handle} />;
  }
  if (failure) {
    if (errorStatus(failure) === 400 && view) {
      return <ReauthCard clientName={view.client_name} onSignIn={signInAgain} />;
    }
    const signIn =
      errorStatus(failure) === 401
        ? { label: m.oidc_consent_reauth_action(), onPress: signInAgain }
        : undefined;
    return <ConsentMessage message={decisionErrorMessage(failure)} action={signIn} />;
  }
  if (!view) return <ConsentMessage message={m.oidc_consent_loading()} />;
  if (automatic || decision.isSuccess) {
    return <ConsentMessage message={m.oidc_consent_redirecting()} />;
  }
  if (view.reauthentication_required) {
    return <ReauthCard clientName={view.client_name} onSignIn={signInAgain} />;
  }
  return (
    <ConsentCard
      view={view}
      email={email}
      pending={decision.isPending}
      onDecide={(allow) => decision.mutate(allow)}
      onSwitchAccount={switchAccount}
    />
  );
}

function ConsentCard({
  view,
  email,
  pending,
  onDecide,
  onSwitchAccount,
}: {
  view: OIDCAuthorizationRequest;
  email: string;
  pending: boolean;
  onDecide: (allow: boolean) => void;
  onSwitchAccount: () => void;
}) {
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.oidc_consent_title({ client: view.client_name })}
        </Card.Title>
        <Card.Description className="text-[13px] text-muted">
          {m.oidc_consent_redirect({ origin: view.redirect_origin })}
        </Card.Description>
      </Card.Header>
      <Card.Content className="flex flex-col gap-4 px-6 pt-4 pb-6">
        <div>
          <p className="text-[13px] font-medium text-foreground">
            {m.oidc_consent_desc({ client: view.client_name })}
          </p>
          <ul className="mt-2 flex list-disc flex-col gap-1 pl-5">
            {view.scopes.map((scope) => (
              <li key={scope} className="text-[13px] text-muted">
                {scopeLabel(scope)}
              </li>
            ))}
          </ul>
        </div>
        <div className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
          <span className="min-w-0 truncate text-[13px] text-muted">
            {m.oidc_consent_account({ email })}
          </span>
          <Button size="sm" variant="ghost" isDisabled={pending} onPress={onSwitchAccount}>
            {m.oidc_consent_switch_account()}
          </Button>
        </div>
        <div className="flex gap-2">
          <Button
            variant="secondary"
            fullWidth
            isDisabled={pending}
            onPress={() => onDecide(false)}
          >
            {m.oidc_consent_deny()}
          </Button>
          <Button variant="primary" fullWidth isDisabled={pending} onPress={() => onDecide(true)}>
            {m.oidc_consent_allow()}
          </Button>
        </div>
      </Card.Content>
    </AuthShell>
  );
}

function ReauthCard({ clientName, onSignIn }: { clientName: string; onSignIn: () => void }) {
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.oidc_consent_reauth_title()}
        </Card.Title>
        <Card.Description className="text-[13px] text-muted">
          {m.oidc_consent_reauth_desc({ client: clientName })}
        </Card.Description>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        <Button variant="primary" fullWidth onPress={onSignIn}>
          {m.oidc_consent_reauth_action()}
        </Button>
      </Card.Content>
    </AuthShell>
  );
}

function ConsentMessage({
  message,
  action,
}: {
  message: string;
  action?: { label: string; onPress: () => void };
}) {
  const appName = useAppName();
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {appName}
        </Card.Title>
      </Card.Header>
      <Card.Content className="flex flex-col gap-3 px-6 pt-4 pb-6">
        <p className="text-[13px] text-muted" role="status">
          {message}
        </p>
        {action && (
          <Button variant="secondary" fullWidth onPress={action.onPress}>
            {action.label}
          </Button>
        )}
      </Card.Content>
    </AuthShell>
  );
}
