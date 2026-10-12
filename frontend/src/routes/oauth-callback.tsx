import { useEffect, useRef, useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Card } from "@heroui/react";
import { completeOAuth, takePendingOAuth, type OAuthIntent } from "~/api/auth";
import { AuthBrandMark, AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import { safeRedirectPath } from "~/lib/safe-redirect";
import { useAppName } from "~/lib/use-app-name";
import * as m from "~/paraglide/messages.js";

interface OAuthCallbackSearch {
  code?: string;
  state?: string;
  error?: string;
}

export const Route = createFileRoute("/oauth-callback")({
  validateSearch: (search: Record<string, unknown>): OAuthCallbackSearch => ({
    code: typeof search.code === "string" ? search.code : undefined,
    state: typeof search.state === "string" ? search.state : undefined,
    error: typeof search.error === "string" ? search.error : undefined,
  }),
  component: OAuthCallbackPage,
});

const COPY: Record<
  OAuthIntent,
  {
    completing: () => string;
    failed: () => string;
    denied: () => string;
    invalid: () => string;
    back: () => string;
  }
> = {
  login: {
    completing: m.auth_oauth_completing,
    failed: m.auth_oauth_login_failed,
    denied: m.auth_oauth_provider_denied,
    invalid: m.auth_oauth_state_mismatch,
    back: m.auth_back_to_sign_in,
  },
  link: {
    completing: m.settings_social_logins_link_completing,
    failed: m.settings_social_logins_link_failed,
    denied: m.settings_social_logins_link_denied,
    invalid: m.settings_social_logins_link_invalid,
    back: m.settings_social_logins_back_to_settings,
  },
};

const DONE_PATH: Record<OAuthIntent, string> = { login: "/", link: "/settings" };

function OAuthCallbackPage() {
  const appName = useAppName();
  const search = Route.useSearch();
  // The pending attempt lives in sessionStorage, so the intent is known only after hydration.
  const [intent, setIntent] = useState<OAuthIntent>("login");
  const [error, setError] = useState("");
  const ran = useRef(false);
  const copy = COPY[intent];

  useEffect(() => {
    // The authorization code is single-use, so StrictMode's second run must not resend it.
    if (ran.current) return;
    ran.current = true;
    const pending = takePendingOAuth();
    const pendingIntent = pending?.intent ?? "login";
    setIntent(pendingIntent);
    if (search.error) {
      setError(COPY[pendingIntent].denied());
      return;
    }
    if (!search.code || !search.state) {
      setError(COPY[pendingIntent].invalid());
      return;
    }
    completeOAuth(pending, search.code, search.state)
      .then(() => {
        const returnTo =
          pendingIntent === "login" ? safeRedirectPath(pending?.returnTo) : undefined;
        window.location.replace(returnTo ?? DONE_PATH[pendingIntent]);
      })
      .catch((err: unknown) => {
        setError(
          err instanceof Error ? localizeApiError(err.message) : COPY[pendingIntent].failed()
        );
      });
  }, [search.code, search.state, search.error]);

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {appName}
        </Card.Title>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        {error ? (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-danger" role="alert">
              {error}
            </p>
            <Link
              to={intent === "link" ? "/settings" : "/login"}
              className="text-[13px] font-medium text-accent hover:opacity-80"
            >
              {copy.back()}
            </Link>
          </div>
        ) : (
          <p className="py-2 text-[13px] text-muted">{copy.completing()}</p>
        )}
      </Card.Content>
    </AuthShell>
  );
}
