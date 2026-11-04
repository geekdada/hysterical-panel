import { Fragment, type CSSProperties, type ReactNode } from "react";
import { Body, Container, Head, Hr, Html, Link, Section, Text } from "react-email";
import * as m from "~/paraglide/messages.js";

export type SendoutLanguage = "en" | "zh-cn";

// Automatic emails go to Users of either language, so their frame shows both.
export type FrameLanguage = SendoutLanguage | "bilingual";

export type ServiceEmailFrameProps = {
  language: FrameLanguage;
  appName: string;
  frontendUrl: string;
  showPanelLink: boolean;
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
  showPanelLink,
  bodyStyle,
  children,
}: ServiceEmailFrameProps & { bodyStyle?: CSSProperties; children: ReactNode }) {
  const locales: SendoutLanguage[] = language === "bilingual" ? ["zh-cn", "en"] : [language];
  // react-email's Body sets lang="en" on body unless given one.
  const lang = locales[0] === "en" ? "en" : "zh-CN";
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
          {locales.map((locale) => (
            <Fragment key={locale}>
              <Text style={styles.footer}>
                {m.email_frame_notice({ app: appName }, { locale })}
              </Text>
              {showPanelLink && frontendUrl ? (
                <Link href={frontendUrl} style={styles.link}>
                  {m.email_frame_open_panel({}, { locale })}
                </Link>
              ) : null}
            </Fragment>
          ))}
        </Container>
      </Body>
    </Html>
  );
}
