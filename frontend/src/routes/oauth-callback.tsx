import { useEffect, useRef, useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Card } from "@heroui/react";
import { completeOAuthLogin } from "~/api/auth";
import { AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
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

function initialError({ code, state, error }: OAuthCallbackSearch): string {
  if (error) return m.auth_oauth_provider_denied();
  if (!code || !state) return m.auth_oauth_state_mismatch();
  return "";
}

function OAuthCallbackPage() {
  const search = Route.useSearch();
  const [error, setError] = useState(() => initialError(search));
  const ran = useRef(false);

  useEffect(() => {
    if (error || !search.code || !search.state || ran.current) return;
    // The authorization code is single-use, so StrictMode's second run must not resend it.
    ran.current = true;
    completeOAuthLogin(search.code, search.state)
      .then(() => {
        window.location.replace("/");
      })
      .catch((err: unknown) => {
        setError(
          err instanceof Error ? localizeApiError(err.message) : m.auth_oauth_login_failed()
        );
      });
  }, [error, search.code, search.state]);

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-accent text-sm font-bold text-accent-foreground">
          H
        </span>
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.app_title()}
        </Card.Title>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        {error ? (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-danger" role="alert">
              {error}
            </p>
            <Link to="/login" className="text-[13px] font-medium text-accent hover:opacity-80">
              {m.auth_back_to_sign_in()}
            </Link>
          </div>
        ) : (
          <p className="py-2 text-[13px] text-muted">{m.auth_oauth_completing()}</p>
        )}
      </Card.Content>
    </AuthShell>
  );
}
