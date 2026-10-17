import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Surface } from "@heroui/react";
import { Globe } from "@gravity-ui/icons";
import { canQueryPanelApi, fetchResourceLinks, queryKeys } from "~/api/queries";
import * as m from "~/paraglide/messages.js";

export function ResourceLinkFavicon({ id, version }: { id: string; version: string }) {
  const [failedVersion, setFailedVersion] = useState<string | null>(null);

  if (!version || failedVersion === version) {
    return <Globe className="size-4 shrink-0 text-muted" aria-hidden />;
  }

  return (
    <img
      src={`/l/${id}/favicon?v=${version}`}
      alt=""
      className="size-4 shrink-0"
      onError={() => setFailedVersion(version)}
    />
  );
}

export function ResourceLinks() {
  const linksQuery = useQuery({
    queryKey: queryKeys.resourceLinks(),
    queryFn: fetchResourceLinks,
    enabled: canQueryPanelApi(),
  });

  if (linksQuery.isPending || linksQuery.isError) return null;

  const links = linksQuery.data ?? [];
  if (links.length === 0) return null;

  return (
    <Surface className="mt-6 w-full rounded-lg border px-4 py-1">
      <ul aria-label={m.resource_links_list_label()} className="divide-y divide-border">
        {links.map((link) => (
          <li key={link.id} className="min-w-0 max-w-full py-2">
            <Link href={`/l/${link.id}`} target="_blank" rel="noopener noreferrer">
              <div className="mr-2">
                <ResourceLinkFavicon id={link.id} version={link.favicon_version} />
              </div>
              <span className="min-w-0 truncate text-sm">{link.title}</span>
              <Link.Icon />
            </Link>
          </li>
        ))}
      </ul>
    </Surface>
  );
}
