import { Link, createFileRoute, redirect } from "@tanstack/react-router";
import { Card } from "@heroui/react";
import { AuthShell } from "~/components/ui";

export const Route = createFileRoute("/forgot-password")({
  beforeLoad: ({ context }) => {
    if (context.auth) {
      throw redirect({ to: "/" });
    }
  },
  component: ForgotPasswordPage,
});

function ForgotPasswordPage() {
  return (
    <AuthShell>
      <Card.Header className="flex-col items-start gap-1 px-6 pt-6 pb-0">
        <span className="mb-2 grid size-7 place-items-center rounded-[7px] bg-(--accent) text-sm font-bold text-(--accent-foreground)">
          H
        </span>
        <Card.Title className="text-[15px] font-semibold tracking-tight text-(--foreground)">
          Reset your password
        </Card.Title>
        <Card.Description className="text-[13px] text-(--muted)">Hysterical Panel</Card.Description>
      </Card.Header>
      <Card.Content className="px-6 pt-4 pb-6">
        <div className="flex flex-col gap-3 py-2">
          <p className="text-[13px] text-(--muted)">
            Password reset isn't available yet. Ask an administrator to reset it for now.
          </p>
          <Link to="/login" className="text-[13px] font-medium text-(--accent) hover:opacity-80">
            Back to sign in
          </Link>
        </div>
      </Card.Content>
    </AuthShell>
  );
}
