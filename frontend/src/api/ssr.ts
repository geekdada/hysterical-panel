import { createIsomorphicFn } from "@tanstack/react-start";
import { setResponseHeader } from "@tanstack/react-start/server";

/** Mark the SSR response as private so auth-bearing HTML is never cached. */
export const markResponsePrivate = createIsomorphicFn()
  .server(() => {
    setResponseHeader("cache-control", "private, no-store");
  })
  .client(() => {});
