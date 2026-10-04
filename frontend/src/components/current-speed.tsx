import type { ReactNode } from "react";
import { ErrorAlert, Section, TableSkeleton, Td, Th } from "~/components/ui";
import { formatBytesPerSecond } from "~/lib/format";
import * as m from "~/paraglide/messages.js";

export type CurrentSpeedRow = {
  id: string;
  label: ReactNode;
  tx: number;
  rx: number;
};

export function CurrentSpeedSection({
  hint,
  nameHeader,
  total,
  rows,
  loading,
  error,
}: {
  hint?: string;
  nameHeader: string;
  total: { tx?: number; rx?: number } | null;
  rows: CurrentSpeedRow[];
  loading: boolean;
  error: string;
}) {
  return (
    <Section
      title={m.current_speed_title()}
      hint={hint}
      meta={
        total ? (
          <span className="font-mono tabular-nums">
            ↑ {formatBytesPerSecond(total.tx ?? 0)} · ↓ {formatBytesPerSecond(total.rx ?? 0)}
          </span>
        ) : undefined
      }
    >
      {loading ? (
        <TableSkeleton rows={2} />
      ) : error ? (
        <div className="p-3">
          <ErrorAlert message={error} />
        </div>
      ) : rows.length === 0 ? (
        <p className="px-4 py-6 text-center text-[13px] text-muted">{m.current_speed_empty()}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-border bg-surface-secondary text-left">
                <Th>{nameHeader}</Th>
                <Th className="text-right">{m.common_th_tx()}</Th>
                <Th className="text-right">{m.common_th_rx()}</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-separator">
              {rows.map((row) => (
                <tr
                  key={row.id}
                  className="transition-colors duration-150 hover:bg-surface-secondary"
                >
                  <Td>{row.label}</Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-muted">↑</span> {formatBytesPerSecond(row.tx)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-muted">↓</span> {formatBytesPerSecond(row.rx)}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
}
