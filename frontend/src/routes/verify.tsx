import { useEffect, useRef, useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Card } from "@heroui/react";
import { confirmVerification } from "~/api/auth";
import { AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/verify")({
  validateSearch: (search: Record<string, unknown>): { token?: string } => ({
    token: typeof search.token === "string" ? search.token : undefined,
  }),
  component: VerifyPage,
});

type Status = "verifying" | "success" | "error";

function VerifyPage() {
  const { token } = Route.useSearch();
  const [status, setStatus] = useState<Status>(token ? "verifying" : "error");
  const [message, setMessage] = useState(token ? "" : m.auth_missing_token());
  const ran = useRef(false);

  useEffect(() => {
    if (!token || ran.current) return;
    ran.current = true;
    confirmVerification(token)
      .then(() => setStatus("success"))
      .catch((err: unknown) => {
        setStatus("error");
        setMessage(
          err instanceof Error ? localizeApiError(err.message) : m.auth_verification_link_invalid()
        );
      });
  }, [token]);

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-(--accent) text-sm font-bold text-(--accent-foreground)">
          H
        </span>
        <Card.Title className="text-[15px] font-semibold tracking-tight text-(--foreground)">
          {m.auth_email_verification()}
        </Card.Title>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        {status === "verifying" && (
          <p className="py-2 text-[13px] text-(--muted)">{m.auth_verifying_email()}</p>
        )}
        {status === "success" && (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--foreground)">{m.auth_email_verified()}</p>
            <p className="text-[13px] text-(--muted)">{m.auth_email_verified_sign_in()}</p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_go_to_sign_in()}
            </Link>
          </div>
        )}
        {status === "error" && (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--danger)" role="alert">
              {message}
            </p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_back_to_sign_in()}
            </Link>
          </div>
        )}
      </Card.Content>
    </AuthShell>
  );
}
