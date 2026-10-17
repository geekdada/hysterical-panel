import { createFileRoute } from "@tanstack/react-router";
import { visitResourceLink } from "~/api/queries";

const NO_STORE = { "Cache-Control": "no-store" };

export const Route = createFileRoute("/l/$id/")({
  server: {
    handlers: {
      GET: async ({ params }) => {
        try {
          const url = await visitResourceLink(params.id);
          if (!url) {
            return new Response("Not found", { status: 404, headers: NO_STORE });
          }
          return new Response(null, {
            status: 302,
            headers: {
              ...NO_STORE,
              Location: url,
              "Referrer-Policy": "no-referrer",
            },
          });
        } catch {
          return new Response("Bad gateway", { status: 502, headers: NO_STORE });
        }
      },
    },
  },
});
