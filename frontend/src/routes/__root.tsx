/// <reference types="vite/client" />

import type { ReactNode } from "react";
import {
  Outlet,
  createRootRouteWithContext,
  HeadContent,
  Scripts,
} from "@tanstack/react-router";
import { readAuthCookie, type Auth } from "~/api/auth";

import "~/styles/globals.css";

export interface RouterContext {
  auth: Auth | null;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: () => ({ auth: readAuthCookie() }),
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: "Hysterical Panel" },
    ],
  }),
  component: RootComponent,
});

// Runs synchronously in <head> before the body paints: mirrors the OS
// color-scheme onto <html> so the themed first paint never flashes, and keeps
// it in sync when the user flips appearance mid-session (like Linear).
const themeScript = `
(function () {
  try {
    var mq = window.matchMedia("(prefers-color-scheme: dark)");
    function apply() {
      var dark = mq.matches;
      var el = document.documentElement;
      el.classList.remove(dark ? "light" : "dark");
      el.classList.add(dark ? "dark" : "light");
      el.setAttribute("data-theme", dark ? "dark" : "light");
      el.style.colorScheme = dark ? "dark" : "light";
    }
    apply();
    mq.addEventListener("change", apply);
  } catch (e) {}
})();
`;

function RootComponent() {
  return (
    <RootDocument>
      <Outlet />
    </RootDocument>
  );
}

function RootDocument({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <HeadContent />
      </head>
      <body className="bg-(--background) text-(--foreground) antialiased">
        {children}
        <AppVersion />
        <Scripts />
      </body>
    </html>
  );
}

function AppVersion() {
  return (
    <small
      aria-label={`Version ${__APP_VERSION__}`}
      className="pointer-events-none fixed right-3 bottom-2 z-50 select-none text-[10px] font-medium leading-none text-(--muted) opacity-70"
    >
      v{__APP_VERSION__}
    </small>
  );
}
