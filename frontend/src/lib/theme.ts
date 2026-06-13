export type ThemePreference = "light" | "dark" | "system";

// Mirror this key and the accepted values in the inline no-flash script in
// __root.tsx — that script runs before the bundle loads and can't import here.
export const THEME_STORAGE_KEY = "hp:theme";

/** Read the saved preference, defaulting to "system" (SSR-safe). */
export function readThemePreference(): ThemePreference {
  if (typeof localStorage === "undefined") return "system";
  const value = localStorage.getItem(THEME_STORAGE_KEY);
  return value === "light" || value === "dark" ? value : "system";
}

function resolveTheme(pref: ThemePreference): "light" | "dark" {
  if (pref !== "system") return pref;
  if (typeof window === "undefined") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** Apply the resolved theme to <html> (class + data-theme + color-scheme). */
export function applyThemePreference(pref: ThemePreference): void {
  if (typeof document === "undefined") return;
  const dark = resolveTheme(pref) === "dark";
  const el = document.documentElement;
  el.classList.remove(dark ? "light" : "dark");
  el.classList.add(dark ? "dark" : "light");
  el.setAttribute("data-theme", dark ? "dark" : "light");
  el.style.colorScheme = dark ? "dark" : "light";
}

/** Persist the preference and apply it immediately. */
export function setThemePreference(pref: ThemePreference): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, pref);
  } catch {
    // Storage unavailable (private mode / disabled) — still apply for this session.
  }
  applyThemePreference(pref);
}
