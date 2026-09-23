import { createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";

export const Route = createFileRoute("/settings/emails/$sendoutId")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: () => null,
});
