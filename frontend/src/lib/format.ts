// Shared formatting helpers. Bytes use binary units; relative time is the live
// "Ns ago" readout used across the panel. All durations are clamped at zero so
// clock skew never renders a negative age.

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  return `${val.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function relTime(fromMs: number, nowMs: number): string {
  const s = Math.max(0, Math.round((nowMs - fromMs) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

export function relTimeFromISO(iso: string, nowMs: number): string {
  const t = Date.parse(iso);
  return Number.isNaN(t) ? "—" : relTime(t, nowMs);
}

export function plural(n: number, word: string): string {
  return n === 1 ? word : `${word}s`;
}

// formatDuration renders a span of seconds compactly (e.g. "3m 12s", "1h 4m").
// Used for stream lifetime/idle readouts. Negative inputs (parse failures) -> "—".
export function formatDuration(seconds: number): string {
  if (seconds < 0) return "—";
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}m ${seconds % 60}s`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  const d = Math.floor(h / 24);
  return `${d}d ${h % 24}h`;
}
