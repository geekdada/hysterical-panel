import { Outlet, createFileRoute } from "@tanstack/react-router";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/users")({
  staticData: breadcrumbStaticData({
    label: () => m.nav_users(),
    href: "/users",
  }),
  component: UsersLayout,
});

function UsersLayout() {
  return <Outlet />;
}
