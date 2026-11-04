import { useCallback, useEffect, useRef, useState } from "react";

// Pixels a section may sit below its scroll-margin line and still count as reached.
const REACHED_TOLERANCE_PX = 8;

/**
 * Scroll spy for in-page anchors. The active section is the last one whose top
 * has scrolled up to its own `scroll-margin-top`, so it agrees with where
 * anchor jumps land. At the page bottom the last section wins, because short
 * trailing sections can never reach the top. `pin` keeps a clicked section
 * active until the user scrolls by hand, since a jump near the bottom would
 * otherwise highlight a different item than the one clicked. Pass a memoized
 * `ids` array, since a new array re-binds the listeners.
 */
export function useActiveSection(ids: readonly string[]) {
  const [activeId, setActiveId] = useState<string | null>(ids[0] ?? null);
  const pinnedRef = useRef<string | null>(null);

  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      setActiveId(pinnedRef.current ?? findActiveSection(ids));
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    const unpin = () => {
      pinnedRef.current = null;
    };
    const unpinOnScrollKey = (event: KeyboardEvent) => {
      if (SCROLL_KEYS.has(event.key)) unpin();
    };

    update();
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    window.addEventListener("wheel", unpin, { passive: true });
    window.addEventListener("touchstart", unpin, { passive: true });
    window.addEventListener("keydown", unpinOnScrollKey);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      window.removeEventListener("wheel", unpin);
      window.removeEventListener("touchstart", unpin);
      window.removeEventListener("keydown", unpinOnScrollKey);
    };
  }, [ids]);

  const pin = useCallback((id: string) => {
    pinnedRef.current = id;
    setActiveId(id);
  }, []);

  return { activeId, pin };
}

const SCROLL_KEYS = new Set(["ArrowUp", "ArrowDown", "PageUp", "PageDown", "Home", "End", " "]);

function findActiveSection(ids: readonly string[]): string | null {
  const elements = ids
    .map((id) => document.getElementById(id))
    .filter((el): el is HTMLElement => el !== null);
  const first = elements[0];
  const last = elements[elements.length - 1];
  if (!first || !last) return null;

  const root = document.documentElement;
  if (window.innerHeight + window.scrollY >= root.scrollHeight - 2) return last.id;

  let current = first.id;
  for (const el of elements) {
    const margin = parseFloat(getComputedStyle(el).scrollMarginTop) || 0;
    if (el.getBoundingClientRect().top - margin > REACHED_TOLERANCE_PX) break;
    current = el.id;
  }
  return current;
}
