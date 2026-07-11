/// <reference types="vite/client" />

import { useEffect, useState, type ReactNode } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { I18nProvider } from "@react-aria/i18n";
import {
  Link,
  Outlet,
  createRootRouteWithContext,
  HeadContent,
  Scripts,
  useRouter,
} from "@tanstack/react-router";
import { Button } from "@heroui/react";
import { getLocale } from "~/paraglide/runtime";
import * as m from "~/paraglide/messages.js";
import { localizeApiError } from "~/lib/api-error";
import { intlLocale } from "~/lib/locale";
import {
  consumeFreshPasswordLogin,
  isPasskeySoftError,
  listPasskeys,
  loginWithPasskey,
  readAuthCookie,
  registerPasskey,
  type Auth,
} from "~/api/auth";
import { PanelQueryDevtools } from "~/api/query-provider";
import {
  ensureSessionFresh,
  getSessionRecoveryFailed,
  subscribeSessionRecovery,
} from "~/api/session";
import { readSsrTimeZone, syncTimezoneCookie } from "~/lib/timezone";

import "~/styles/globals.css";

export interface RouterContext {
  auth: Auth | null;
  queryClient: QueryClient;
  timeZone: string | null;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: () => ({ auth: readAuthCookie(), timeZone: readSsrTimeZone() }),
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: m.app_title() },
    ],
    links: [
      { rel: "icon", href: "/favicon.ico", sizes: "48x48" },
      { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" },
      { rel: "apple-touch-icon", href: "/apple-touch-icon.png" },
    ],
  }),
  component: RootComponent,
  notFoundComponent: NotFoundComponent,
});

const SESSION_KEEPALIVE_MS = 4 * 60 * 60 * 1000;

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
  const locale = intlLocale();
  return (
    <I18nProvider locale={locale}>
      <RootDocument>
        <SessionKeeper />
        <TimezoneCookieSync />
        <SessionRecoveryBanner />
        <Outlet />
        <PanelQueryDevtools />
      </RootDocument>
    </I18nProvider>
  );
}

function NotFoundComponent() {
  const { auth } = Route.useRouteContext();
  return (
    <div className="grid min-h-svh place-items-center bg-background px-4 text-foreground">
      <div className="text-center">
        <p className="text-[13px] font-medium">{m.not_found_title()}</p>
        <p className="mx-auto mt-1 max-w-sm text-xs text-muted">{m.not_found_hint()}</p>
        <div className="mt-4 flex justify-center">
          <Link
            to={auth ? "/" : "/login"}
            className="inline-flex cursor-pointer place-items-center rounded-[5px] border bg-surface-secondary px-3 py-0.5 text-[11px] text-foreground no-underline transition-colors duration-150 hover:bg-surface-tertiary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
          >
            ← {auth ? m.not_found_back() : m.auth_back_to_sign_in()}
          </Link>
        </div>
      </div>
    </div>
  );
}

function RootDocument({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang={getLocale()} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <HeadContent />
      </head>
      <body className="bg-background text-foreground antialiased">
        {children}
        <PasskeyAutoEnrollment />
        <Scripts />
      </body>
    </html>
  );
}

function TimezoneCookieSync() {
  useEffect(() => {
    syncTimezoneCookie();
  }, []);
  return null;
}

function SessionKeeper() {
  const router = useRouter();
  const { auth } = Route.useRouteContext();

  useEffect(() => {
    if (!auth) return;

    let cancelled = false;

    async function refresh(force: boolean) {
      const refreshed = await ensureSessionFresh({ force });
      if (cancelled || !refreshed) return;
      await router.invalidate();
    }

    void refresh(true);

    const intervalId = window.setInterval(() => {
      void refresh(false);
    }, SESSION_KEEPALIVE_MS);

    function onVisibilityChange() {
      if (document.visibilityState === "visible") {
        void refresh(false);
      }
    }

    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [auth?.user.id, router]);

  return null;
}

function SessionRecoveryBanner() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { auth } = Route.useRouteContext();
  const [failed, setFailed] = useState(getSessionRecoveryFailed);
  const [recovering, setRecovering] = useState(false);
  const [recoveryError, setRecoveryError] = useState("");

  useEffect(() => subscribeSessionRecovery(() => setFailed(getSessionRecoveryFailed())), []);

  if (!auth || !failed) return null;

  async function reconnectWithPasskey() {
    setRecovering(true);
    setRecoveryError("");
    try {
      await loginWithPasskey(true);
      setFailed(false);
      await router.invalidate();
      await queryClient.invalidateQueries();
    } catch (error) {
      if (!isPasskeySoftError(error)) {
        setRecoveryError(
          error instanceof Error ? localizeApiError(error.message) : m.error_session_renew()
        );
      }
    } finally {
      setRecovering(false);
    }
  }

  return (
    <div
      className="border-b border-border bg-danger-soft px-4 py-2 text-[13px] text-danger-soft-foreground sm:px-6"
      role="alert"
    >
      <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-3">
        <span>{m.session_renewal_message()}</span>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            onPress={reconnectWithPasskey}
            isDisabled={recovering}
          >
            {recovering ? m.session_reconnecting() : m.session_use_passkey()}
          </Button>
          <Link
            to="/users/$userId"
            params={{ userId: auth.user.id }}
            className="text-[13px] font-medium text-accent underline-offset-2 hover:underline"
          >
            {m.session_account_settings()}
          </Link>
        </div>
        {recoveryError && <span className="text-danger-soft-foreground">{recoveryError}</span>}
      </div>
    </div>
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
        await registerPasskey(userId, m.user_passkeys_default_name(), true);
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
