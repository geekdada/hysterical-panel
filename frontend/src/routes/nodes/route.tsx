import { Outlet, createFileRoute } from "@tanstack/react-router";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/nodes")({
  staticData: breadcrumbStaticData({
    label: () => m.nav_nodes(),
    href: "/",
  }),
  component: NodesLayout,
});

function NodesLayout() {
  return <Outlet />;
}
