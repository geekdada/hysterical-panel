import { Outlet, createFileRoute } from "@tanstack/react-router";
import { requireAuth } from "~/api/guards";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings")({
  beforeLoad: ({ context }) => requireAuth(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.settings_title(),
    href: "/settings",
  }),
  component: SettingsLayout,
});

function SettingsLayout() {
  return <Outlet />;
}
