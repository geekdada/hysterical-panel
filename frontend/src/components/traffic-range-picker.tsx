"use client";

import { Chip, DateField, DateRangePicker, RangeCalendar, useLocale } from "@heroui/react";
import { DateFormatter } from "@internationalized/date";
import { useState } from "react";
import {
  clampLocalTrafficRange,
  type LocalDateRange,
  localTrafficMaxDay,
  shortcutForLocalRange,
  trafficShortcutRange,
  type TrafficRangeShortcut,
} from "~/lib/traffic-range";
import { useMounted } from "~/lib/use-mounted";
import { useActiveTimeZone } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

const SHORTCUT_LABELS: Record<TrafficRangeShortcut, () => string> = {
  today: () => m.traffic_range_shortcut_today(),
  yesterday: () => m.traffic_range_shortcut_yesterday(),
  "last-24h": () => m.traffic_range_shortcut_last_24h(),
  "last-7d": () => m.traffic_range_shortcut_last_7d(),
  "last-14d": () => m.traffic_range_shortcut_last_14d(),
  "last-30d": () => m.traffic_range_shortcut_last_30d(),
  "this-month": () => m.traffic_range_shortcut_this_month(),
  "last-month": () => m.traffic_range_shortcut_last_month(),
};

const SHORTCUTS: { key: TrafficRangeShortcut; label: () => string }[] = [
  { key: "today", label: SHORTCUT_LABELS.today },
  { key: "yesterday", label: SHORTCUT_LABELS.yesterday },
  { key: "last-24h", label: SHORTCUT_LABELS["last-24h"] },
  { key: "last-7d", label: SHORTCUT_LABELS["last-7d"] },
  { key: "last-14d", label: SHORTCUT_LABELS["last-14d"] },
  { key: "last-30d", label: SHORTCUT_LABELS["last-30d"] },
  { key: "this-month", label: SHORTCUT_LABELS["this-month"] },
  { key: "last-month", label: SHORTCUT_LABELS["last-month"] },
];

const CALENDAR_CELL =
  "size-8 min-w-8 p-0 text-center text-[12px] tabular-nums sm:size-9 sm:min-w-9 sm:text-[13px]";

const TRIGGER =
  "inline-flex h-8 w-full min-w-0 items-center gap-1.5 border border-(--border) bg-(--surface) px-2.5 font-mono text-[11px] font-medium tabular-nums text-(--foreground) transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) sm:w-auto sm:max-w-[min(100%,16rem)]";

function rangeLabelFormatter(locale: string, tz: string) {
  return new DateFormatter(locale, {
    month: "short",
    day: "numeric",
    year: "numeric",
    timeZone: tz,
  });
}

function formatTriggerRange(range: LocalDateRange, locale: string, tz: string): string {
  const formatter = rangeLabelFormatter(locale, tz);
  const start = range.start.toDate(tz);
  if (range.start.compare(range.end) === 0) {
    return formatter.format(start);
  }
  return formatter.formatRange(start, range.end.toDate(tz));
}

export function TrafficRangePicker({
  value,
  onChange,
}: {
  value: LocalDateRange;
  onChange: (range: LocalDateRange) => void;
}) {
  const mounted = useMounted();
  const { locale } = useLocale();
  const tz = useActiveTimeZone();
  const [open, setOpen] = useState(false);
  const maxDay = localTrafficMaxDay(tz);
  const activeShortcut = shortcutForLocalRange(value, tz);

  function selectShortcut(key: TrafficRangeShortcut) {
    onChange(trafficShortcutRange(key, tz));
    setOpen(false);
  }
  const pickerValue = value.shortcut ? trafficShortcutRange(value.shortcut, tz) : value;
  const triggerLabel = formatTriggerRange(pickerValue, locale, tz);

  return (
    <div className="flex w-full min-w-0 justify-end">
      {!mounted ? (
        <div
          className="h-8 w-full shrink-0 border border-(--border) bg-(--surface-secondary) animate-pulse sm:w-40"
          aria-hidden
        />
      ) : (
        <DateRangePicker
          aria-label={m.traffic_range_aria()}
          className="traffic-range-picker relative w-full min-w-0 sm:w-auto"
          endName="trafficEnd"
          granularity="day"
          isOpen={open}
          maxValue={maxDay}
          onOpenChange={setOpen}
          startName="trafficStart"
          value={pickerValue}
          onChange={(next) => {
            if (next) {
              const range = next as LocalDateRange;
              onChange(clampLocalTrafficRange({ start: range.start, end: range.end }, tz));
            }
          }}
        >
          <DateField.Group className={TRIGGER}>
            <DateField.Suffix className="flex w-full min-w-0 p-0">
              <DateRangePicker.Trigger
                aria-label={m.traffic_range_aria_with_label({ label: triggerLabel })}
                className="inline-flex w-full min-w-0 items-center gap-1.5 border-0 bg-transparent p-0 font-inherit text-inherit shadow-none outline-none hover:bg-transparent data-[focus-visible=true]:outline-none"
              >
                <span className="min-w-0 flex-1 truncate text-left text-[11px]">
                  {triggerLabel}
                </span>
                <DateRangePicker.TriggerIndicator className="size-3.5 shrink-0 text-(--muted)" />
              </DateRangePicker.Trigger>
            </DateField.Suffix>
          </DateField.Group>

          <DateRangePicker.Popover
            className="traffic-range-popover z-50 flex w-[min(100vw-2rem,18rem)] max-w-full flex-col overflow-visible border border-(--border) bg-(--surface) p-2.5 shadow-sm outline-none focus:outline-none focus-visible:outline-none"
            offset={8}
            placement="bottom end"
            shouldFlip
          >
            <div className="flex flex-wrap gap-1.5 px-2">
              {SHORTCUTS.map(({ key, label }) => {
                const active = activeShortcut === key;
                return (
                  <button
                    key={key}
                    type="button"
                    aria-pressed={active}
                    onClick={() => selectShortcut(key)}
                    className="group inline-flex rounded-2xl outline-none focus-visible:ring-2 focus-visible:ring-(--focus) cursor-pointer"
                  >
                    <Chip
                      color={active ? "accent" : "default"}
                      variant={active ? "primary" : "secondary"}
                      className={
                        active
                          ? undefined
                          : "bg-(--surface-secondary) text-(--muted) transition-colors group-hover:bg-(--surface-tertiary) group-hover:text-(--foreground)"
                      }
                    >
                      {label()}
                    </Chip>
                  </button>
                );
              })}
            </div>
            <div className="mt-4 border-t border-(--separator) pt-2">
              <RangeCalendar aria-label={m.traffic_range_aria()} className="mx-auto w-full">
                <RangeCalendar.Header className="flex items-center">
                  <RangeCalendar.YearPickerTrigger className="inline-flex h-7 min-w-0 flex-1 items-center gap-1 px-2 text-left text-[12px] font-semibold text-(--foreground) transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) sm:text-[13px]">
                    <RangeCalendar.YearPickerTriggerHeading />
                    <RangeCalendar.YearPickerTriggerIndicator className="text-(--muted)" />
                  </RangeCalendar.YearPickerTrigger>
                  <RangeCalendar.NavButton
                    slot="previous"
                    className="size-7 shrink-0 text-(--muted) transition-colors duration-150 hover:bg-(--surface-secondary) hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                  />
                  <RangeCalendar.NavButton
                    slot="next"
                    className="size-7 shrink-0 text-(--muted) transition-colors duration-150 hover:bg-(--surface-secondary) hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                  />
                </RangeCalendar.Header>
                <RangeCalendar.Grid className="w-full border-collapse">
                  <RangeCalendar.GridHeader>
                    {(day) => (
                      <RangeCalendar.HeaderCell
                        className={`${CALENDAR_CELL} text-[10px] font-medium uppercase tracking-wide text-(--muted) sm:text-[11px]`}
                      >
                        {day}
                      </RangeCalendar.HeaderCell>
                    )}
                  </RangeCalendar.GridHeader>
                  <RangeCalendar.GridBody>
                    {(date) => (
                      <RangeCalendar.Cell
                        date={date}
                        className={`${CALENDAR_CELL} text-(--foreground) outline-none transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:ring-2 focus-visible:ring-(--focus)`}
                      />
                    )}
                  </RangeCalendar.GridBody>
                </RangeCalendar.Grid>
                <RangeCalendar.YearPickerGrid>
                  <RangeCalendar.YearPickerGridBody>
                    {({ year }) => (
                      <RangeCalendar.YearPickerCell
                        year={year}
                        className="text-(--foreground) transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                      />
                    )}
                  </RangeCalendar.YearPickerGridBody>
                </RangeCalendar.YearPickerGrid>
              </RangeCalendar>
            </div>
          </DateRangePicker.Popover>
        </DateRangePicker>
      )}
    </div>
  );
}
