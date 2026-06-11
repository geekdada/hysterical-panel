/// <reference types="vite/client" />

import { useEffect, type ReactNode } from "react";
import {
  Outlet,
  createRootRouteWithContext,
  HeadContent,
  Scripts,
} from "@tanstack/react-router";
import {
  consumeFreshPasswordLogin,
  isPasskeySoftError,
  listPasskeys,
  readAuthCookie,
  registerPasskey,
  type Auth,
} from "~/api/auth";
import { PanelQueryProvider } from "~/api/query-provider";

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
    links: [
      { rel: "icon", href: "/favicon.ico", sizes: "48x48" },
      { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" },
      { rel: "apple-touch-icon", href: "/apple-touch-icon.png" },
    ],
  }),
  component: RootComponent,
});

// Runs synchronously in <head> before the body paints: applies the saved theme
// preference (or the OS color-scheme when "system") onto <html> so the themed
// first paint never flashes, and keeps it in sync when the user flips appearance
// mid-session (like Linear). Self-contained on purpose — it runs before the
// bundle loads, so it can't import src/lib/theme.ts; keep the key + values in sync.
const themeScript = `
(function () {
  try {
    var KEY = "hp:theme";
    var mq = window.matchMedia("(prefers-color-scheme: dark)");
    function resolve() {
      var pref;
      try { pref = localStorage.getItem(KEY); } catch (e) {}
      if (pref !== "light" && pref !== "dark") pref = "system";
      return pref === "system" ? (mq.matches ? "dark" : "light") : pref;
    }
    function apply() {
      var dark = resolve() === "dark";
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
      <PanelQueryProvider>
        <Outlet />
      </PanelQueryProvider>
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
        <PasskeyAutoEnrollment />
        <Scripts />
      </body>
    </html>
  );
}

function PasskeyAutoEnrollment() {
  const { auth } = Route.useRouteContext();

  useEffect(() => {
    if (!auth || !consumeFreshPasswordLogin(auth.user.id)) return;
    const userId = auth.user.id;
    let cancelled = false;

    async function run() {
      try {
        const passkeys = await listPasskeys(userId);
        if (cancelled || passkeys.length > 0) return;
        await registerPasskey(userId, "Passkey", true);
      } catch (error) {
        if (!isPasskeySoftError(error)) {
          // The explicit account-page button remains available.
        }
      }
    }

    void run();
    return () => {
      cancelled = true;
      void import("@simplewebauthn/browser").then(({ WebAuthnAbortService }) => {
        WebAuthnAbortService.cancelCeremony();
      });
    };
  }, [auth?.user.id]);

  return null;
}
