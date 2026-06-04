import { toPbDateTime } from "~/components/traffic";

export type TrafficPeriod = "today" | "t-1" | "7d";

function startOfUtcDay(ms: number): Date {
  const d = new Date(ms);
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
}

/** UTC daily bucket bounds for the dashboard traffic toggle (inclusive on both ends). */
export function trafficPeriodUtcRange(period: TrafficPeriod): { from: string; to: string } {
  const today = startOfUtcDay(Date.now());
  switch (period) {
    case "today":
      return { from: toPbDateTime(today), to: toPbDateTime(today) };
    case "t-1": {
      const day = new Date(today);
      day.setUTCDate(day.getUTCDate() - 1);
      return { from: toPbDateTime(day), to: toPbDateTime(day) };
    }
    case "7d": {
      // Recent 7 UTC calendar days inclusive (today and the prior 6).
      const from = new Date(today);
      from.setUTCDate(from.getUTCDate() - 6);
      return { from: toPbDateTime(from), to: toPbDateTime(today) };
    }
  }
}
