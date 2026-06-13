import {
  type CalendarDate,
  fromDate,
  getLocalTimeZone,
  parseZonedDateTime,
  today,
  toCalendarDate,
  type ZonedDateTime,
} from "@internationalized/date";
import { toPbDateTime, type Granularity } from "~/components/traffic";

export type TrafficRangeShortcut =
  | "today"
  | "yesterday"
  | "last-24h"
  | "last-7d"
  | "last-14d"
  | "last-30d"
  | "this-month"
  | "last-month";

/** Inclusive local calendar-day bounds (start and end are whole days). */
export type LocalDateRange = {
  start: CalendarDate;
  end: CalendarDate;
  shortcut?: TrafficRangeShortcut;
};

function zonedMidnight(day: CalendarDate, tz: string): ZonedDateTime {
  return parseZonedDateTime(`${day.toString()}T00:00:00[${tz}]`);
}

function zonedEndOfDay(day: CalendarDate, tz: string): ZonedDateTime {
  return parseZonedDateTime(`${day.toString()}T23:59:59[${tz}]`);
}

function localDateFromDate(date: Date, tz: string): CalendarDate {
  return toCalendarDate(fromDate(date, tz));
}

function rollingWindow(hours: number): { from: Date; to: Date } {
  const to = new Date();
  to.setSeconds(0, 0);
  return { from: new Date(to.getTime() - hours * 60 * 60 * 1000), to };
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
  return trafficShortcutRange("today");
}

export function trafficShortcutRange(shortcut: TrafficRangeShortcut): LocalDateRange {
  const tz = getLocalTimeZone();
  const end = localTrafficMaxDay();
  switch (shortcut) {
    case "today":
      return { start: end, end, shortcut };
    case "yesterday": {
      const day = end.subtract({ days: 1 });
      return { start: day, end: day, shortcut };
    }
    case "last-24h": {
      const { from, to } = rollingWindow(24);
      return {
        start: localDateFromDate(from, tz),
        end: localDateFromDate(to, tz),
        shortcut,
      };
    }
    case "last-7d": {
      const { from, to } = rollingWindow(24 * 7);
      return {
        start: localDateFromDate(from, tz),
        end: localDateFromDate(to, tz),
        shortcut,
      };
    }
    case "last-14d": {
      const { from, to } = rollingWindow(24 * 14);
      return {
        start: localDateFromDate(from, tz),
        end: localDateFromDate(to, tz),
        shortcut,
      };
    }
    case "last-30d": {
      const { from, to } = rollingWindow(24 * 30);
      return {
        start: localDateFromDate(from, tz),
        end: localDateFromDate(to, tz),
        shortcut,
      };
    }
    case "this-month":
      return { start: end.set({ day: 1 }), end, shortcut };
    case "last-month": {
      const thisMonthStart = end.set({ day: 1 });
      const start = thisMonthStart.subtract({ months: 1 });
      return {
        start,
        end: thisMonthStart.subtract({ days: 1 }),
        shortcut,
      };
    }
  }
}

export function shortcutForLocalRange(range: LocalDateRange): TrafficRangeShortcut | null {
  if (range.shortcut) return range.shortcut;

  const end = localTrafficMaxDay();
  const yesterday = end.subtract({ days: 1 });
  const thisMonthStart = end.set({ day: 1 });
  const lastMonthStart = thisMonthStart.subtract({ months: 1 });
  const lastMonthEnd = thisMonthStart.subtract({ days: 1 });

  if (range.start.compare(end) === 0 && range.end.compare(end) === 0) {
    return "today";
  }
  if (range.start.compare(yesterday) === 0 && range.end.compare(yesterday) === 0) {
    return "yesterday";
  }
  if (range.start.compare(thisMonthStart) === 0 && range.end.compare(end) === 0) {
    return "this-month";
  }
  if (range.start.compare(lastMonthStart) === 0 && range.end.compare(lastMonthEnd) === 0) {
    return "last-month";
  }
  return null;
}

/** Local calendar days → UTC API query bounds (start 00:00, end 23:59:59 local). */
export function localRangeToUtcQuery(range: LocalDateRange): { from: string; to: string } {
  const tz = getLocalTimeZone();
  switch (range.shortcut) {
    case "today":
    case "yesterday":
    case "this-month":
    case "last-month": {
      const { start, end } = trafficShortcutRange(range.shortcut);
      return {
        from: toPbDateTime(zonedMidnight(start, tz).toDate()),
        to: toPbDateTime(zonedEndOfDay(end, tz).toDate()),
      };
    }
    case "last-24h": {
      const { from, to } = rollingWindow(24);
      return { from: toPbDateTime(from), to: toPbDateTime(to) };
    }
    case "last-7d": {
      const { from, to } = rollingWindow(24 * 7);
      return { from: toPbDateTime(from), to: toPbDateTime(to) };
    }
    case "last-14d": {
      const { from, to } = rollingWindow(24 * 14);
      return { from: toPbDateTime(from), to: toPbDateTime(to) };
    }
    case "last-30d": {
      const { from, to } = rollingWindow(24 * 30);
      return { from: toPbDateTime(from), to: toPbDateTime(to) };
    }
  }

  const { start, end } = clampLocalTrafficRange(range);
  return {
    from: toPbDateTime(zonedMidnight(start, tz).toDate()),
    to: toPbDateTime(zonedEndOfDay(end, tz).toDate()),
  };
}

export function granularityForLocalRange(range: LocalDateRange): Granularity {
  if (range.shortcut === "last-24h") return "hourly";
  if (
    range.shortcut === "last-7d" ||
    range.shortcut === "last-14d" ||
    range.shortcut === "last-30d"
  ) {
    return "daily";
  }

  const days = range.end.compare(range.start);
  return days <= 3 ? "hourly" : "daily";
}
