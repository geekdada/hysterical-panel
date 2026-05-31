import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { formatBytes } from "~/lib/format";

export type Granularity = "hourly" | "daily";

export type TrafficPoint = {
  bucket?: string;
  rx?: number;
  tx?: number;
};

export const RANGE_MS: Record<Granularity, number> = {
  hourly: 48 * 60 * 60 * 1000,
  daily: 30 * 24 * 60 * 60 * 1000,
};

export function GranularityToggle({
  value,
  onChange,
}: {
  value: Granularity;
  onChange: (g: Granularity) => void;
}) {
  const opts: Granularity[] = ["hourly", "daily"];
  return (
    <div className="inline-flex rounded-(--radius) border border-(--border) p-0.5">
      {opts.map((o) => (
        <button
          key={o}
          type="button"
          onClick={() => onChange(o)}
          className={`rounded-[calc(var(--radius)-2px)] px-2.5 py-1 text-xs font-medium capitalize transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) ${
            value === o
              ? "bg-(--surface-secondary) text-(--foreground)"
              : "text-(--muted) hover:text-(--foreground)"
          }`}
          aria-pressed={value === o}
        >
          {o}
        </button>
      ))}
    </div>
  );
}

export function TrafficChart({
  points,
  granularity,
  idPrefix,
}: {
  points: TrafficPoint[];
  granularity: Granularity;
  idPrefix: string;
}) {
  const data = points.map((p) => ({
    label: bucketLabel(p.bucket ?? "", granularity),
    rx: p.rx ?? 0,
    tx: p.tx ?? 0,
  }));
  const rxFill = `${idPrefix}-rx-fill`;
  const txFill = `${idPrefix}-tx-fill`;

  return (
    <ResponsiveContainer width="100%" height={220}>
      <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={rxFill} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--accent)" stopOpacity={0.35} />
            <stop offset="100%" stopColor="var(--accent)" stopOpacity={0.04} />
          </linearGradient>
          <linearGradient id={txFill} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--muted)" stopOpacity={0.28} />
            <stop offset="100%" stopColor="var(--muted)" stopOpacity={0.03} />
          </linearGradient>
        </defs>
        <CartesianGrid stroke="var(--separator)" vertical={false} />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 11, fill: "var(--muted)" }}
          tickLine={false}
          axisLine={{ stroke: "var(--border)" }}
          minTickGap={28}
          interval="preserveStartEnd"
        />
        <YAxis
          tick={{ fontSize: 11, fill: "var(--muted)" }}
          tickLine={false}
          axisLine={false}
          width={56}
          tickFormatter={(v: number) => formatBytes(v)}
        />
        <Tooltip
          content={<ChartTooltip />}
          cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
        />
        <Area
          type="monotone"
          dataKey="rx"
          name="RX"
          stackId="t"
          stroke="var(--accent)"
          strokeWidth={1.5}
          fill={`url(#${rxFill})`}
          activeDot={{ r: 3, fill: "var(--accent)", stroke: "var(--surface)" }}
          animationDuration={250}
        />
        <Area
          type="monotone"
          dataKey="tx"
          name="TX"
          stackId="t"
          stroke="var(--muted)"
          strokeWidth={1.5}
          fill={`url(#${txFill})`}
          activeDot={{ r: 3, fill: "var(--muted)", stroke: "var(--surface)" }}
          animationDuration={250}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

type TooltipPayload = { dataKey?: string | number; value?: number };

function ChartTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: TooltipPayload[];
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  const rx = payload.find((p) => p.dataKey === "rx")?.value ?? 0;
  const tx = payload.find((p) => p.dataKey === "tx")?.value ?? 0;
  return (
    <div className="rounded-(--radius) border border-(--border) bg-(--surface) px-2.5 py-1.5 text-xs shadow-sm">
      <div className="mb-1 font-medium text-(--foreground)">{label}</div>
      <div className="flex flex-col gap-0.5 font-mono tabular-nums text-(--muted)">
        <span>
          <span className="text-(--accent)">↓</span> {formatBytes(rx)}
        </span>
        <span>↑ {formatBytes(tx)}</span>
        <span className="text-(--foreground)">Σ {formatBytes(rx + tx)}</span>
      </div>
    </div>
  );
}

export function toPbDateTime(d: Date): string {
  const pad = (n: number, w = 2) => String(n).padStart(w, "0");
  return (
    `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ` +
    `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}.${pad(d.getUTCMilliseconds(), 3)}Z`
  );
}

function bucketLabel(bucket: string, granularity: Granularity): string {
  const ms = Date.parse(bucket.replace(" ", "T"));
  if (Number.isNaN(ms)) return bucket;
  const d = new Date(ms);
  if (granularity === "hourly") {
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString([], { month: "numeric", day: "numeric" });
}
