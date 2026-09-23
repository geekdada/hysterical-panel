import { Outlet, createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/emails")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.settings_email_manage(),
    href: "/settings/emails",
  }),
  component: EmailsLayout,
});

function EmailsLayout() {
  return <Outlet />;
}
