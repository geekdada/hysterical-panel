import { Outlet, createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/announcements")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.settings_announcements_manage(),
    href: "/settings/announcements",
  }),
  component: AnnouncementsLayout,
});

function AnnouncementsLayout() {
  return <Outlet />;
}
