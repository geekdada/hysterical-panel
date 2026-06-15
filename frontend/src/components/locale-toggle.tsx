import {
  useEffect,
  useRef,
  useState,
  type ComponentType,
  type KeyboardEvent,
  type SVGProps,
} from "react";
import { useRouter } from "@tanstack/react-router";
import { getLocale, locales, setLocale, type Locale } from "~/paraglide/runtime";
import * as m from "~/paraglide/messages.js";

const OPTIONS: {
  value: Locale;
  label: () => string;
  short: string;
}[] = [
  { value: "en", label: () => m.locale_en(), short: "EN" },
  { value: "zh-cn", label: () => m.locale_zh_cn(), short: "中" },
];

export function LocaleToggle() {
  const router = useRouter();
  const [locale, setLocalLocale] = useState<Locale>("en");
  const refs = useRef<(HTMLButtonElement | null)[]>([]);

  useEffect(() => {
    setLocalLocale(getLocale());
  }, []);

  function select(next: Locale) {
    if (!locales.includes(next)) return;
    setLocalLocale(next);
    setLocale(next);
    void router.invalidate();
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
    const current = OPTIONS.findIndex((o) => o.value === locale);
    const nextIndex = (current + dir + OPTIONS.length) % OPTIONS.length;
    const next = OPTIONS[nextIndex];
    if (!next) return;
    select(next.value);
    refs.current[nextIndex]?.focus();
  }

  return (
    <div
      role="radiogroup"
      aria-label={m.aria_language()}
      onKeyDown={handleKeyDown}
      className="inline-flex items-center rounded-(--radius) border border-(--border) p-0.5"
    >
      {OPTIONS.map(({ value, label, short }, index) => {
        const active = locale === value;
        return (
          <button
            key={value}
            ref={(el) => {
              refs.current[index] = el;
            }}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={label()}
            title={label()}
            tabIndex={active ? 0 : -1}
            onClick={() => select(value)}
            className={`min-w-6 px-1.5 text-[11px] font-medium leading-6 rounded-[5px] transition-colors duration-150 ${
              active
                ? "bg-(--surface-secondary) text-(--foreground)"
                : "text-(--muted) hover:text-(--foreground)"
            }`}
          >
            {short}
          </button>
        );
      })}
    </div>
  );
}
