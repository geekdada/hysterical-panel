import {
  type CalendarDate,
  getLocalTimeZone,
  parseZonedDateTime,
  today,
  type ZonedDateTime,
} from "@internationalized/date";
import { toPbDateTime, type Granularity } from "~/components/traffic";

export type TrafficPeriod = "today" | "t-1" | "7d";

export type TrafficFastPreset = 1 | 2 | 3 | 7;

/** Inclusive local calendar-day bounds (start and end are whole days). */
export type LocalDateRange = {
  start: CalendarDate;
  end: CalendarDate;
};

function startOfUtcDay(ms: number): Date {
  const d = new Date(ms);
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
}

function zonedMidnight(day: CalendarDate, tz: string): ZonedDateTime {
  return parseZonedDateTime(`${day.toString()}T00:00:00[${tz}]`);
}

function zonedEndOfDay(day: CalendarDate, tz: string): ZonedDateTime {
  return parseZonedDateTime(`${day.toString()}T23:59:59[${tz}]`);
}

/** UTC daily bucket bounds for the dashboard traffic toggle (inclusive on both ends). */
export function trafficPeriodUtcRange(period: TrafficPeriod): { from: string; to: string } {
  const todayUtc = startOfUtcDay(Date.now());
  switch (period) {
    case "today":
      return { from: toPbDateTime(todayUtc), to: toPbDateTime(todayUtc) };
    case "t-1": {
      const day = new Date(todayUtc);
      day.setUTCDate(day.getUTCDate() - 1);
      return { from: toPbDateTime(day), to: toPbDateTime(day) };
    }
    case "7d": {
      const from = new Date(todayUtc);
      from.setUTCDate(from.getUTCDate() - 6);
      return { from: toPbDateTime(from), to: toPbDateTime(todayUtc) };
    }
  }
}

/** Latest selectable end day (local timezone). */
export function localTrafficMaxDay(): CalendarDate {
  return today(getLocalTimeZone());
}

/** Keep start <= end and end <= today (local). */
export function clampLocalTrafficRange(range: LocalDateRange): LocalDateRange {
  const cap = localTrafficMaxDay();
  let { start, end } = range;
  if (end.compare(cap) > 0) end = cap;
  if (start.compare(end) > 0) start = end;
  return { start, end };
}

/** Default node traffic window: local today through today. */
export function defaultLocalTrafficRange(): LocalDateRange {
  const day = localTrafficMaxDay();
  return { start: day, end: day };
}

/** Fast preset: N local calendar days through today (inclusive). */
export function presetLocalTrafficRange(days: TrafficFastPreset): LocalDateRange {
  const end = localTrafficMaxDay();
  return { start: end.subtract({ days: days - 1 }), end };
}

export function presetForLocalRange(range: LocalDateRange): TrafficFastPreset | null {
  const end = localTrafficMaxDay();
  if (range.end.compare(end) !== 0) return null;

  const presets: TrafficFastPreset[] = [1, 2, 3, 7];
  for (const days of presets) {
    if (range.start.compare(end.subtract({ days: days - 1 })) === 0) {
      return days;
    }
  }
  return null;
}

/** Local calendar days → UTC API query bounds (start 00:00, end 23:59:59 local). */
export function localRangeToUtcQuery(range: LocalDateRange): { from: string; to: string } {
  const tz = getLocalTimeZone();
  const { start, end } = clampLocalTrafficRange(range);
  return {
    from: toPbDateTime(zonedMidnight(start, tz).toDate()),
    to: toPbDateTime(zonedEndOfDay(end, tz).toDate()),
  };
}

export function granularityForLocalRange(range: LocalDateRange): Granularity {
  const days = range.end.compare(range.start);
  return days <= 3 ? "hourly" : "daily";
}
