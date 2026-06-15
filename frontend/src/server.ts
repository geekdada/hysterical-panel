import { paraglideMiddleware } from "./paraglide/server.js";
import handler from "@tanstack/react-start/server-entry";

export default {
  fetch(req: Request) {
    return paraglideMiddleware(req, () => handler.fetch(req));
  },
};
