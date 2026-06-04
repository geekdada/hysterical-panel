import { useEffect, useState } from "react";

/** True after the first client commit (skip SSR/hydration for locale/time UI). */
export function useMounted(): boolean {
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  return mounted;
}
