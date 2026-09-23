import {
  EmailTheming,
  getEmailTheming,
  getMergedCssJs,
  getResolvedNodeStyles,
} from "@react-email/editor/plugins";
import { ServiceEmailFrame, type ServiceEmailFrameProps } from "./service-email-frame";

type SerializerPlugin = NonNullable<
  NonNullable<Parameters<typeof EmailTheming.configure>[0]>["serializerPlugin"]
>;

// composeReactEmail renders the serializer plugin's BaseTemplate around the
// editor content, so replacing it is how the fixed service frame wraps every
// Sendout. Node styles still come from the editor theme. `frame` is read at
// compose time because the extension is configured once per editor.
export function createServiceEmailSerializer(
  frame: () => ServiceEmailFrameProps
): SerializerPlugin {
  return {
    getNodeStyles(node, depth, editor) {
      const theming = getEmailTheming(editor);
      return getResolvedNodeStyles(node, depth, getMergedCssJs(theming.theme, theming.styles));
    },
    BaseTemplate({ children, editor }) {
      const theming = getEmailTheming(editor);
      const merged = getMergedCssJs(theming.theme, theming.styles);
      return (
        <ServiceEmailFrame {...frame()} bodyStyle={merged.body}>
          {children}
        </ServiceEmailFrame>
      );
    },
  };
}
