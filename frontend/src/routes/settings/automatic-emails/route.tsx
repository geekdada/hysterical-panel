import { Outlet, createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/automatic-emails")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.automatic_emails_title(),
    href: "/settings/automatic-emails",
  }),
  component: AutomaticEmailsLayout,
});

function AutomaticEmailsLayout() {
  return <Outlet />;
}
