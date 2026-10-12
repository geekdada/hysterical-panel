import { Link, createFileRoute } from "@tanstack/react-router";
import { Card } from "@heroui/react";
import { AuthBrandMark, AuthShell } from "~/components/ui";
import { defaultDashboardSearch } from "~/lib/dashboard-search";
import * as m from "~/paraglide/messages.js";

// The backend sends the browser here when it cannot trust the app or its
// redirect_uri, so the error must not go back to the app (RFC 6749 4.1.2.1).
const REASONS = new Map<string, () => string>([
  ["invalid_request", m.oidc_error_invalid_request],
  ["invalid_client", m.oidc_error_invalid_client],
  ["invalid_redirect_uri", m.oidc_error_invalid_redirect_uri],
  ["rate_limited", m.oidc_error_rate_limited],
]);

export const Route = createFileRoute("/oidc/error")({
  validateSearch: (search: Record<string, unknown>): { reason?: string } => ({
    reason: typeof search.reason === "string" ? search.reason : undefined,
  }),
  component: OIDCErrorPage,
});

function OIDCErrorPage() {
  const { reason } = Route.useSearch();
  const message = (reason && REASONS.get(reason)?.()) || m.oidc_error_unknown();

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <AuthBrandMark />
        <Card.Title className="text-[15px] font-semibold tracking-tight text-foreground">
          {m.oidc_error_title()}
        </Card.Title>
      </Card.Header>
      <Card.Content className="flex flex-col gap-3 px-6 pt-4 pb-6">
        <p className="text-[13px] text-muted" role="alert">
          {message}
        </p>
        <Link
          to="/"
          search={defaultDashboardSearch()}
          className="text-[13px] font-medium text-accent hover:opacity-80"
        >
          {m.oidc_error_home()}
        </Link>
      </Card.Content>
    </AuthShell>
  );
}
