import { createFileRoute } from "@tanstack/react-router";
import { resolveServerApiBaseUrl } from "~/api/panel-config";

const FORWARDED_HEADERS = [
  "Content-Type",
  "Cache-Control",
  "ETag",
  "X-Content-Type-Options",
] as const;

export const Route = createFileRoute("/l/$id/favicon")({
  server: {
    handlers: {
      GET: async ({ request, params }) => {
        const upstreamUrl = `${resolveServerApiBaseUrl()}/api/resource-links/${encodeURIComponent(params.id)}/favicon`;
        const upstreamHeaders = new Headers();
        const ifNoneMatch = request.headers.get("If-None-Match");
        if (ifNoneMatch) upstreamHeaders.set("If-None-Match", ifNoneMatch);

        let upstream: Response;
        try {
          upstream = await fetch(upstreamUrl, { headers: upstreamHeaders });
        } catch {
          return new Response("Bad gateway", { status: 502 });
        }

        const responseHeaders = new Headers();
        for (const name of FORWARDED_HEADERS) {
          const value = upstream.headers.get(name);
          if (value) responseHeaders.set(name, value);
        }

        // 304 must not carry a body, while 200 keeps the upstream stream.
        const body = upstream.status === 304 ? null : upstream.body;
        return new Response(body, { status: upstream.status, headers: responseHeaders });
      },
    },
  },
});
