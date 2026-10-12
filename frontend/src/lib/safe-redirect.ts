const PANEL_ORIGIN = "http://panel.invalid";

/**
 * Returns value when it is a path on this panel, normalized, or undefined.
 * A protocol-relative or absolute value would turn the sign-in page into an
 * open redirect. Parsing against a fixed origin also catches the forms that
 * browsers rewrite, such as backslashes and embedded tabs.
 */
export function safeRedirectPath(value: unknown): string | undefined {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//")) {
    return undefined;
  }
  let url: URL;
  try {
    url = new URL(value, PANEL_ORIGIN);
  } catch {
    return undefined;
  }
  // Dot segments can normalize into a protocol-relative path, as in /.//host.
  if (url.origin !== PANEL_ORIGIN || url.pathname.startsWith("//")) return undefined;
  return url.pathname + url.search + url.hash;
}
