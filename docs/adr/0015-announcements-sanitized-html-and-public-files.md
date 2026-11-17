---
status: accepted
---

# Render sanitized Announcement HTML in the page and keep uploads as public generic files

Announcements are read inside the panel, so their body is rendered directly in the page rather than in the sandboxed iframe ADR-0008 uses for Sendout HTML. The administrator writes it in a `@tiptap/react` editor. The browser submits HTML together with the TipTap JSON, and the backend runs the HTML through a `bluemonday` allowlist limited to what the editor can produce before storing it. `src` may only be a relative `/api/files/` path. `href` may also be `http:`, `https:` or `mailto:`. The panel token cookie is readable by scripts, so the backend does not trust the browser even though only administrators write Announcements.

Files uploaded from the editor go into a generic `files` collection, not one owned by Announcements. Each row holds one file field that is not protected and has a 1 GB `MaxSize`. Anyone holding the URL can download the file without signing in, which lets a CDN cache it. The body stores only the relative `/api/files/...` path, and the frontend joins it with the API base when it renders, so a domain change or a new CDN does not rewrite stored content. Deleting an Announcement leaves its files and their URLs in place, and nothing cleans up unused files.

## Considered options

- Store only TipTap JSON and render it with `generateHTML` in the browser. We rejected it because the backend would then never check what reaches the page.
- Render Announcements in a sandboxed iframe like Sendouts. We rejected it because the body must follow the panel theme and size to its content.
- An Announcement-owned file collection with cascade delete and orphan cleanup. We rejected it in favor of a generic store that other features can reuse.
- Quill as the editor. We rejected it because the panel already ships TipTap.

## Consequences

- Attachments must not carry sensitive content. Hiding or deleting an Announcement does not revoke its files, and a CDN may keep serving them.
- Uploading any file type is safe for the panel origin. PocketBase serves everything outside its inline list (images, video, audio, PDF) as `attachment`, and every file response carries a `sandbox` CSP.
- The upload route raises PocketBase's 32 MB request body limit to match the field, and the nginx config needs the same allowance on that path. Deployments behind another proxy or CDN must raise its upload limit too.
- Storage grows without bound until file management exists.
