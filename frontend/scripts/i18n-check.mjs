// Verifies that every message key is translated in every configured locale.
//
// Reads the locales and path pattern from project.inlang/settings.json so the
// check stays correct when locales are added. A key fails when it is missing
// from a locale, or present but empty/whitespace-only. Exits non-zero on any
// problem so it can gate CI and the `pnpm check` aggregate.

import { readFileSync } from "node:fs";

const settingsUrl = new URL("../project.inlang/settings.json", import.meta.url);
const settings = JSON.parse(readFileSync(settingsUrl, "utf8"));

const locales = settings.locales ?? [];
const baseLocale = settings.baseLocale ?? locales[0];
const pathPattern = settings["plugin.inlang.messageFormat"]?.pathPattern;

if (!pathPattern || !pathPattern.includes("{locale}")) {
  console.error(
    "i18n-check: could not resolve a {locale} path pattern from project.inlang/settings.json"
  );
  process.exit(1);
}

if (locales.length < 2) {
  console.error("i18n-check: fewer than two locales configured; nothing to compare");
  process.exit(1);
}

// Keys reserved by the message-format plugin (e.g. "$schema"), not real messages.
const isReservedKey = (key) => key.startsWith("$");

const isTranslated = (value) => typeof value === "string" && value.trim().length > 0;

const loadMessages = (locale) => {
  const url = new URL("../" + pathPattern.replace("{locale}", locale), import.meta.url);
  return JSON.parse(readFileSync(url, "utf8"));
};

const messagesByLocale = new Map();
for (const locale of locales) {
  messagesByLocale.set(locale, loadMessages(locale));
}

// The base locale is the source of truth for the expected key set; we also fold
// in keys from other locales so stray keys (present elsewhere but not in base)
// surface as "missing in base" rather than passing silently.
const allKeys = new Set();
for (const messages of messagesByLocale.values()) {
  for (const key of Object.keys(messages)) {
    if (!isReservedKey(key)) allKeys.add(key);
  }
}

const missing = new Map(); // locale -> string[]
const empty = new Map(); // locale -> string[]
for (const locale of locales) {
  missing.set(locale, []);
  empty.set(locale, []);
}

for (const key of [...allKeys].sort()) {
  for (const locale of locales) {
    const value = messagesByLocale.get(locale)[key];
    if (value === undefined) {
      missing.get(locale).push(key);
    } else if (!isTranslated(value)) {
      empty.get(locale).push(key);
    }
  }
}

let failed = false;
for (const locale of locales) {
  const missingKeys = missing.get(locale);
  const emptyKeys = empty.get(locale);
  if (missingKeys.length === 0 && emptyKeys.length === 0) continue;

  failed = true;
  console.error(`\n✗ ${locale}${locale === baseLocale ? " (base)" : ""}`);
  if (missingKeys.length > 0) {
    console.error(`  missing ${missingKeys.length} key(s):`);
    for (const key of missingKeys) console.error(`    - ${key}`);
  }
  if (emptyKeys.length > 0) {
    console.error(`  empty ${emptyKeys.length} value(s):`);
    for (const key of emptyKeys) console.error(`    - ${key}`);
  }
}

if (failed) {
  console.error(
    `\ni18n-check failed: ${allKeys.size} keys × ${locales.length} locales (${locales.join(", ")}) are not fully translated.`
  );
  process.exit(1);
}

console.log(`i18n-check passed: ${allKeys.size} keys translated across ${locales.join(", ")}`);
