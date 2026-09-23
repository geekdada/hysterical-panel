import type { CSSProperties, ReactNode } from "react";
import { Body, Container, Head, Hr, Html, Link, Section, Text } from "react-email";
import * as m from "~/paraglide/messages.js";

export type SendoutLanguage = "en" | "zh-cn";

export type ServiceEmailFrameProps = {
  language: SendoutLanguage;
  appName: string;
  frontendUrl: string;
};

// Email clients drop stylesheets and ignore the panel theme, so the frame
// uses inline styles with literal colors.
const styles = {
  body: { margin: 0, padding: "24px 0", backgroundColor: "#f4f4f5" },
  container: {
    maxWidth: "560px",
    margin: "0 auto",
    padding: "32px",
    backgroundColor: "#ffffff",
    borderRadius: "8px",
  },
  brand: { margin: "0 0 24px", fontSize: "14px", fontWeight: 600, color: "#18181b" },
  rule: { margin: "32px 0 16px", borderColor: "#e4e4e7" },
  footer: { margin: "0 0 4px", fontSize: "12px", lineHeight: "18px", color: "#71717a" },
  link: { fontSize: "12px", color: "#3f3f46", textDecoration: "underline" },
} satisfies Record<string, CSSProperties>;

export function ServiceEmailFrame({
  language,
  appName,
  frontendUrl,
  bodyStyle,
  children,
}: ServiceEmailFrameProps & { bodyStyle?: CSSProperties; children: ReactNode }) {
  const options = { locale: language };
  // react-email's Body sets lang="en" on body unless given one.
  const lang = language === "zh-cn" ? "zh-CN" : "en";
  return (
    <Html lang={lang}>
      <Head>
        <meta content="width=device-width" name="viewport" />
      </Head>
      <Body lang={lang} style={{ ...bodyStyle, ...styles.body }}>
        <Container style={styles.container}>
          <Text style={styles.brand}>{appName}</Text>
          <Section>{children}</Section>
          <Hr style={styles.rule} />
          <Text style={styles.footer}>{m.email_frame_notice({ app: appName }, options)}</Text>
          {frontendUrl ? (
            <Link href={frontendUrl} style={styles.link}>
              {m.email_frame_open_panel({}, options)}
            </Link>
          ) : null}
        </Container>
      </Body>
    </Html>
  );
}
