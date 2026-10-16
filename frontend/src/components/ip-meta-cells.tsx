import type { ReactNode } from "react";
import type { components } from "~/api/schema";
import { Td } from "~/components/ui";
import * as m from "~/paraglide/messages.js";

type IPMeta = NonNullable<components["schemas"]["LoginIP"]["ip_meta"]>;

// Renders the IP, ASN and country cells shared by the user page IP tables.
export function IPMetaCells({
  ip,
  meta,
  badge,
}: {
  ip: string | undefined;
  meta: IPMeta | null | undefined;
  badge?: ReactNode;
}) {
  const label = ip || m.common_em_dash();
  const countryTitle = meta?.country_name || meta?.country_code || "";
  return (
    <>
      <Td>
        <div className="flex min-w-0 items-center gap-2">
          {meta?.ipinfo_url ? (
            <a
              href={meta.ipinfo_url}
              target="_blank"
              rel="noreferrer"
              className="block max-w-[260px] truncate font-mono text-xs text-foreground underline decoration-border underline-offset-2 transition-colors duration-150 hover:text-accent"
              title={m.common_ipinfo_open({ target: meta.ip || label })}
            >
              {label}
            </a>
          ) : (
            <span className="block max-w-[260px] truncate font-mono text-xs" title={label}>
              {label}
            </span>
          )}
          {badge}
        </div>
      </Td>
      <Td className="whitespace-nowrap font-mono text-xs text-muted">
        {meta?.asn || m.common_em_dash()}
      </Td>
      <Td className="whitespace-nowrap text-xs text-muted">
        {meta?.country_code ? (
          <span title={countryTitle}>
            <span className="font-mono text-foreground">{meta.country_code}</span>
            {meta.country_name && (
              <span className="ml-1 hidden max-w-[140px] truncate align-bottom sm:inline-block">
                {meta.country_name}
              </span>
            )}
          </span>
        ) : (
          m.common_em_dash()
        )}
      </Td>
    </>
  );
}
