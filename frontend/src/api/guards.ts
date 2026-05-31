import { redirect } from "@tanstack/react-router";
import type { Auth } from "./auth";

/**
 * Route guard for admin-only pages. Use inside a route's `beforeLoad`:
 *
 *   beforeLoad: ({ context }) => requireAdmin(context.auth)
 *
 * Unauthenticated visitors go to login; signed-in non-admins are bounced to the
 * dashboard rather than shown a forbidden page they can't act on.
 */
export function requireAdmin(auth: Auth | null): void {
  if (!auth) {
    throw redirect({ to: "/login" });
  }
  if (auth.user.role !== "admin") {
    throw redirect({ to: "/" });
  }
}
