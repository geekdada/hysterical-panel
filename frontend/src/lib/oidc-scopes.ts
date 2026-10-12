import { getLocale } from "~/paraglide/runtime";
import * as m from "~/paraglide/messages.js";

const SCOPE_LABELS = new Map<string, () => string>([
  ["openid", m.oidc_scope_openid],
  ["email", m.oidc_scope_email],
  ["offline_access", m.oidc_scope_offline_access],
]);

/** Describes what a Connected App receives for a scope. */
export function scopeLabel(scope: string): string {
  return SCOPE_LABELS.get(scope)?.() ?? scope;
}

/** Joins scope labels the way the current locale writes a list. */
export function formatScopes(scopes: string[]): string {
  return new Intl.ListFormat(getLocale(), { type: "conjunction" }).format(scopes.map(scopeLabel));
}
