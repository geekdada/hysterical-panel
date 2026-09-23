import { useImperativeHandle, useMemo, useRef, useState, type Ref } from "react";
import { EditorProvider, useCurrentEditor } from "@tiptap/react";
import { Placeholder } from "@tiptap/extension-placeholder";
import { composeReactEmail, isDocumentVisuallyEmpty } from "@react-email/editor/core";
import { StarterKit } from "@react-email/editor/extensions";
import { EmailTheming } from "@react-email/editor/plugins";
import {
  BubbleMenu,
  BubbleMenuBold,
  BubbleMenuItalic,
  BubbleMenuItemGroup,
  BubbleMenuLinkSelector,
  BubbleMenuNodeSelector,
  BubbleMenuStrike,
  BubbleMenuUnderline,
  BULLET_LIST,
  BUTTON,
  DIVIDER,
  H1,
  H2,
  H3,
  NUMBERED_LIST,
  SlashCommand,
  TEXT,
} from "@react-email/editor/ui";
// Imported here, not in globals.css, so the editor stylesheet ships only
// with the composer chunk.
// oxlint-disable-next-line import/no-unassigned-import
import "@react-email/editor/themes/default.css";
import * as m from "~/paraglide/messages.js";
import { createServiceEmailSerializer } from "./service-email-serializer";
import type { ServiceEmailFrameProps } from "./service-email-frame";

export type ComposedSendout = { html: string; text: string; content: Record<string, unknown> };
export type SendoutEditorHandle = { compose: () => Promise<ComposedSendout> };

// Only the blocks agreed for v1 stay enabled; images, columns, code and
// tables are out of scope, and the Inspector sidebar is never mounted.
// Headings are limited to H1-H3 separately, in the StarterKit call.
const DISABLED_BLOCKS = {
  CodeBlockPrism: false,
  Code: false,
  TwoColumns: false,
  ThreeColumns: false,
  FourColumns: false,
  ColumnsColumn: false,
  Blockquote: false,
  Sup: false,
  Uppercase: false,
  Table: false,
  TableRow: false,
  TableCell: false,
  TableHeader: false,
  Section: false,
} as const;

// The node selector's Quote and Code entries point at disabled blocks.
const OMITTED_NODE_TYPES = ["Quote", "Code"];

// The library groups slash items by their English `category` and prints it as
// the group heading, so each item gets a localized category.
function slashCommands() {
  const textCategory = m.email_editor_cat_text();
  const layoutCategory = m.email_editor_cat_layout();
  return [
    {
      ...TEXT,
      category: textCategory,
      title: m.email_editor_cmd_text(),
      description: m.email_editor_cmd_text_desc(),
    },
    {
      ...H1,
      category: textCategory,
      title: m.email_editor_cmd_h1(),
      description: m.email_editor_cmd_h1_desc(),
    },
    {
      ...H2,
      category: textCategory,
      title: m.email_editor_cmd_h2(),
      description: m.email_editor_cmd_h2_desc(),
    },
    {
      ...H3,
      category: textCategory,
      title: m.email_editor_cmd_h3(),
      description: m.email_editor_cmd_h3_desc(),
    },
    {
      ...BULLET_LIST,
      category: textCategory,
      title: m.email_editor_cmd_bullets(),
      description: m.email_editor_cmd_bullets_desc(),
    },
    {
      ...NUMBERED_LIST,
      category: textCategory,
      title: m.email_editor_cmd_numbers(),
      description: m.email_editor_cmd_numbers_desc(),
    },
    {
      ...BUTTON,
      category: layoutCategory,
      title: m.email_editor_cmd_button(),
      description: m.email_editor_cmd_button_desc(),
    },
    {
      ...DIVIDER,
      category: layoutCategory,
      title: m.email_editor_cmd_divider(),
      description: m.email_editor_cmd_divider_desc(),
    },
  ];
}

export function SendoutEditor({
  frame,
  ref,
  onEmptyChange,
}: {
  frame: ServiceEmailFrameProps;
  ref?: Ref<SendoutEditorHandle>;
  onEmptyChange?: (empty: boolean) => void;
}) {
  const frameRef = useRef(frame);
  frameRef.current = frame;
  const extensions = useMemo(
    () => [
      StarterKit.configure({ ...DISABLED_BLOCKS, Heading: { levels: [1, 2, 3] } }),
      Placeholder.configure({
        placeholder: () => m.email_editor_placeholder(),
        includeChildren: true,
      }),
      EmailTheming.configure({
        theme: "basic",
        serializerPlugin: createServiceEmailSerializer(() => frameRef.current),
      }),
    ],
    []
  );
  const options = { locale: frame.language };

  return (
    <div className="sendout-canvas">
      <p className="sendout-canvas-brand">{frame.appName}</p>
      <EditorProvider
        extensions={extensions}
        immediatelyRender={false}
        onUpdate={({ editor }) => onEmptyChange?.(isDocumentVisuallyEmpty(editor.state.doc))}
      >
        <EditorBridge ref={ref} />
        <TextBubbleMenu />
        <BubbleMenu.LinkDefault />
        <BubbleMenu.ButtonDefault />
        <SlashCommand items={slashCommands()} />
      </EditorProvider>
      <p className="sendout-canvas-footer">
        {m.email_frame_notice({ app: frame.appName }, options)}
      </p>
    </div>
  );
}

function EditorBridge({ ref }: { ref?: Ref<SendoutEditorHandle> }) {
  const { editor } = useCurrentEditor();
  useImperativeHandle(
    ref,
    () => ({
      async compose() {
        if (!editor) throw new Error(m.email_editor_not_ready());
        const { unformattedHtml, text } = await composeReactEmail({ editor });
        return {
          html: unformattedHtml,
          text,
          content: editor.getJSON() as Record<string, unknown>,
        };
      },
    }),
    [editor]
  );
  return null;
}

function TextBubbleMenu() {
  const [nodeOpen, setNodeOpen] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  return (
    <BubbleMenu
      hideWhenActiveNodes={["button", "horizontalRule"]}
      hideWhenActiveMarks={["link"]}
      onHide={() => {
        setNodeOpen(false);
        setLinkOpen(false);
      }}
    >
      <BubbleMenuNodeSelector
        omit={OMITTED_NODE_TYPES}
        open={nodeOpen}
        onOpenChange={(open) => {
          setNodeOpen(open);
          if (open) setLinkOpen(false);
        }}
      />
      <BubbleMenuLinkSelector
        open={linkOpen}
        onOpenChange={(open) => {
          setLinkOpen(open);
          if (open) setNodeOpen(false);
        }}
      />
      <BubbleMenuItemGroup>
        <BubbleMenuBold />
        <BubbleMenuItalic />
        <BubbleMenuUnderline />
        <BubbleMenuStrike />
      </BubbleMenuItemGroup>
    </BubbleMenu>
  );
}
