import { getLocale, type Locale } from "~/paraglide/runtime";

/** BCP 47 tag for Intl / React Aria from the active Paraglide locale. */
export function intlLocale(): string {
  return getLocale() === "zh-cn" ? "zh-CN" : "en";
}

export function isZhLocale(locale: Locale = getLocale()): boolean {
  return locale === "zh-cn";
}
