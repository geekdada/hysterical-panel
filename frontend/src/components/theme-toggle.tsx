import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Display, Moon, Sun } from "@gravity-ui/icons";
import { readThemePreference, setThemePreference, type ThemePreference } from "~/lib/theme";
import * as m from "~/paraglide/messages.js";

const OPTIONS: {
  value: ThemePreference;
  label: () => string;
  Icon: typeof Sun;
}[] = [
  { value: "light", label: () => m.theme_light(), Icon: Sun },
  { value: "dark", label: () => m.theme_dark(), Icon: Moon },
  { value: "system", label: () => m.theme_system(), Icon: Display },
];

export function ThemeToggle() {
  const [pref, setPref] = useState<ThemePreference>("system");
  const refs = useRef<(HTMLButtonElement | null)[]>([]);

  useEffect(() => {
    setPref(readThemePreference());
  }, []);

  function select(next: ThemePreference) {
    setPref(next);
    setThemePreference(next);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const dir =
      event.key === "ArrowRight" || event.key === "ArrowDown"
        ? 1
        : event.key === "ArrowLeft" || event.key === "ArrowUp"
          ? -1
          : 0;
    if (dir === 0) return;
    event.preventDefault();
    const current = OPTIONS.findIndex((o) => o.value === pref);
    const nextIndex = (current + dir + OPTIONS.length) % OPTIONS.length;
    const next = OPTIONS[nextIndex];
    if (!next) return;
    select(next.value);
    refs.current[nextIndex]?.focus();
  }

  return (
    <div
      role="radiogroup"
      aria-label={m.aria_theme()}
      onKeyDown={handleKeyDown}
      className="inline-flex items-center rounded-lg border p-0.5"
    >
      {OPTIONS.map(({ value, label, Icon }, index) => {
        const active = pref === value;
        const labelText = label();
        return (
          <button
            key={value}
            ref={(el) => {
              refs.current[index] = el;
            }}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={labelText}
            title={labelText}
            tabIndex={active ? 0 : -1}
            onClick={() => select(value)}
            className={`grid size-6 place-items-center rounded-[5px] transition-colors duration-150 ${
              active ? "bg-surface-secondary text-foreground" : "text-muted hover:text-foreground"
            }`}
          >
            <Icon className="size-3.5" aria-hidden />
          </button>
        );
      })}
    </div>
  );
}
