/** Mirrors storedFilePattern in backend/internal/announcements/sanitize.go. */
const STORED_FILE_PATH =
  /^\/api\/files\/files\/[a-z0-9]{15}\/[A-Za-z0-9_-]+(\.[A-Za-z0-9_*+=%-]+)*$/;

export const DOWNLOAD_QUERY = "?download=1";

const EXTERNAL_LINK = /^(https?:|mailto:)/i;

/** Image types browsers display inline. Other uploads become download links. */
const INLINE_IMAGE_TYPES = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
  "image/avif",
  "image/bmp",
  "image/svg+xml",
]);

export function isInlineImage(mime: string): boolean {
  return INLINE_IMAGE_TYPES.has(mime.toLowerCase());
}

/**
 * Turns a stored file URL back into the relative path the backend accepts.
 * The editor shows absolute URLs, so content pasted from a rendered page
 * carries the panel API origin. Returns null for anything else.
 */
export function storedFilePath(src: string, origin: string): string | null {
  const path = origin && src.startsWith(`${origin}/`) ? src.slice(origin.length) : src;
  return STORED_FILE_PATH.test(path) ? path : null;
}

/** Mirrors linkTargetPattern in the backend sanitizer. */
export function isAllowedLinkTarget(href: string): boolean {
  if (EXTERNAL_LINK.test(href)) return true;
  const path = href.endsWith(DOWNLOAD_QUERY) ? href.slice(0, -DOWNLOAD_QUERY.length) : href;
  return STORED_FILE_PATH.test(path);
}

/** Stored paths are relative to the panel API, which may sit on another origin. */
export function panelFileUrl(path: string, origin: string): string {
  return path.startsWith("/") ? `${origin}${path}` : path;
}
