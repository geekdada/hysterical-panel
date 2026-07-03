import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, redirect } from "@tanstack/react-router";
import { Button, Card, Input, Label, TextField } from "@heroui/react";
import { register } from "~/api/auth";
import { canQueryPanelApi, fetchPanelConfigQuery, queryKeys } from "~/api/queries";
import { AuthShell } from "~/components/ui";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/register")({
  validateSearch: (search: Record<string, unknown>): { code?: string } => ({
    code: typeof search.code === "string" ? search.code : undefined,
  }),
  beforeLoad: ({ context }) => {
    if (context.auth) {
      throw redirect({ to: "/" });
    }
  },
  component: RegisterPage,
});

function RegisterPage() {
  const { code: codeParam } = Route.useSearch();
  const configQuery = useQuery({
    queryKey: queryKeys.config(),
    queryFn: fetchPanelConfigQuery,
    enabled: canQueryPanelApi(),
  });
  const config = configQuery.data;

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState(codeParam ?? "");

  useEffect(() => {
    if (codeParam) setCode(codeParam);
  }, [codeParam]);

  const registerMutation = useMutation({
    mutationFn: (vars: { email: string; password: string; code?: string }) =>
      register(vars.email, vars.password, vars.code),
    onSuccess: (result) => {
      if (!result.requires_verification) {
        window.location.href = "/";
      }
    },
  });

  const open = config?.registration_open ?? false;
  const requireInviteForOpen = config?.registration_require_invite ?? false;
  const canRegister = open;
  const codeRequired = open && requireInviteForOpen;
  const showCode = codeRequired || code.length > 0;

  const result = registerMutation.data;
  const error =
    registerMutation.error instanceof Error
      ? localizeApiError(registerMutation.error.message)
      : registerMutation.error
        ? m.auth_registration_failed()
        : "";

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    registerMutation.mutate({ email, password, code: code.trim() || undefined });
  }

  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-(--accent) text-sm font-bold text-(--accent-foreground)">
          H
        </span>
        <Card.Title className="text-[15px] font-semibold tracking-tight text-(--foreground)">
          {m.auth_create_your_account()}
        </Card.Title>
        <Card.Description className="text-[13px] text-(--muted)">{m.app_title()}</Card.Description>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        {configQuery.isPending ? (
          <p className="py-4 text-[13px] text-(--muted)">{m.common_loading()}</p>
        ) : !canRegister ? (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--muted)">{m.auth_registration_closed()}</p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_back_to_sign_in()}
            </Link>
          </div>
        ) : result?.requires_verification ? (
          <div className="flex flex-col gap-3 py-2">
            <p className="text-[13px] text-(--foreground)">{m.auth_check_email()}</p>
            <p className="text-[13px] text-(--muted)">
              {result.verification_sent === false
                ? m.auth_verification_failed_send()
                : m.auth_verification_sent()}
            </p>
            <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
              {m.auth_back_to_sign_in()}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <TextField>
              <Label>{m.auth_email()}</Label>
              <Input
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => {
                  registerMutation.reset();
                  setEmail(e.target.value);
                }}
              />
            </TextField>
            <TextField>
              <Label>{m.auth_password()}</Label>
              <Input
                type="password"
                autoComplete="new-password"
                required
                value={password}
                onChange={(e) => {
                  registerMutation.reset();
                  setPassword(e.target.value);
                }}
              />
            </TextField>
            {showCode && (
              <TextField>
                <Label>{codeRequired ? m.auth_invite_code() : m.auth_invite_code_optional()}</Label>
                <Input
                  type="text"
                  required={codeRequired}
                  value={code}
                  className="font-mono text-[13px]"
                  onChange={(e) => {
                    registerMutation.reset();
                    setCode(e.target.value);
                  }}
                />
              </TextField>
            )}

            {error && (
              <p className="text-sm text-(--danger)" role="alert">
                {error}
              </p>
            )}

            <Button
              type="submit"
              variant="primary"
              fullWidth
              isDisabled={registerMutation.isPending}
              className="mt-1"
            >
              {registerMutation.isPending ? m.auth_creating_account() : m.auth_create_account_btn()}
            </Button>
            <Link
              to="/login"
              className="text-center text-[13px] text-(--muted) hover:text-(--foreground)"
            >
              {m.auth_already_have_account()}
            </Link>
          </form>
        )}
      </Card.Content>
    </AuthShell>
  );
}
