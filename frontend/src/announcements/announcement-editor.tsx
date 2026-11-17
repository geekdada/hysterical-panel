import { useEffect, useMemo, useRef } from "react";
import { EditorContent, useEditor, type Editor, type JSONContent } from "@tiptap/react";
import { Button, ProgressBar } from "@heroui/react";
import { Xmark } from "@gravity-ui/icons";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";
import * as m from "~/paraglide/messages.js";
import { EditorToolbar } from "./editor-toolbar";
import { announcementExtensions } from "./extensions";
import { useEditorUploads, type EditorUpload } from "./use-editor-uploads";

export type AnnouncementDocument = {
  html: string;
  content: Record<string, unknown>;
  empty: boolean;
};

// Matches the backend rule: a body needs some text or an image.
function readDocument(editor: Editor): AnnouncementDocument {
  let hasImage = false;
  editor.state.doc.descendants((node) => {
    if (node.type.name === "image") hasImage = true;
    return !hasImage;
  });
  return {
    html: editor.getHTML(),
    content: editor.getJSON() as Record<string, unknown>,
    empty: !hasImage && editor.state.doc.textContent.trim() === "",
  };
}

export function AnnouncementEditor({
  initialContent,
  onChange,
  onBusyChange,
}: {
  initialContent?: Record<string, unknown>;
  onChange: (document: AnnouncementDocument) => void;
  onBusyChange: (busy: boolean) => void;
}) {
  const origin = usePanelApiOrigin();
  const originRef = useRef(origin);
  originRef.current = origin;
  const startRef = useRef<(files: File[], position: number) => void>(() => undefined);

  const extensions = useMemo(() => announcementExtensions(() => originRef.current), []);
  const editor = useEditor({
    extensions,
    content: (initialContent as JSONContent | undefined) ?? "",
    immediatelyRender: false,
    onCreate: (event) => onChange(readDocument(event.editor)),
    onUpdate: (event) => onChange(readDocument(event.editor)),
    editorProps: {
      attributes: { class: "announcement-body announcement-canvas" },
      handlePaste: (view, event) => {
        const files = Array.from(event.clipboardData?.files ?? []);
        if (files.length === 0) return false;
        startRef.current(files, view.state.selection.to);
        return true;
      },
      handleDrop: (view, event, _slice, moved) => {
        const files = Array.from(event.dataTransfer?.files ?? []);
        if (moved || files.length === 0) return false;
        const at = view.posAtCoords({ left: event.clientX, top: event.clientY });
        startRef.current(files, at?.pos ?? view.state.selection.to);
        event.preventDefault();
        return true;
      },
    },
  });

  const { uploads, busy, start, cancel, dismiss } = useEditorUploads(editor);
  startRef.current = start;

  // The callback comes from a form field and changes identity on every render.
  const onBusyChangeRef = useRef(onBusyChange);
  onBusyChangeRef.current = onBusyChange;
  useEffect(() => {
    onBusyChangeRef.current(busy);
  }, [busy]);

  return (
    <div className="overflow-hidden rounded-lg border bg-field">
      {editor ? (
        <EditorToolbar
          editor={editor}
          onPickFiles={(files) => start(files, editor.state.selection.to)}
        />
      ) : (
        <div className="h-[41px] border-b" />
      )}
      <EditorContent editor={editor} />
      {uploads.length > 0 ? (
        <ul className="flex flex-col gap-2 border-t px-4 py-3">
          {uploads.map((upload) => (
            <UploadRow key={upload.id} upload={upload} onCancel={cancel} onDismiss={dismiss} />
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function UploadRow({
  upload,
  onCancel,
  onDismiss,
}: {
  upload: EditorUpload;
  onCancel: (id: string) => void;
  onDismiss: (id: string) => void;
}) {
  const failed = upload.error !== null;
  return (
    <li className="flex items-center gap-3 text-[13px]">
      <div className="min-w-0 flex-1">
        <p className="truncate text-foreground">{upload.name}</p>
        {failed ? (
          <p className="text-danger">{upload.error}</p>
        ) : (
          <ProgressBar
            aria-label={m.announcement_upload_progress({ name: upload.name })}
            value={Math.round(upload.progress * 100)}
            size="sm"
            className="mt-1.5"
          >
            <ProgressBar.Track>
              <ProgressBar.Fill />
            </ProgressBar.Track>
          </ProgressBar>
        )}
      </div>
      <Button
        variant="ghost"
        size="sm"
        isIconOnly
        aria-label={failed ? m.announcement_upload_dismiss() : m.announcement_upload_cancel()}
        onPress={() => (failed ? onDismiss(upload.id) : onCancel(upload.id))}
      >
        <Xmark className="size-4" />
      </Button>
    </li>
  );
}
