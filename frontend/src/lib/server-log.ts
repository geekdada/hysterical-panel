export type LogLevel = "error" | "warn" | "info" | "debug";

const LEVEL_RANK: Record<LogLevel, number> = {
  error: 0,
  warn: 1,
  info: 2,
  debug: 3,
};

declare const process:
  | {
      env?: {
        PANEL_LOG_LEVEL?: string;
      };
    }
  | undefined;

export type ServerLogFields = Record<
  string,
  string | number | boolean | undefined | null | readonly unknown[]
>;

function isServerRuntime(): boolean {
  return typeof window === "undefined";
}

function currentLevel(): LogLevel {
  const raw = typeof process !== "undefined" ? process.env?.PANEL_LOG_LEVEL : undefined;
  if (raw === "error" || raw === "warn" || raw === "info" || raw === "debug") return raw;
  return "info";
}

function shouldLog(level: LogLevel): boolean {
  return isServerRuntime() && LEVEL_RANK[level] <= LEVEL_RANK[currentLevel()];
}

/** Hostname[:port] from an absolute base URL; undefined if unparseable. */
export function hostFromBaseUrl(base: string): string | undefined {
  if (!base) return undefined;
  try {
    const url = new URL(base);
    return url.port ? `${url.hostname}:${url.port}` : url.hostname;
  } catch {
    return undefined;
  }
}

export function statusFromError(error: unknown): number | undefined {
  if (error && typeof error === "object" && "status" in error) {
    const status = (error as { status: unknown }).status;
    if (typeof status === "number" && Number.isFinite(status)) return status;
  }
  return undefined;
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "string" && error) return error;
  return String(error);
}

function formatValue(value: NonNullable<ServerLogFields[string]>): string {
  if (Array.isArray(value)) {
    try {
      return JSON.stringify(value);
    } catch {
      return "[unserializable]";
    }
  }
  return String(value);
}

function formatFields(fields?: ServerLogFields): string {
  if (!fields) return "";
  const parts: string[] = [];
  for (const [key, value] of Object.entries(fields)) {
    if (value === undefined || value === null) continue;
    parts.push(`${key}=${formatValue(value)}`);
  }
  return parts.length ? ` ${parts.join(" ")}` : "";
}

function write(level: LogLevel, event: string, fields?: ServerLogFields): void {
  if (!shouldLog(level)) return;
  const line = `[panel] ${level} ${event}${formatFields(fields)}`;
  if (level === "error") {
    console.error(line);
    return;
  }
  if (level === "warn") {
    console.warn(line);
    return;
  }
  console.log(line);
}

export const serverLog = {
  error(event: string, fields?: ServerLogFields): void {
    write("error", event, fields);
  },
  warn(event: string, fields?: ServerLogFields): void {
    write("warn", event, fields);
  },
  info(event: string, fields?: ServerLogFields): void {
    write("info", event, fields);
  },
  debug(event: string, fields?: ServerLogFields): void {
    write("debug", event, fields);
  },
  log(level: LogLevel, event: string, fields?: ServerLogFields): void {
    write(level, event, fields);
  },
};
