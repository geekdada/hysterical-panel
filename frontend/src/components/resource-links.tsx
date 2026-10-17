import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ListBox } from "@heroui/react";
import { Globe } from "@gravity-ui/icons";
import { canQueryPanelApi, fetchResourceLinks, queryKeys } from "~/api/queries";
import * as m from "~/paraglide/messages.js";

export function ResourceLinkFavicon({ id, version }: { id: string; version: string }) {
  const [failedVersion, setFailedVersion] = useState<string | null>(null);

  if (!version || failedVersion === version) {
    return <Globe className="size-4 text-muted" aria-hidden />;
  }

  return (
    <img
      src={`/l/${id}/favicon?v=${version}`}
      alt=""
      className="size-4"
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
    <ListBox
      aria-label={m.resource_links_list_label()}
      layout="grid"
      selectionMode="none"
      className="mt-6 grid grid-cols-1 gap-1 p-0 sm:grid-cols-2 lg:grid-cols-3 [&>*+*]:mt-0"
    >
      {links.map((link) => (
        <ListBox.Item
          key={link.id}
          id={link.id}
          textValue={link.title}
          href={`/l/${link.id}`}
          target="_blank"
          rel="noopener noreferrer"
        >
          <ResourceLinkFavicon id={link.id} version={link.favicon_version} />
          <span className="min-w-0 truncate text-[13px] text-foreground">{link.title}</span>
        </ListBox.Item>
      ))}
    </ListBox>
  );
}
