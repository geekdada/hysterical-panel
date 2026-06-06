import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { Button, Card, Input, Label, TextField } from "@heroui/react";
import { login } from "~/api/auth";

export const Route = createFileRoute("/login")({
  beforeLoad: ({ context }) => {
    if (context.auth) {
      throw redirect({ to: "/" });
    }
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

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    loginMutation.mutate({ email, password });
  }

  const error =
    loginMutation.error instanceof Error
      ? loginMutation.error.message
      : loginMutation.error
        ? "Login failed"
        : "";

  return (
    <div className="flex min-h-svh items-center justify-center bg-(--background) p-4 text-(--foreground)">
      <Card className="w-full max-w-sm border border-(--border) bg-(--surface)">
        <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
          <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-(--accent) text-sm font-bold text-(--accent-foreground)">
            H
          </span>
          <Card.Title className="text-[15px] font-semibold tracking-tight text-(--foreground)">
            Hysterical Panel
          </Card.Title>
          <Card.Description className="text-[13px] text-(--muted)">
            Sign in to continue
          </Card.Description>
        </Card.Header>
        <Card.Content className="px-6 pt-4 pb-6">
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <TextField>
              <Label>Email</Label>
              <Input
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => {
                  loginMutation.reset();
                  setEmail(e.target.value);
                }}
              />
            </TextField>
            <TextField>
              <Label>Password</Label>
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

            {error && (
              <p className="text-sm text-(--danger)" role="alert">
                {error}
              </p>
            )}

            <Button
              type="submit"
              variant="primary"
              isDisabled={loginMutation.isPending}
              className="mt-1"
            >
              {loginMutation.isPending ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </Card.Content>
      </Card>
    </div>
  );
}
