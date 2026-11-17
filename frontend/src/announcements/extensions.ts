import { Extension, type Extensions } from "@tiptap/react";
import { StarterKit } from "@tiptap/starter-kit";
import { Image } from "@tiptap/extension-image";
import { Placeholder } from "@tiptap/extension-placeholder";
import { isAllowedLinkTarget, panelFileUrl, storedFilePath } from "~/lib/panel-files";
import * as m from "~/paraglide/messages.js";

type OriginSource = () => string;

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
      return ({ node }) => {
        const img = document.createElement("img");
        const sync = (attrs: Record<string, unknown>) => {
          img.src = panelFileUrl(String(attrs.src ?? ""), origin());
          img.alt = String(attrs.alt ?? "");
        };
        sync(node.attrs);
        return {
          dom: img,
          update: (updated) => {
            if (updated.type !== node.type) return false;
            sync(updated.attrs);
            return true;
          },
        };
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
