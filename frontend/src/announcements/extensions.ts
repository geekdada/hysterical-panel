import { Extension, ResizableNodeView, type Extensions } from "@tiptap/react";
import { StarterKit } from "@tiptap/starter-kit";
import { Image } from "@tiptap/extension-image";
import { Placeholder } from "@tiptap/extension-placeholder";
import { isAllowedLinkTarget, panelFileUrl, storedFilePath } from "~/lib/panel-files";
import * as m from "~/paraglide/messages.js";

type OriginSource = () => string;

// The backend keeps a resized width only as a whole number of pixels below 10000.
const IMAGE_WIDTH_PATTERN = /^[1-9]\d{0,3}$/;
const MIN_IMAGE_WIDTH = 48;

function parseImageWidth(value: string | null): number | null {
  return value !== null && IMAGE_WIDTH_PATTERN.test(value) ? Number(value) : null;
}

// The document keeps the relative path the backend accepts. Only the canvas
// shows the absolute URL, because the panel API may live on another origin.
function panelImage(origin: OriginSource) {
  return Image.extend({
    // Only files stored by the panel survive the backend sanitizer. The src
    // attribute's own parser runs after the rule's getAttrs and wins, so the
    // relative path is computed there.
    addAttributes() {
      return {
        src: {
          default: null,
          parseHTML: (element) => storedFilePath(element.getAttribute("src") ?? "", origin()),
        },
        alt: { default: null },
        title: { default: null },
        width: {
          default: null,
          parseHTML: (element) => parseImageWidth(element.getAttribute("width")),
        },
      };
    },
    parseHTML() {
      return [
        {
          tag: "img[src]",
          getAttrs: (element) =>
            storedFilePath(element.getAttribute("src") ?? "", origin()) === null ? false : null,
        },
      ];
    },
    // The markdown shortcut would insert external images the backend rejects.
    addInputRules() {
      return [];
    },
    addNodeView() {
      return ({ node, getPos, editor }) => {
        const img = document.createElement("img");
        const sync = (attrs: Record<string, unknown>) => {
          const src = panelFileUrl(String(attrs.src ?? ""), origin());
          if (img.getAttribute("src") !== src) img.src = src;
          img.alt = String(attrs.alt ?? "");
          img.style.width = attrs.width ? `${attrs.width}px` : "";
          img.style.height = "";
        };
        sync(node.attrs);

        return new ResizableNodeView({
          element: img,
          editor,
          node,
          getPos,
          // Only the width is stored. The height follows it through CSS, so a
          // narrow screen scales the image down without stretching it.
          onResize: (width) => {
            img.style.width = `${width}px`;
            img.style.height = "";
          },
          onCommit: (width) => {
            const pos = getPos();
            const rounded = Math.round(width);
            if (pos === undefined || editor.state.doc.nodeAt(pos)?.attrs.width === rounded) return;
            editor
              .chain()
              .setNodeSelection(pos)
              .updateAttributes(this.name, { width: rounded })
              .run();
          },
          onUpdate: (updated) => {
            if (updated.type !== node.type) return false;
            sync(updated.attrs);
            return true;
          },
          options: {
            // Images sit on the left edge, so only the right corners resize.
            directions: ["top-right", "bottom-right"],
            min: { width: MIN_IMAGE_WIDTH },
            preserveAspectRatio: true,
          },
        });
      };
    },
  });
}

// Attachment links carry the original file name so the browser saves the
// file under it instead of the stored name.
const LinkDownload = Extension.create({
  name: "linkDownload",
  addGlobalAttributes() {
    return [
      {
        types: ["link"],
        attributes: {
          download: {
            default: null,
            parseHTML: (element) => element.getAttribute("download"),
            renderHTML: (attributes) =>
              attributes.download == null ? {} : { download: attributes.download },
          },
        },
      },
    ];
  },
});

export function announcementExtensions(origin: OriginSource): Extensions {
  return [
    StarterKit.configure({
      heading: { levels: [2, 3] },
      underline: false,
      link: {
        openOnClick: false,
        autolink: true,
        defaultProtocol: "https",
        // The backend adds target and rel to external links when it saves.
        HTMLAttributes: { target: null, rel: null },
        isAllowedUri: (url) => isAllowedLinkTarget(url),
      },
    }),
    panelImage(origin),
    LinkDownload,
    Placeholder.configure({ placeholder: () => m.announcement_editor_placeholder() }),
  ];
}
