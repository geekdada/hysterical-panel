import { useEffect, useState } from "react";

/**
 * A live clock whose server render and first client render are deterministic.
 * The real time is installed after hydration, then refreshed at the requested interval.
 */
export function useHydratedNow(intervalMs = 5_000): number | null {
  const [now, setNow] = useState<number | null>(null);

  useEffect(() => {
    const updateNow = () => setNow(Date.now());
    updateNow();
    const id = window.setInterval(updateNow, intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);

  return now;
}
