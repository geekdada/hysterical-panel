"use client";

import {
  DateField,
  DateRangePicker,
  RangeCalendar,
  useLocale,
} from "@heroui/react";
import { DateFormatter, getLocalTimeZone } from "@internationalized/date";
import {
  clampLocalTrafficRange,
  type LocalDateRange,
  localTrafficMaxDay,
  presetForLocalRange,
  presetLocalTrafficRange,
  type TrafficFastPreset,
} from "~/lib/traffic-range";
import { useMounted } from "~/lib/use-mounted";

const PRESETS: { days: TrafficFastPreset; label: string }[] = [
  { days: 1, label: "24h" },
  { days: 2, label: "2d" },
  { days: 3, label: "3d" },
  { days: 7, label: "7d" },
];

const CALENDAR_CELL =
  "size-8 min-w-8 p-0 text-center text-[12px] tabular-nums sm:size-9 sm:min-w-9 sm:text-[13px]";

const TRIGGER =
  "inline-flex h-8 w-full min-w-0 items-center gap-1.5 rounded-(--radius) border border-(--border) bg-(--surface) px-2.5 font-mono text-[11px] font-medium tabular-nums text-(--foreground) transition-colors duration-150 hover:bg-(--surface-secondary) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) sm:w-auto sm:max-w-[min(100%,16rem)]";

function rangeLabelFormatter(locale: string) {
  return new DateFormatter(locale, {
    month: "short",
    day: "numeric",
    year: "numeric",
    timeZone: getLocalTimeZone(),
  });
}

function formatTriggerRange(range: LocalDateRange, locale: string): string {
  const formatter = rangeLabelFormatter(locale);
  const tz = getLocalTimeZone();
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
  const maxDay = localTrafficMaxDay();
  const activePreset = presetForLocalRange(value);
  const triggerLabel = formatTriggerRange(value, locale);

  return (
    <div className="flex w-full min-w-0 flex-col items-stretch gap-1.5 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end sm:gap-2">
      <div className="inline-flex w-full rounded-(--radius) border border-(--border) p-0.5 sm:w-auto">
        {PRESETS.map(({ days, label }) => (
          <button
            key={days}
            type="button"
            onClick={() => onChange(presetLocalTrafficRange(days))}
            className={`min-w-0 flex-1 rounded-[calc(var(--radius)-2px)] px-2 py-0.5 font-mono text-[11px] font-medium tabular-nums transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) sm:flex-none sm:px-2.5 ${
              activePreset === days
                ? "bg-(--surface-secondary) text-(--foreground)"
                : "text-(--muted) hover:text-(--foreground)"
            }`}
            aria-pressed={activePreset === days}
          >
            {label}
          </button>
        ))}
      </div>

      {!mounted ? (
        <div
          className="h-8 w-full shrink-0 rounded-(--radius) border border-(--border) bg-(--surface-secondary) animate-pulse sm:w-40"
          aria-hidden
        />
      ) : (
        <DateRangePicker
          aria-label="Traffic period"
          className="traffic-range-picker relative w-full min-w-0 sm:w-auto"
          endName="trafficEnd"
          granularity="day"
          maxValue={maxDay}
          startName="trafficStart"
          value={value}
          onChange={(next) => {
            if (next) onChange(clampLocalTrafficRange(next as LocalDateRange));
          }}
        >
          <DateField.Group className={TRIGGER}>
            <DateField.Suffix className="flex w-full min-w-0 p-0">
              <DateRangePicker.Trigger
                aria-label={`Traffic period, ${triggerLabel}`}
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
            className="traffic-range-popover z-50 flex w-[min(100vw-2rem,18rem)] max-w-full flex-col overflow-visible rounded-(--radius) border border-(--border) bg-(--surface) p-3 shadow-sm"
            offset={8}
            placement="bottom end"
            shouldFlip
          >
            <RangeCalendar
              aria-label="Traffic period"
              className="w-full mx-auto"
            >
              <RangeCalendar.Header className="mb-2 flex items-center gap-1">
                <RangeCalendar.YearPickerTrigger className="min-w-0 flex-1 text-left text-[12px] font-semibold text-(--foreground) sm:text-[13px]">
                  <RangeCalendar.YearPickerTriggerHeading />
                  <RangeCalendar.YearPickerTriggerIndicator className="text-(--muted)" />
                </RangeCalendar.YearPickerTrigger>
                <RangeCalendar.NavButton
                  slot="previous"
                  className="size-7 shrink-0 rounded text-(--muted) hover:bg-(--surface-secondary) hover:text-(--foreground)"
                />
                <RangeCalendar.NavButton
                  slot="next"
                  className="size-7 shrink-0 rounded text-(--muted) hover:bg-(--surface-secondary) hover:text-(--foreground)"
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
                      className={`${CALENDAR_CELL} rounded text-(--foreground) outline-none hover:bg-(--surface-secondary) focus-visible:ring-2 focus-visible:ring-(--focus)`}
                    />
                  )}
                </RangeCalendar.GridBody>
              </RangeCalendar.Grid>
              <RangeCalendar.YearPickerGrid>
                <RangeCalendar.YearPickerGridBody>
                  {({ year }) => <RangeCalendar.YearPickerCell year={year} />}
                </RangeCalendar.YearPickerGridBody>
              </RangeCalendar.YearPickerGrid>
            </RangeCalendar>
          </DateRangePicker.Popover>
        </DateRangePicker>
      )}
    </div>
  );
}
