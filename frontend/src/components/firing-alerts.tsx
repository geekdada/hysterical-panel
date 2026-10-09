import { Link } from "@tanstack/react-router";
import type { AlertListResponse } from "~/api/queries";
import { Section, SeverityBadge } from "~/components/ui";
import { relTimeFromISO } from "~/lib/format";
import * as m from "~/paraglide/messages.js";

type AlertItem = NonNullable<AlertListResponse["items"]>[number];

/** Firing Alerts for one Node or one User, each linking to Monitoring. Renders nothing when clear. */
export function FiringAlertsSection({ alerts, now }: { alerts: AlertItem[]; now: number | null }) {
  const firing = alerts.filter((alert) => alert.status === "firing");
  if (firing.length === 0) return null;
  return (
    <Section title={m.monitoring_node_alerts()} meta={String(firing.length)}>
      <div className="divide-y divide-separator">
        {firing.map((alert) => (
          <Link
            key={alert.id}
            to="/settings/monitoring"
            className="flex items-center gap-3 px-3 py-2.5 text-foreground no-underline hover:bg-surface-secondary"
          >
            <SeverityBadge
              severity={alert.severity === "critical" ? "critical" : "warning"}
              label={
                alert.severity === "critical" ? m.monitoring_critical() : m.monitoring_warning()
              }
            />
            <span className="min-w-0 flex-1 truncate text-[13px] font-medium">
              {alert.monitor_name ?? m.monitoring_monitor()}
            </span>
            <span className="text-xs text-muted">
              {alert.started_at ? relTimeFromISO(alert.started_at, now) : m.common_em_dash()}
            </span>
          </Link>
        ))}
      </div>
    </Section>
  );
}
