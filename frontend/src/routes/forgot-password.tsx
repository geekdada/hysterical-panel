import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Link, createFileRoute, redirect } from "@tanstack/react-router";
import { Button, Card, Input, Label, TextField } from "@heroui/react";
import { requestPasswordReset } from "~/api/auth";
import { AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/forgot-password")({
  beforeLoad: ({ context }) => {
    if (context.auth) {
      throw redirect({ to: "/" });
    }
  },
  component: ForgotPasswordPage,
});

function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const resetMutation = useMutation({
    mutationFn: (vars: { email: string }) => requestPasswordReset(vars.email),
  });

  const error =
    resetMutation.error instanceof Error
      ? localizeApiError(resetMutation.error.message)
      : resetMutation.error
        ? m.auth_could_not_send_reset()
        : "";

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    resetMutation.mutate({ email: email.trim() });
  }

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-(--accent) text-sm font-bold text-(--accent-foreground)">
          H
        </span>
        <Card.Title className="text-[15px] font-semibold tracking-tight text-(--foreground)">
          {m.auth_reset_password()}
        </Card.Title>
        <Card.Description className="text-[13px] text-(--muted)">{m.app_title()}</Card.Description>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        {resetMutation.isSuccess ? (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--foreground)">{m.auth_check_email()}</p>
            <p className="text-[13px] text-(--muted)">{m.auth_reset_email_sent_intro()}</p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_back_to_sign_in()}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <p className="text-[13px] text-(--muted)">{m.auth_reset_enter_email()}</p>
            <TextField>
              <Label>{m.auth_email()}</Label>
              <Input
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => {
                  resetMutation.reset();
                  setEmail(e.target.value);
                }}
              />
            </TextField>

            {error && (
              <p className="text-sm text-(--danger)" role="alert">
                {error}
              </p>
            )}

            <Button
              type="submit"
              variant="primary"
              fullWidth
              isDisabled={resetMutation.isPending}
              className="mt-1"
            >
              {resetMutation.isPending ? m.auth_sending() : m.auth_send_reset_link()}
            </Button>
            <Link
              to="/login"
              className="text-center text-[13px] text-(--muted) hover:text-(--foreground)"
            >
              {m.auth_back_to_sign_in()}
            </Link>
          </form>
        )}
      </Card.Content>
    </AuthShell>
  );
}
