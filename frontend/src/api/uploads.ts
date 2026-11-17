import { fetchPanelConfig, resolvePanelOrigin } from "./panel-config";
import { PanelApiError } from "./queries";
import { ensureSessionFresh } from "./session";
import type { components } from "./schema";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

export type UploadedFile = components["schemas"]["UploadedFile"];

/** Mirrors uploads.MaxBytes on the backend. */
export const MAX_UPLOAD_BYTES = 1 << 30;

const HTTP_PAYLOAD_TOO_LARGE = 413;

type UploadOptions = {
  onProgress?: (fraction: number) => void;
  signal?: AbortSignal;
};

export class UploadAbortedError extends Error {
  constructor() {
    super("upload aborted");
    this.name = "UploadAbortedError";
  }
}

// fetch cannot report request body progress, so large uploads go through XHR.
export async function uploadFile(file: File, options: UploadOptions = {}): Promise<UploadedFile> {
  if (file.size > MAX_UPLOAD_BYTES) {
    throw new PanelApiError(m.error_upload_too_large(), HTTP_PAYLOAD_TOO_LARGE);
  }
  const [auth, config] = await Promise.all([ensureSessionFresh(), fetchPanelConfig()]);
  if (options.signal?.aborted) throw new UploadAbortedError();

  const body = new FormData();
  body.append("file", file, file.name);
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${resolvePanelOrigin(config)}/api/panel/files`);
    if (auth?.token) xhr.setRequestHeader("Authorization", auth.token);
    xhr.responseType = "json";
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) options.onProgress?.(event.loaded / event.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve(xhr.response as UploadedFile);
      else reject(uploadError(xhr));
    };
    xhr.onerror = () => reject(new PanelApiError(m.error_upload_network()));
    xhr.onabort = () => reject(new UploadAbortedError());
    options.signal?.addEventListener("abort", () => xhr.abort(), { once: true });
    xhr.send(body);
  });
}

function uploadError(xhr: XMLHttpRequest): PanelApiError {
  // A proxy in front of the panel may answer 413 with an HTML page.
  if (xhr.status === HTTP_PAYLOAD_TOO_LARGE) {
    return new PanelApiError(m.error_upload_too_large(), xhr.status);
  }
  const response = xhr.response as { message?: unknown } | null;
  const message = typeof response?.message === "string" ? response.message : "";
  return new PanelApiError(localizeApiError(message) || m.error_upload_failed(), xhr.status);
}
