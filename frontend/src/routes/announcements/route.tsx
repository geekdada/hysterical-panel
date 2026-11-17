import { Outlet, createFileRoute } from "@tanstack/react-router";
import { requireAuth } from "~/api/guards";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/announcements")({
  beforeLoad: ({ context }) => requireAuth(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.announcements_title(),
    href: "/announcements",
  }),
  component: AnnouncementsLayout,
});

function AnnouncementsLayout() {
  return <Outlet />;
}
