import { redirect } from "@tanstack/react-router";
import type { Auth } from "./auth";

export function requireAuth(auth: Auth | null): asserts auth is Auth {
  if (!auth) {
    throw redirect({ to: "/login" });
  }
}

/**
 * Route guard for admin-only pages. Use inside a route's `beforeLoad`:
 *
 *   beforeLoad: ({ context }) => requireAdmin(context.auth)
 *
 * Unauthenticated visitors go to login; signed-in non-admins are bounced to the
 * app root, whose loader sends them to their own account page.
 */
export function requireAdmin(auth: Auth | null): void {
  requireAuth(auth);
  if (auth.user.role !== "admin") {
    throw redirect({ to: "/" });
  }
}

export function requireAdminOrSelf(auth: Auth | null, userId: string): void {
  requireAuth(auth);
  if (auth.user.role === "admin" || auth.user.id === userId) {
    return;
  }
  throw redirect({ to: "/users/$userId", params: { userId: auth.user.id } });
}
