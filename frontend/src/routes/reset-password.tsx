import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Card, Input, Label, TextField } from "@heroui/react";
import { confirmPasswordReset } from "~/api/auth";
import { AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/reset-password")({
  validateSearch: (search: Record<string, unknown>): { token?: string } => ({
    token: typeof search.token === "string" ? search.token : undefined,
  }),
  component: ResetPasswordPage,
});

function Header() {
  return (
    <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
      <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-(--accent) text-sm font-bold text-(--accent-foreground)">
        H
      </span>
      <Card.Title className="text-[15px] font-semibold tracking-tight text-(--foreground)">
        {m.auth_reset_password()}
      </Card.Title>
    </Card.Header>
  );
}

function ResetPasswordPage() {
  const { token } = Route.useSearch();
  const [password, setPassword] = useState("");
  const [passwordConfirm, setPasswordConfirm] = useState("");
  const [localError, setLocalError] = useState("");

  const resetMutation = useMutation({
    mutationFn: (vars: { token: string; password: string; passwordConfirm: string }) =>
      confirmPasswordReset(vars.token, vars.password, vars.passwordConfirm),
  });

  const error =
    localError ||
    (resetMutation.error instanceof Error
      ? localizeApiError(resetMutation.error.message)
      : resetMutation.error
        ? m.auth_password_reset_failed()
        : "");

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    if (password !== passwordConfirm) {
      setLocalError(m.auth_passwords_dont_match());
      return;
    }
    setLocalError("");
    resetMutation.mutate({ token, password, passwordConfirm });
  }

  if (!token) {
    return (
      <AuthShell>
        <Header />
        <Card.Content className="px-6 pt-4 pb-6">
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--danger)" role="alert">
              {m.auth_reset_link_invalid()}
            </p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_back_to_sign_in()}
            </Link>
          </div>
        </Card.Content>
      </AuthShell>
    );
  }

  return (
    <AuthShell>
      <Header />
      <Card.Content className="px-6 pt-4 pb-6">
        {resetMutation.isSuccess ? (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--foreground)">{m.auth_password_reset_success()}</p>
            <p className="text-[13px] text-(--muted)">{m.auth_sign_in_new_password()}</p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_go_to_sign_in()}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <TextField>
              <Label>{m.auth_new_password()}</Label>
              <Input
                type="password"
                autoComplete="new-password"
                required
                value={password}
                onChange={(e) => {
                  resetMutation.reset();
                  setLocalError("");
                  setPassword(e.target.value);
                }}
              />
            </TextField>
            <TextField>
              <Label>{m.auth_confirm_new_password()}</Label>
              <Input
                type="password"
                autoComplete="new-password"
                required
                value={passwordConfirm}
                onChange={(e) => {
                  resetMutation.reset();
                  setLocalError("");
                  setPasswordConfirm(e.target.value);
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
              {resetMutation.isPending ? m.auth_resetting() : m.auth_reset_password_btn()}
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
