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
              // Stored URLs keep their original text, so non-ASCII paths and IDN
              // hosts must be serialized before they are valid header values.
              Location: new URL(url).href,
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
