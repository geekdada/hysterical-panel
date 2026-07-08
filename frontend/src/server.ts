import { paraglideMiddleware } from "./paraglide/server.js";
import handler from "@tanstack/react-start/server-entry";
import { errorMessage, serverLog } from "~/lib/server-log";

export default {
  async fetch(req: Request) {
    try {
      return await paraglideMiddleware(req, () => handler.fetch(req));
    } catch (error) {
      serverLog.error("ssr_unhandled", { message: errorMessage(error) });
      throw error;
    }
  },
};
