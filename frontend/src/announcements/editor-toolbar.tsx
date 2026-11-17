import { useRef, useState, type ComponentType, type ReactNode, type SVGProps } from "react";
import { useEditorState, type Editor } from "@tiptap/react";
import {
  Button,
  Input,
  Label,
  Modal,
  Separator,
  TextField,
  ToggleButton,
  Toolbar,
  Tooltip,
} from "@heroui/react";
import {
  Bold,
  Code,
  Heading2,
  Heading3,
  Italic,
  Link,
  ListOl,
  ListUl,
  Minus,
  Paperclip,
  QuoteOpen,
  SquareBracketsBarsVertical,
  Strikethrough,
} from "@gravity-ui/icons";
import { isAllowedLinkTarget } from "~/lib/panel-files";
import * as m from "~/paraglide/messages.js";

type Icon = ComponentType<SVGProps<SVGSVGElement>>;

function activeMarks(editor: Editor) {
  return {
    h2: editor.isActive("heading", { level: 2 }),
    h3: editor.isActive("heading", { level: 3 }),
    bold: editor.isActive("bold"),
    italic: editor.isActive("italic"),
    strike: editor.isActive("strike"),
    code: editor.isActive("code"),
    bulletList: editor.isActive("bulletList"),
    orderedList: editor.isActive("orderedList"),
    blockquote: editor.isActive("blockquote"),
    codeBlock: editor.isActive("codeBlock"),
    link: editor.isActive("link"),
  };
}

export function EditorToolbar({
  editor,
  onPickFiles,
}: {
  editor: Editor;
  onPickFiles: (files: File[]) => void;
}) {
  const active = useEditorState({ editor, selector: (state) => activeMarks(state.editor) });
  const fileInput = useRef<HTMLInputElement>(null);
  const [linkOpen, setLinkOpen] = useState(false);
  const chain = () => editor.chain().focus();

  return (
    <>
      <Toolbar
        aria-label={m.announcement_editor_toolbar()}
        className="flex w-full flex-wrap items-center gap-0.5 rounded-none border-b px-2 py-1.5"
      >
        <Tool
          icon={Heading2}
          label={m.announcement_editor_h2()}
          selected={active.h2}
          onToggle={() => chain().toggleHeading({ level: 2 }).run()}
        />
        <Tool
          icon={Heading3}
          label={m.announcement_editor_h3()}
          selected={active.h3}
          onToggle={() => chain().toggleHeading({ level: 3 }).run()}
        />
        <ToolSeparator />
        <Tool
          icon={Bold}
          label={m.announcement_editor_bold()}
          selected={active.bold}
          onToggle={() => chain().toggleBold().run()}
        />
        <Tool
          icon={Italic}
          label={m.announcement_editor_italic()}
          selected={active.italic}
          onToggle={() => chain().toggleItalic().run()}
        />
        <Tool
          icon={Strikethrough}
          label={m.announcement_editor_strike()}
          selected={active.strike}
          onToggle={() => chain().toggleStrike().run()}
        />
        <Tool
          icon={Code}
          label={m.announcement_editor_code()}
          selected={active.code}
          onToggle={() => chain().toggleCode().run()}
        />
        <Tool
          icon={Link}
          label={m.announcement_editor_link()}
          selected={active.link}
          onToggle={() => setLinkOpen(true)}
        />
        <ToolSeparator />
        <Tool
          icon={ListUl}
          label={m.announcement_editor_bullets()}
          selected={active.bulletList}
          onToggle={() => chain().toggleBulletList().run()}
        />
        <Tool
          icon={ListOl}
          label={m.announcement_editor_numbers()}
          selected={active.orderedList}
          onToggle={() => chain().toggleOrderedList().run()}
        />
        <Tool
          icon={QuoteOpen}
          label={m.announcement_editor_quote()}
          selected={active.blockquote}
          onToggle={() => chain().toggleBlockquote().run()}
        />
        <Tool
          icon={SquareBracketsBarsVertical}
          label={m.announcement_editor_code_block()}
          selected={active.codeBlock}
          onToggle={() => chain().toggleCodeBlock().run()}
        />
        <Tool
          icon={Minus}
          label={m.announcement_editor_divider()}
          selected={false}
          onToggle={() => chain().setHorizontalRule().run()}
        />
        <ToolSeparator />
        <Tool
          icon={Paperclip}
          label={m.announcement_editor_upload()}
          selected={false}
          onToggle={() => fileInput.current?.click()}
        />
      </Toolbar>
      <input
        ref={fileInput}
        type="file"
        multiple
        hidden
        onChange={(event) => {
          onPickFiles(Array.from(event.target.files ?? []));
          event.target.value = "";
        }}
      />
      <LinkModal editor={editor} isOpen={linkOpen} onOpenChange={setLinkOpen} />
    </>
  );
}

// Commands like the divider are actions, so the toggle never stays pressed.
// A pointer press must leave focus in the editor. Otherwise the button takes
// focus after the command runs and the caret lands outside the new block.
function Tool({
  icon: IconComponent,
  label,
  selected,
  onToggle,
}: {
  icon: Icon;
  label: string;
  selected: boolean;
  onToggle: () => void;
}) {
  return (
    <Tooltip delay={600} closeDelay={0}>
      <ToggleButton
        variant="ghost"
        size="sm"
        isIconOnly
        aria-label={label}
        isSelected={selected}
        preventFocusOnPress
        onChange={onToggle}
      >
        <IconComponent className="size-4" />
      </ToggleButton>
      <Tooltip.Content>{label}</Tooltip.Content>
    </Tooltip>
  );
}

function ToolSeparator(): ReactNode {
  return <Separator orientation="vertical" className="mx-1 h-5 bg-separator" />;
}

function LinkModal({
  editor,
  isOpen,
  onOpenChange,
}: {
  editor: Editor;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          {isOpen ? <LinkForm editor={editor} onDone={() => onOpenChange(false)} /> : null}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function LinkForm({ editor, onDone }: { editor: Editor; onDone: () => void }) {
  const current = String(editor.getAttributes("link").href ?? "");
  const [href, setHref] = useState(current);
  const trimmed = href.trim();
  const valid = trimmed === "" || isAllowedLinkTarget(trimmed);

  function apply() {
    const chain = editor.chain().focus().extendMarkRange("link");
    if (trimmed === "") chain.unsetLink().run();
    else if (editor.state.selection.empty && !current) {
      // Nothing is selected to wrap, so the address becomes the link text.
      chain
        .insertContent({
          type: "text",
          text: trimmed,
          marks: [{ type: "link", attrs: { href: trimmed } }],
        })
        .run();
    } else chain.setLink({ href: trimmed }).run();
    onDone();
  }

  return (
    <>
      <Modal.Header>
        <Modal.Heading>{m.announcement_editor_link()}</Modal.Heading>
      </Modal.Header>
      <Modal.Body>
        <TextField
          value={href}
          onChange={setHref}
          isInvalid={!valid}
          autoFocus
          onKeyDown={(event) => {
            if (event.key === "Enter" && valid) apply();
          }}
        >
          <Label>{m.announcement_editor_link_url()}</Label>
          <Input autoComplete="off" placeholder="https://" />
        </TextField>
        <p className="mt-2 text-xs text-muted">{m.announcement_editor_link_hint()}</p>
      </Modal.Body>
      <Modal.Footer>
        {current ? (
          <Button
            variant="ghost"
            onPress={() => {
              editor.chain().focus().extendMarkRange("link").unsetLink().run();
              onDone();
            }}
          >
            {m.announcement_editor_link_remove()}
          </Button>
        ) : null}
        <Button variant="primary" isDisabled={!valid} onPress={apply}>
          {m.announcement_editor_link_apply()}
        </Button>
      </Modal.Footer>
    </>
  );
}
