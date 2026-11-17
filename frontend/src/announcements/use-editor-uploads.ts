import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor, EditorEvents, JSONContent } from "@tiptap/react";
import { UploadAbortedError, uploadFile, type UploadedFile } from "~/api/uploads";
import { DOWNLOAD_QUERY, isInlineImage } from "~/lib/panel-files";
import * as m from "~/paraglide/messages.js";

export type EditorUpload = {
  id: string;
  name: string;
  progress: number;
  error: string | null;
};

type Pending = { position: number; controller: AbortController };

function uploadedContent(file: UploadedFile): JSONContent {
  if (isInlineImage(file.mime)) {
    return { type: "image", attrs: { src: file.path, alt: file.name } };
  }
  return {
    type: "paragraph",
    content: [
      {
        type: "text",
        text: file.name,
        marks: [{ type: "link", attrs: { href: file.path + DOWNLOAD_QUERY, download: file.name } }],
      },
    ],
  };
}

/**
 * Uploads files and inserts each one where it was dropped or pasted. The
 * position follows later edits, so typing during a long upload does not move
 * the file into the wrong place.
 */
export function useEditorUploads(editor: Editor | null) {
  const [uploads, setUploads] = useState<EditorUpload[]>([]);
  const pending = useRef(new Map<string, Pending>());

  useEffect(() => {
    if (!editor) return;
    const follow = ({ transaction }: EditorEvents["transaction"]) => {
      for (const entry of pending.current.values()) {
        // Mapping to the right keeps several files dropped together in order.
        entry.position = transaction.mapping.map(entry.position, 1);
      }
    };
    editor.on("transaction", follow);
    return () => {
      editor.off("transaction", follow);
    };
  }, [editor]);

  useEffect(() => {
    const entries = pending.current;
    return () => {
      for (const entry of entries.values()) entry.controller.abort();
    };
  }, []);

  const patch = useCallback((id: string, change: Partial<EditorUpload>) => {
    setUploads((list) => list.map((item) => (item.id === id ? { ...item, ...change } : item)));
  }, []);

  const uploadOne = useCallback(
    async (file: File, position: number) => {
      const id = crypto.randomUUID();
      const controller = new AbortController();
      pending.current.set(id, { position, controller });
      setUploads((list) => [...list, { id, name: file.name, progress: 0, error: null }]);
      try {
        const stored = await uploadFile(file, {
          signal: controller.signal,
          onProgress: (progress) => patch(id, { progress }),
        });
        const at = pending.current.get(id)?.position ?? position;
        pending.current.delete(id);
        editor?.chain().insertContentAt(at, uploadedContent(stored)).run();
        setUploads((list) => list.filter((item) => item.id !== id));
      } catch (error) {
        pending.current.delete(id);
        if (error instanceof UploadAbortedError) {
          setUploads((list) => list.filter((item) => item.id !== id));
          return;
        }
        patch(id, { error: error instanceof Error ? error.message : m.error_upload_failed() });
      }
    },
    [editor, patch]
  );

  const start = useCallback(
    (files: Iterable<File>, position: number) => {
      for (const file of files) void uploadOne(file, position);
    },
    [uploadOne]
  );

  const cancel = useCallback((id: string) => {
    pending.current.get(id)?.controller.abort();
  }, []);

  const dismiss = useCallback((id: string) => {
    setUploads((list) => list.filter((item) => item.id !== id));
  }, []);

  const busy = uploads.some((item) => item.error === null);
  return { uploads, busy, start, cancel, dismiss };
}
