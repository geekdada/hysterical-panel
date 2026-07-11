import * as m from "~/paraglide/messages.js";
import { intlLocale } from "~/lib/locale";

// Shared formatting helpers. Bytes use binary units; relative time is the live
// "Ns ago" readout used across the panel. All durations are clamped at zero so
// clock skew never renders a negative age.

export function formatBytes(bytes: number, locale = intlLocale()): string {
  if (bytes === 0) return m.format_bytes_zero();
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  const formatted = new Intl.NumberFormat(locale, {
    maximumFractionDigits: i === 0 ? 0 : 1,
    minimumFractionDigits: i === 0 ? 0 : 1,
  }).format(val);
  return `${formatted} ${units[i]}`;
}

export function formatBytesPerSecond(bytesPerSecond: number, locale = intlLocale()): string {
  return `${formatBytes(Math.max(0, bytesPerSecond), locale)}/s`;
}

export function relTime(fromMs: number, nowMs: number | null): string {
  if (nowMs === null) return m.common_em_dash();
  const s = Math.max(0, Math.round((nowMs - fromMs) / 1000));
  if (s < 5) return m.format_just_now();
  if (s < 60) return m.format_seconds_ago({ s: String(s) });
  const mins = Math.floor(s / 60);
  if (mins < 60) return m.format_minutes_ago({ m: String(mins) });
  const hours = Math.floor(mins / 60);
  if (hours < 24) return m.format_hours_ago({ h: String(hours) });
  return m.format_days_ago({ d: String(Math.floor(hours / 24)) });
}

export function relTimeFromISO(iso: string, nowMs: number | null): string {
  const t = Date.parse(iso);
  return Number.isNaN(t) ? m.common_em_dash() : relTime(t, nowMs);
}

export function formatDuration(seconds: number): string {
  if (seconds < 0) return m.common_em_dash();
  if (seconds < 60) return `${seconds}s`;
  const mins = Math.floor(seconds / 60);
  if (mins < 60) return `${mins}m ${seconds % 60}s`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ${mins % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

export function formatLocaleDateTime(ms: number, locale = intlLocale(), tz?: string): string {
  return new Date(ms).toLocaleString(locale, tz ? { timeZone: tz } : undefined);
}

export function formatLocaleCount(n: number, locale = intlLocale()): string {
  return new Intl.NumberFormat(locale).format(n);
}
