import type { UserSubscription } from "~/api/queries";

// Client-side mirror of subscriptions.Reschedule's rules, so the modal can
// explain a rejected start before the server does.

export const GRANT_MS = 360 * 24 * 60 * 60 * 1000;

export type RescheduleProblem = "future" | "ended" | "overlap";

/**
 * When the latest earlier grant that expired on its own ended, or null if none.
 * Terminated grants may be overlapped, since admins end them to replace a
 * mistaken grant.
 */
export function previousGrantEnd(
  grants: UserSubscription[],
  current: UserSubscription
): number | null {
  const currentStart = Date.parse(current.starts_at);
  let latest: number | null = null;
  for (const grant of grants) {
    if (grant.id === current.id || grant.terminated_at) continue;
    if (Date.parse(grant.starts_at) >= currentStart) continue;
    const end = Date.parse(grant.ends_at);
    if (latest === null || end > latest) latest = end;
  }
  return latest;
}

export function rescheduleProblem(
  startMs: number,
  nowMs: number,
  previousEndMs: number | null
): RescheduleProblem | null {
  if (startMs > nowMs) return "future";
  if (startMs + GRANT_MS <= nowMs) return "ended";
  if (previousEndMs !== null && startMs < previousEndMs) return "overlap";
  return null;
}
