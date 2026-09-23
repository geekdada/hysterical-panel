# Email Sendouts Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admins write an Email Sendout in `@react-email/editor` under Settings, review the exact email, send it to one or every active and Verified User, and then follow delivery, cancel, and resend failed recipients.

**Architecture:**
- The editor lives in `frontend/src/emails/`. A custom `serializerPlugin.BaseTemplate` wraps the editor content in the fixed service frame (en or zh-cn), so the browser's `composeReactEmail` output is the final email.
- Three routes under `/settings/emails` use the typed API client through `src/api/queries.ts`.
- The editor loads client-only (lazy import behind `useMounted`), so SSR never renders it.

**Tech Stack:** TanStack Start / Router (React 19, SSR), HeroUI v3, TanStack Query, `@react-email/editor` 1.7.8 (TipTap 3), `react-email` 6.9.5, Paraglide (en / zh-cn).

**Spec:** `docs/superpowers/specs/2026-09-22-email-sendouts-design.md`, plus `docs/adr/0008-browser-composed-email-sendouts.md` and the backend contract in `backend/README.md` (section `### Email Sendout`). The backend on branch `feat/email-sendouts` is complete.

## Global Constraints

- Work on branch `feat/email-sendouts`. Run frontend commands from `frontend/` with **pnpm**.
- The frontend has no unit test runner. Each task's gate is `pnpm typecheck`, `pnpm i18n:check` and `pnpm build`; the plan names the tasks that also need a browser check. Do not add a test framework.
- Every visible string comes from `~/paraglide/messages.js`. Every new key goes into both `messages/en.json` and `messages/zh-cn.json` with non-empty values. UI copy uses no em dashes (`PRODUCT.md`).
- Never hand-edit `src/api/schema.d.ts` or `src/paraglide/`. Regenerate them with `pnpm api:sync` and `pnpm build`. `src/routeTree.gen.ts` is generated and committed: after adding routes, run `pnpm build` (or `pnpm dev`) and commit the regenerated file.
- Components call only functions and query keys from `src/api/queries.ts`, never `client.ts` directly.
- Pin these dependencies exactly: `@react-email/editor@1.7.8`, `react-email@6.9.5`. `@tiptap/react` and `@tiptap/extension-placeholder` must resolve to the same single version the editor uses (`^3.17.1`).
- Editor scope:
  - Enabled: paragraph, H1–H3, bold, italic, underline, strike, link, bullet and numbered lists, divider, button.
  - Removed: images, columns, code, code block, table, quote, sup, uppercase, section, and the Inspector sidebar.
- The editor canvas always renders light, as the email looks. The editor's floating menus follow the panel theme through `--re-*` variables mapped to panel tokens.
- The sendout language is `en` or `zh-cn` and defaults to the admin's current locale.
- Stored or composed email HTML is shown only in `<iframe sandbox="" srcDoc=...>`, never through `dangerouslySetInnerHTML`.
- The "Send" button is disabled while the create request is in flight (double-submit guard, ruling from the backend review).
- All admin pages use `beforeLoad: ({ context }) => requireAdmin(context.auth)` and call `markResponsePrivate()` in their loader.
- API datetimes are UTC PocketBase strings. Display them with `relTimeFromISO` or `formatLocaleDateTime(Date.parse(iso), undefined, tz)`, with `tz` from `useActiveTimeZone()`.

## Review Focus

1. **Double click on "Send".** A second click must not queue a second broadcast. The Task 4 browser check clicks Send twice quickly and expects exactly one new Sendout.
2. **Preview must equal the email that is sent.** The review modal composes once and sends those exact `html`/`text` bytes. Task 4 edits the text after opening the review, then confirms that the stored Sendout (Task 5 detail preview) shows the reviewed version.
3. **Hard refresh of the composer (SSR).** Loading `/settings/emails/new` directly must render without a server error or hydration warning, because the editor is client-only. Covered in Task 4's browser check.
4. **Panel theme differs from the OS theme.** With the panel set to Light on an OS in dark mode, the bubble and slash menus must render light. Covered in Task 2's CSS and Task 4's browser check.
5. **Stored HTML containing script or event-handler markup.** It must not execute inside the panel. The sandboxed iframe (no `allow-scripts`) covers this; Task 5 checks the `sandbox=""` attribute.

---

### Task 1: Dependencies, generated types, and the API layer

**Files:**
- Modify: `frontend/package.json`, `frontend/pnpm-lock.yaml` (via pnpm)
- Modify: `frontend/src/api/queries.ts`
- Modify: `frontend/src/lib/api-error.ts`
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes: the backend OpenAPI operations `listEmailSendouts`, `createEmailSendout`, `getEmailSendoutContext`, `listEmailSendoutCandidates`, `getEmailSendout`, `listEmailSendoutRecipients`, `cancelEmailSendout`, `resendEmailSendout`, and `SettingsResponse.email_sendout_rate_per_minute`.
- Produces (exported from `~/api/queries`):
  - types `EmailSendout`, `EmailSendoutDetail`, `EmailSendoutRecipient`, `EmailSendoutCandidate`, `EmailSendoutContext`, `EmailSendoutCreateRequest`, `RecipientStatus`
  - `queryKeys.emailSendouts()`, `queryKeys.emailSendoutContext()`, `queryKeys.emailSendoutCandidates(search: string)`, `queryKeys.emailSendout(id: string)`, `queryKeys.emailSendoutRecipients(id: string, status: RecipientStatus | "")`
  - `fetchEmailSendoutContext(): Promise<EmailSendoutContext>`
  - `fetchEmailSendoutCandidates(search: string): Promise<EmailSendoutCandidate[]>`
  - `fetchEmailSendouts(): Promise<EmailSendout[]>`
  - `fetchEmailSendout(id: string): Promise<EmailSendoutDetail>`
  - `fetchEmailSendoutRecipients(id: string, status: RecipientStatus | ""): Promise<EmailSendoutRecipient[]>`
  - `createEmailSendout(body: EmailSendoutCreateRequest): Promise<EmailSendout>`
  - `cancelEmailSendout(id: string): Promise<EmailSendout>`
  - `resendEmailSendout(id: string, recipientIds: string[]): Promise<{ requeued?: number }>`

- [ ] **Step 1: Install the editor dependencies**

```bash
cd frontend
pnpm add @react-email/editor@1.7.8 react-email@6.9.5 @tiptap/react@^3.17.1 @tiptap/extension-placeholder@^3.17.1
pnpm why @tiptap/react
```
Expected: `pnpm why` lists exactly one `@tiptap/react` version, shared by the app and `@react-email/editor`. If it lists two, align the direct dependency to the version the editor resolved (`pnpm add @tiptap/react@<that version> @tiptap/extension-placeholder@<that version>`) and run it again. Two copies break `useCurrentEditor` context.

- [ ] **Step 2: Regenerate the API types**

```bash
pnpm api:sync
grep -c "email-sendouts" src/api/schema.d.ts
```
Expected: a non-zero count. `schema.d.ts` is gitignored and is not committed.

- [ ] **Step 3: Add the query layer**

In `frontend/src/api/queries.ts`:

1. Next to the other `type X = components["schemas"][...]` lines, add:

```ts
type EmailSendout = components["schemas"]["EmailSendout"];
type EmailSendoutDetail = components["schemas"]["EmailSendoutDetail"];
type EmailSendoutRecipient = components["schemas"]["EmailSendoutRecipient"];
type EmailSendoutCandidate = components["schemas"]["EmailSendoutRecipientCandidate"];
type EmailSendoutContext = components["schemas"]["EmailSendoutContextResponse"];
type EmailSendoutCreateRequest = components["schemas"]["EmailSendoutCreateRequest"];
type EmailSendoutResendResponse = components["schemas"]["EmailSendoutResendResponse"];
type RecipientStatus = NonNullable<EmailSendoutRecipient["status"]>;
```

2. Add all eight names to the existing `export type { ... }` block.

3. Add these entries to the `queryKeys` object, after `notificationChannels`:

```ts
  emailSendouts: () => [...queryKeys.all, "email-sendouts"] as const,
  emailSendoutContext: () => [...queryKeys.emailSendouts(), "context"] as const,
  emailSendoutCandidates: (search: string) =>
    [...queryKeys.emailSendouts(), "candidates", search] as const,
  emailSendout: (id: string) => [...queryKeys.emailSendouts(), id] as const,
  emailSendoutRecipients: (id: string, status: RecipientStatus | "") =>
    [...queryKeys.emailSendouts(), id, "recipients", status] as const,
```

4. After `revealNotificationChannelURL`, add:

```ts
export function fetchEmailSendoutContext(): Promise<EmailSendoutContext> {
  return apiRequest<EmailSendoutContext>(
    apiClient.GET("/api/panel/email-sendouts/context"),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendoutCandidates(search: string): Promise<EmailSendoutCandidate[]> {
  return apiRequest<EmailSendoutCandidate[]>(
    apiClient.GET("/api/panel/email-sendouts/eligible-recipients", {
      params: { query: { search } },
    }),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendouts(): Promise<EmailSendout[]> {
  return apiRequest<EmailSendout[]>(
    apiClient.GET("/api/panel/email-sendouts"),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendout(id: string): Promise<EmailSendoutDetail> {
  return apiRequest<EmailSendoutDetail>(
    apiClient.GET("/api/panel/email-sendouts/{id}", { params: { path: { id } } }),
    m.error_email_sendouts_load()
  );
}

export function fetchEmailSendoutRecipients(
  id: string,
  status: RecipientStatus | ""
): Promise<EmailSendoutRecipient[]> {
  return apiRequest<EmailSendoutRecipient[]>(
    apiClient.GET("/api/panel/email-sendouts/{id}/recipients", {
      params: { path: { id }, query: status ? { status } : {} },
    }),
    m.error_email_sendouts_load()
  );
}

export function createEmailSendout(body: EmailSendoutCreateRequest): Promise<EmailSendout> {
  return apiRequest<EmailSendout>(
    apiClient.POST("/api/panel/email-sendouts", { body }),
    m.error_email_sendout_create(),
    m.error_email_sendout_create_network()
  );
}

export function cancelEmailSendout(id: string): Promise<EmailSendout> {
  return apiRequest<EmailSendout>(
    apiClient.POST("/api/panel/email-sendouts/{id}/cancel", { params: { path: { id } } }),
    m.error_email_sendout_cancel()
  );
}

export function resendEmailSendout(
  id: string,
  recipientIds: string[]
): Promise<EmailSendoutResendResponse> {
  return apiRequest<EmailSendoutResendResponse>(
    apiClient.POST("/api/panel/email-sendouts/{id}/resend", {
      params: { path: { id } },
      body: { recipient_ids: recipientIds },
    }),
    m.error_email_sendout_resend()
  );
}
```

If `openapi-fetch` types reject `query: {}` for the optional `status` parameter, use `query: status ? { status } : undefined`.

- [ ] **Step 4: Localize the new backend error messages**

In `frontend/src/lib/api-error.ts`, add these entries to `API_ERROR_MAP`. The keys are the backend messages, lowercased.

```ts
  "email sendouts are unavailable: smtp is not configured": () => m.error_email_smtp_unavailable(),
  "no users are active and verified": () => m.error_email_no_recipients(),
  "recipient must be active and verified": () => m.error_email_recipient_ineligible(),
  "recipient not found": () => m.error_email_recipient_ineligible(),
  "email sendout is cancelled": () => m.error_email_sendout_cancelled(),
  "email sendout has no pending recipients": () => m.error_email_nothing_to_cancel(),
  "subject must be between 1 and 200 characters": () => m.error_email_subject_invalid(),
  "subject cannot contain control characters": () => m.error_email_subject_invalid(),
  "email content is too large": () => m.error_email_content_too_large(),
  "content is too large": () => m.error_email_content_too_large(),
  "email_sendout_rate_per_minute must be between 1 and 600": () => m.error_email_rate_invalid(),
```

- [ ] **Step 5: Add the message keys**

Add to `frontend/messages/en.json`:

```json
  "error_email_sendouts_load": "Could not load email sendouts.",
  "error_email_sendout_create": "Could not queue the email.",
  "error_email_sendout_create_network": "Could not reach the server to queue the email.",
  "error_email_sendout_cancel": "Could not cancel the sendout.",
  "error_email_sendout_resend": "Could not requeue the failed recipients.",
  "error_email_smtp_unavailable": "Email is unavailable because SMTP is not configured in PocketBase.",
  "error_email_no_recipients": "No users are active and verified.",
  "error_email_recipient_ineligible": "The recipient must be an active, verified user.",
  "error_email_sendout_cancelled": "This sendout is cancelled.",
  "error_email_nothing_to_cancel": "This sendout has no pending recipients left.",
  "error_email_subject_invalid": "The subject must be 1 to 200 characters on a single line.",
  "error_email_content_too_large": "The email is too large to send.",
  "error_email_rate_invalid": "The rate must be between 1 and 600 emails per minute."
```

Add to `frontend/messages/zh-cn.json`:

```json
  "error_email_sendouts_load": "无法加载邮件发送记录。",
  "error_email_sendout_create": "邮件未能加入发送队列。",
  "error_email_sendout_create_network": "无法连接服务器，邮件未能加入发送队列。",
  "error_email_sendout_cancel": "无法取消这次发送。",
  "error_email_sendout_resend": "无法重新发送失败的收件人。",
  "error_email_smtp_unavailable": "PocketBase 未配置 SMTP，暂时无法发送邮件。",
  "error_email_no_recipients": "没有启用且已验证的用户。",
  "error_email_recipient_ineligible": "收件人必须是启用且已验证的用户。",
  "error_email_sendout_cancelled": "这次发送已取消。",
  "error_email_nothing_to_cancel": "这次发送已没有待发送的收件人。",
  "error_email_subject_invalid": "主题需为 1 到 200 个字符，且不能换行。",
  "error_email_content_too_large": "邮件内容过大，无法发送。",
  "error_email_rate_invalid": "发送速率需在每分钟 1 到 600 封之间。"
```

Keep both files valid JSON: add a comma after the previous last entry.

- [ ] **Step 6: Verify and commit**

```bash
pnpm build >/dev/null && pnpm typecheck && pnpm i18n:check && pnpm lint && pnpm format:check
```
Expected: all pass. `pnpm build` runs first so that `src/paraglide/` contains the new keys. If `format:check` fails on the files you touched, run `pnpm format` and check again.

```bash
git add package.json pnpm-lock.yaml src/api/queries.ts src/lib/api-error.ts messages/en.json messages/zh-cn.json
git commit -m "feat(frontend): add email sendout API layer and editor dependencies"
```

---

### Task 2: Service email frame and the editor component

**Files:**
- Create: `frontend/src/emails/service-email-frame.tsx`
- Create: `frontend/src/emails/service-email-serializer.tsx`
- Create: `frontend/src/emails/sendout-editor.tsx`
- Modify: `frontend/src/styles/globals.css`
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes: none from Task 1 beyond the installed packages.
- Produces:
  - `type SendoutLanguage = "en" | "zh-cn"`
  - `type ServiceEmailFrameProps = { language: SendoutLanguage; appName: string; frontendUrl: string }`
  - `ServiceEmailFrame` React Email component
  - `createServiceEmailSerializer(frame: () => ServiceEmailFrameProps)`
  - `SendoutEditor` component, props `{ frame: ServiceEmailFrameProps; ref?: Ref<SendoutEditorHandle>; onEmptyChange?: (empty: boolean) => void }`
  - `type SendoutEditorHandle = { compose: () => Promise<ComposedSendout> }`
  - `type ComposedSendout = { html: string; text: string; content: Record<string, unknown> }`

- [ ] **Step 1: Add the message keys**

Add to `messages/en.json`:

```json
  "email_frame_notice": "This is a service message about your {app} account. It cannot be unsubscribed from.",
  "email_frame_open_panel": "Open the panel",
  "email_editor_placeholder": "Write the message, or press / for blocks",
  "email_editor_cmd_text": "Text",
  "email_editor_cmd_text_desc": "Plain paragraph",
  "email_editor_cmd_h1": "Heading 1",
  "email_editor_cmd_h1_desc": "Largest heading",
  "email_editor_cmd_h2": "Heading 2",
  "email_editor_cmd_h2_desc": "Section heading",
  "email_editor_cmd_h3": "Heading 3",
  "email_editor_cmd_h3_desc": "Small heading",
  "email_editor_cmd_bullets": "Bulleted list",
  "email_editor_cmd_bullets_desc": "List with bullets",
  "email_editor_cmd_numbers": "Numbered list",
  "email_editor_cmd_numbers_desc": "List with numbers",
  "email_editor_cmd_button": "Button",
  "email_editor_cmd_button_desc": "Link styled as a button",
  "email_editor_cmd_divider": "Divider",
  "email_editor_cmd_divider_desc": "Horizontal rule",
  "email_editor_not_ready": "The editor is still loading."
```

Add to `messages/zh-cn.json`:

```json
  "email_frame_notice": "这是一封与你的 {app} 账号相关的服务通知，无法退订。",
  "email_frame_open_panel": "打开面板",
  "email_editor_placeholder": "输入正文，或按 / 插入内容块",
  "email_editor_cmd_text": "正文",
  "email_editor_cmd_text_desc": "普通段落",
  "email_editor_cmd_h1": "一级标题",
  "email_editor_cmd_h1_desc": "最大的标题",
  "email_editor_cmd_h2": "二级标题",
  "email_editor_cmd_h2_desc": "章节标题",
  "email_editor_cmd_h3": "三级标题",
  "email_editor_cmd_h3_desc": "小标题",
  "email_editor_cmd_bullets": "无序列表",
  "email_editor_cmd_bullets_desc": "带圆点的列表",
  "email_editor_cmd_numbers": "有序列表",
  "email_editor_cmd_numbers_desc": "带编号的列表",
  "email_editor_cmd_button": "按钮",
  "email_editor_cmd_button_desc": "按钮样式的链接",
  "email_editor_cmd_divider": "分隔线",
  "email_editor_cmd_divider_desc": "水平分隔线",
  "email_editor_not_ready": "编辑器仍在加载。"
```

- [ ] **Step 2: Write the fixed frame**

Create `frontend/src/emails/service-email-frame.tsx`:

```tsx
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
  return (
    <Html lang={language === "zh-cn" ? "zh-CN" : "en"}>
      <Head>
        <meta content="width=device-width" name="viewport" />
        <meta name="x-apple-disable-message-reformatting" />
      </Head>
      <Body style={{ ...bodyStyle, ...styles.body }}>
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
```

If `react-email` does not export one of these components (typecheck error "has no exported member"), import that component from the path the editor's own code uses. Report the change; do not add `@react-email/components`.

- [ ] **Step 3: Write the serializer plugin**

Create `frontend/src/emails/service-email-serializer.tsx`:

```tsx
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
export function createServiceEmailSerializer(frame: () => ServiceEmailFrameProps): SerializerPlugin {
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
```

- [ ] **Step 4: Write the editor component**

Create `frontend/src/emails/sendout-editor.tsx`:

```tsx
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
import "@react-email/editor/themes/default.css";
import * as m from "~/paraglide/messages.js";
import { createServiceEmailSerializer } from "./service-email-serializer";
import type { ServiceEmailFrameProps } from "./service-email-frame";

export type ComposedSendout = { html: string; text: string; content: Record<string, unknown> };
export type SendoutEditorHandle = { compose: () => Promise<ComposedSendout> };

// Only the blocks agreed for v1 stay enabled; images, columns, code and
// tables are out of scope, and the Inspector sidebar is never mounted.
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

function slashCommands() {
  return [
    { ...TEXT, title: m.email_editor_cmd_text(), description: m.email_editor_cmd_text_desc() },
    { ...H1, title: m.email_editor_cmd_h1(), description: m.email_editor_cmd_h1_desc() },
    { ...H2, title: m.email_editor_cmd_h2(), description: m.email_editor_cmd_h2_desc() },
    { ...H3, title: m.email_editor_cmd_h3(), description: m.email_editor_cmd_h3_desc() },
    { ...BULLET_LIST, title: m.email_editor_cmd_bullets(), description: m.email_editor_cmd_bullets_desc() },
    { ...NUMBERED_LIST, title: m.email_editor_cmd_numbers(), description: m.email_editor_cmd_numbers_desc() },
    { ...BUTTON, title: m.email_editor_cmd_button(), description: m.email_editor_cmd_button_desc() },
    { ...DIVIDER, title: m.email_editor_cmd_divider(), description: m.email_editor_cmd_divider_desc() },
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
      StarterKit.configure(DISABLED_BLOCKS),
      Placeholder.configure({ placeholder: () => m.email_editor_placeholder(), includeChildren: true }),
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
      <p className="sendout-canvas-footer">{m.email_frame_notice({ app: frame.appName }, options)}</p>
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
        const { unformattedHtml, text } = await composeReactEmail({ editor, preview: null });
        return { html: unformattedHtml, text, content: editor.getJSON() as Record<string, unknown> };
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
        omit={["Quote", "Code"]}
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
```

Two failure modes to watch for, both in the browser check of Task 4:

- If the editor throws at startup because a disabled extension is required by another one, re-enable only that one and record it in the report.
- `BubbleMenuLinkSelector` may not accept `open` / `onOpenChange` (typecheck error). In that case drop those two props and the `linkOpen` state.

- [ ] **Step 5: Map the editor theme to panel tokens and style the canvas**

Append to `frontend/src/styles/globals.css`:

```css
/* @react-email/editor floating menus: follow the panel theme (data-theme on
   <html>) instead of prefers-color-scheme, which the editor's default.css uses. */
html[data-theme] {
  --re-bg: var(--overlay);
  --re-bg-active: var(--surface-secondary);
  --re-border: var(--border);
  --re-separator: var(--separator);
  --re-text: var(--overlay-foreground);
  --re-text-muted: var(--muted);
  --re-hover: var(--surface-secondary);
  --re-active: var(--surface-tertiary);
  --re-pressed: var(--surface-tertiary);
  --re-danger: var(--danger);
  --re-radius: var(--radius);
}

/* The composer canvas always shows the email as recipients see it: light,
   whatever the panel theme is. */
.sendout-canvas {
  color-scheme: light;
  background: #ffffff;
  color: #18181b;
  border-radius: var(--radius);
  padding: 32px;
  max-width: 560px;
  margin: 0 auto;
}
.sendout-canvas .tiptap {
  min-height: 240px;
  outline: none;
}
.sendout-canvas-brand {
  margin: 0 0 24px;
  font-size: 14px;
  font-weight: 600;
}
.sendout-canvas-footer {
  margin: 32px 0 0;
  padding-top: 16px;
  border-top: 1px solid #e4e4e7;
  font-size: 12px;
  color: #71717a;
}
```

- [ ] **Step 6: Verify and commit**

```bash
pnpm build >/dev/null && pnpm typecheck && pnpm i18n:check && pnpm lint && pnpm format:check
```
Expected: all pass. Nothing renders these files yet. The build proves that the editor packages bundle for both client and SSR.

```bash
git add src/emails src/styles/globals.css messages/en.json messages/zh-cn.json
git commit -m "feat(frontend): add service email frame and sendout editor"
```

---

### Task 3: Sendout history page, rate setting, and Settings entry

**Files:**
- Create: `frontend/src/components/email-sendouts.tsx`
- Create: `frontend/src/routes/settings/emails/route.tsx`
- Create: `frontend/src/routes/settings/emails/index.tsx`
- Modify: `frontend/src/routes/settings/index.tsx`
- Modify: `frontend/src/routeTree.gen.ts` (regenerated)
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes: from Task 1, `fetchEmailSendoutContext`, `fetchEmailSendouts`, `fetchSettings`, `updateSettings`, `queryKeys.emailSendoutContext()`, `queryKeys.emailSendouts()`, `queryKeys.settings()`, and the types `EmailSendout`, `RecipientStatus`.
- Produces:
  - From `~/components/email-sendouts`: `SendoutStatusChip({ status })`, `RecipientStatusChip({ status })`, `languageLabel(language: string): string`, `audienceLabel(audience: string): string`, `reasonLabel(reason: string | undefined): string`, `RECIPIENT_STATUSES: RecipientStatus[]`, `recipientStatusLabel(status: string): string`.
  - Routes `/settings/emails` (layout + index).

- [ ] **Step 1: Add the message keys**

Add to `messages/en.json`:

```json
  "settings_email": "Email",
  "settings_email_desc": "Send transactional email to users.",
  "email_sendouts_title": "Email sendouts",
  "email_sendouts_desc": "Transactional email sent through the PocketBase SMTP settings. Recipients cannot unsubscribe.",
  "email_sendouts_new": "New sendout",
  "email_sendouts_smtp_off": "SMTP is not configured in PocketBase, so new sendouts are disabled. Configure it under Settings, Mail settings in the PocketBase dashboard.",
  "email_sendouts_history": "History",
  "email_sendouts_empty_title": "No sendouts yet",
  "email_sendouts_empty_hint": "Sendouts you queue appear here with their delivery progress.",
  "email_sendouts_th_subject": "Subject",
  "email_sendouts_th_audience": "Audience",
  "email_sendouts_th_progress": "Progress",
  "email_sendouts_th_created": "Created",
  "email_sendouts_progress": "{sent} of {total} sent",
  "email_sendouts_progress_failed": "{count} failed",
  "email_sendouts_rate": "Delivery rate",
  "email_sendouts_rate_label": "Emails per minute",
  "email_sendouts_rate_hint": "One worker sends emails one at a time at this rate. Match your SMTP provider's limit.",
  "email_sendouts_rate_save": "Save rate",
  "email_sendouts_rate_saved": "Saved",
  "email_language_en": "English",
  "email_language_zh_cn": "Simplified Chinese",
  "email_audience_single": "One user",
  "email_audience_all": "All active and verified users",
  "email_status_sending": "Sending",
  "email_status_completed": "Completed",
  "email_status_cancelled": "Cancelled",
  "email_recipient_status_pending": "Pending",
  "email_recipient_status_sending": "Sending",
  "email_recipient_status_sent": "Sent",
  "email_recipient_status_failed": "Failed",
  "email_recipient_status_skipped": "Skipped",
  "email_recipient_status_cancelled": "Cancelled",
  "email_reason_delivery_failed": "SMTP delivery failed",
  "email_reason_smtp_disabled": "SMTP was not configured",
  "email_reason_interrupted": "Interrupted by a restart",
  "email_reason_user_ineligible": "User no longer active and verified",
  "email_reason_user_deleted": "User was deleted"
```

Add to `messages/zh-cn.json`:

```json
  "settings_email": "邮件",
  "settings_email_desc": "向用户发送事务邮件。",
  "email_sendouts_title": "邮件发送",
  "email_sendouts_desc": "通过 PocketBase 的 SMTP 设置发送事务邮件，收件人无法退订。",
  "email_sendouts_new": "新建发送",
  "email_sendouts_smtp_off": "PocketBase 未配置 SMTP，暂时无法新建发送。请在 PocketBase 后台的 Settings 中配置 Mail settings。",
  "email_sendouts_history": "发送记录",
  "email_sendouts_empty_title": "还没有发送记录",
  "email_sendouts_empty_hint": "加入队列的邮件会显示在这里，并附带发送进度。",
  "email_sendouts_th_subject": "主题",
  "email_sendouts_th_audience": "收件范围",
  "email_sendouts_th_progress": "进度",
  "email_sendouts_th_created": "创建时间",
  "email_sendouts_progress": "已发送 {sent} / {total}",
  "email_sendouts_progress_failed": "失败 {count}",
  "email_sendouts_rate": "发送速率",
  "email_sendouts_rate_label": "每分钟封数",
  "email_sendouts_rate_hint": "单个 worker 按这个速率逐封发送，请与 SMTP 服务商的限额保持一致。",
  "email_sendouts_rate_save": "保存速率",
  "email_sendouts_rate_saved": "已保存",
  "email_language_en": "英文",
  "email_language_zh_cn": "简体中文",
  "email_audience_single": "单个用户",
  "email_audience_all": "所有启用且已验证的用户",
  "email_status_sending": "发送中",
  "email_status_completed": "已完成",
  "email_status_cancelled": "已取消",
  "email_recipient_status_pending": "待发送",
  "email_recipient_status_sending": "发送中",
  "email_recipient_status_sent": "已发送",
  "email_recipient_status_failed": "失败",
  "email_recipient_status_skipped": "已跳过",
  "email_recipient_status_cancelled": "已取消",
  "email_reason_delivery_failed": "SMTP 投递失败",
  "email_reason_smtp_disabled": "当时未配置 SMTP",
  "email_reason_interrupted": "因重启中断",
  "email_reason_user_ineligible": "用户已不再启用或未验证",
  "email_reason_user_deleted": "用户已被删除"
```

- [ ] **Step 2: Write the shared display helpers**

Create `frontend/src/components/email-sendouts.tsx`:

```tsx
import { Chip } from "@heroui/react";
import type { RecipientStatus } from "~/api/queries";
import * as m from "~/paraglide/messages.js";

export const RECIPIENT_STATUSES: RecipientStatus[] = [
  "pending",
  "sending",
  "sent",
  "failed",
  "skipped",
  "cancelled",
];

export function languageLabel(language: string): string {
  return language === "zh-cn" ? m.email_language_zh_cn() : m.email_language_en();
}

export function audienceLabel(audience: string): string {
  return audience === "all" ? m.email_audience_all() : m.email_audience_single();
}

export function recipientStatusLabel(status: string): string {
  const labels: Record<string, () => string> = {
    pending: m.email_recipient_status_pending,
    sending: m.email_recipient_status_sending,
    sent: m.email_recipient_status_sent,
    failed: m.email_recipient_status_failed,
    skipped: m.email_recipient_status_skipped,
    cancelled: m.email_recipient_status_cancelled,
  };
  return (labels[status] ?? m.common_unknown)();
}

export function reasonLabel(reason: string | undefined): string {
  if (!reason) return "";
  const labels: Record<string, () => string> = {
    delivery_failed: m.email_reason_delivery_failed,
    smtp_disabled: m.email_reason_smtp_disabled,
    interrupted: m.email_reason_interrupted,
    user_ineligible: m.email_reason_user_ineligible,
    user_deleted: m.email_reason_user_deleted,
  };
  return (labels[reason] ?? m.common_unknown)();
}

export function SendoutStatusChip({ status }: { status: string | undefined }) {
  const tone = status === "sending" ? "accent" : status === "completed" ? "success" : "default";
  const label =
    status === "sending"
      ? m.email_status_sending()
      : status === "completed"
        ? m.email_status_completed()
        : m.email_status_cancelled();
  return (
    <Chip size="sm" variant="soft" color={tone}>
      {label}
    </Chip>
  );
}

export function RecipientStatusChip({ status }: { status: string | undefined }) {
  const tone =
    status === "sent"
      ? "success"
      : status === "failed"
        ? "danger"
        : status === "pending" || status === "sending"
          ? "accent"
          : "default";
  return (
    <Chip size="sm" variant="soft" color={tone}>
      {recipientStatusLabel(status ?? "")}
    </Chip>
  );
}
```

If `Chip`'s `color` prop does not accept `"accent"` or `"danger"`, check `@heroui/react`'s `Chip` props (via the HeroUI MCP `get_component_docs(["Chip"])`) and use the closest supported values (for example `"warning"` for in-progress). Record the mapping in the report.

- [ ] **Step 3: Write the layout route**

Create `frontend/src/routes/settings/emails/route.tsx`:

```tsx
import { Outlet, createFileRoute } from "@tanstack/react-router";
import { requireAdmin } from "~/api/guards";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/emails")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.email_sendouts_title(),
    href: "/settings/emails",
  }),
  component: EmailsLayout,
});

function EmailsLayout() {
  return <Outlet />;
}
```

- [ ] **Step 4: Write the history page**

Create `frontend/src/routes/settings/emails/index.tsx`:

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Label, NumberField } from "@heroui/react";
import { Plus } from "@gravity-ui/icons";
import {
  fetchEmailSendoutContext,
  fetchEmailSendouts,
  fetchSettings,
  queryErrorMessage,
  queryKeys,
  updateSettings,
  type EmailSendout,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { SendoutStatusChip, audienceLabel, languageLabel } from "~/components/email-sendouts";
import { relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import * as m from "~/paraglide/messages.js";

const LIVE_REFRESH_MS = 5000;

export const Route = createFileRoute("/settings/emails/")({
  loader: ({ context }) => {
    markResponsePrivate();
    return Promise.allSettled([
      context.queryClient.ensureQueryData({
        queryKey: queryKeys.emailSendoutContext(),
        queryFn: fetchEmailSendoutContext,
      }),
      context.queryClient.ensureQueryData({
        queryKey: queryKeys.emailSendouts(),
        queryFn: fetchEmailSendouts,
      }),
    ]);
  },
  component: EmailSendoutsPage,
});

function EmailSendoutsPage() {
  const { auth } = Route.useRouteContext();
  const now = useHydratedNow();
  const contextQuery = useQuery({
    queryKey: queryKeys.emailSendoutContext(),
    queryFn: fetchEmailSendoutContext,
  });
  const sendoutsQuery = useQuery({
    queryKey: queryKeys.emailSendouts(),
    queryFn: fetchEmailSendouts,
    refetchInterval: (query) =>
      query.state.data?.some((s) => s.status === "sending") ? LIVE_REFRESH_MS : false,
  });
  const smtpEnabled = contextQuery.data?.smtp_enabled ?? false;
  const sendouts = sendoutsQuery.data ?? [];
  const loadError = sendoutsQuery.error ? queryErrorMessage(sendoutsQuery.error) : "";

  return (
    <PageShell
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-base font-semibold tracking-tight">{m.email_sendouts_title()}</h1>
          <p className="mt-0.5 max-w-2xl text-[13px] text-muted">{m.email_sendouts_desc()}</p>
        </div>
        {smtpEnabled ? (
          <Link
            to="/settings/emails/new"
            className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg bg-accent px-3 text-[13px] font-medium text-accent-foreground hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
          >
            <Plus className="size-3.5" aria-hidden />
            {m.email_sendouts_new()}
          </Link>
        ) : (
          <Button size="sm" variant="primary" isDisabled>
            <Plus className="size-3.5" aria-hidden />
            {m.email_sendouts_new()}
          </Button>
        )}
      </div>

      {contextQuery.data && !smtpEnabled ? (
        <div className="mb-4 rounded-lg border bg-surface-secondary px-4 py-3 text-[13px] text-muted">
          {m.email_sendouts_smtp_off()}
        </div>
      ) : null}
      <ErrorAlert message={loadError} icon className="mb-4" />

      <RateSection />

      <Section title={m.email_sendouts_history()}>
        {sendoutsQuery.isPending ? (
          <TableSkeleton />
        ) : sendouts.length === 0 ? (
          <PanelMessage>
            <span className="block font-medium text-foreground">{m.email_sendouts_empty_title()}</span>
            <span className="mt-1 block">{m.email_sendouts_empty_hint()}</span>
          </PanelMessage>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-[13px]">
              <thead>
                <tr className="border-b border-border bg-surface-secondary text-left">
                  <Th>{m.email_sendouts_th_subject()}</Th>
                  <Th>{m.common_status()}</Th>
                  <Th>{m.email_sendouts_th_audience()}</Th>
                  <Th>{m.email_sendouts_th_progress()}</Th>
                  <Th className="text-right">{m.email_sendouts_th_created()}</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-separator">
                {sendouts.map((sendout) => (
                  <SendoutRow key={sendout.id} sendout={sendout} now={now} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </PageShell>
  );
}

function SendoutRow({ sendout, now }: { sendout: EmailSendout; now: number | null }) {
  const counts = sendout.counts;
  return (
    <tr className="hover:bg-surface-secondary">
      <Td>
        <Link
          to="/settings/emails/$sendoutId"
          params={{ sendoutId: sendout.id ?? "" }}
          className="font-medium text-foreground hover:underline"
        >
          {sendout.subject}
        </Link>
        <span className="block text-xs text-muted">
          {languageLabel(sendout.language ?? "")} · {sendout.created_by_email || m.common_unknown()}
        </span>
      </Td>
      <Td>
        <SendoutStatusChip status={sendout.status} />
      </Td>
      <Td className="text-muted">{audienceLabel(sendout.audience ?? "")}</Td>
      <Td className="tabular-nums">
        {m.email_sendouts_progress({
          sent: String(counts?.sent ?? 0),
          total: String(counts?.total ?? 0),
        })}
        {counts?.failed ? (
          <span className="ml-2 text-danger">
            {m.email_sendouts_progress_failed({ count: String(counts.failed) })}
          </span>
        ) : null}
      </Td>
      <Td className="whitespace-nowrap text-right text-muted">
        {relTimeFromISO(sendout.created ?? "", now)}
      </Td>
    </tr>
  );
}

function RateSection() {
  const queryClient = useQueryClient();
  const settingsQuery = useQuery({ queryKey: queryKeys.settings(), queryFn: fetchSettings });
  const saved = settingsQuery.data?.email_sendout_rate_per_minute ?? 30;
  const [draft, setDraft] = useState<number | null>(null);
  const value = draft ?? saved;
  const mutation = useMutation({
    mutationFn: (rate: number) => updateSettings({ email_sendout_rate_per_minute: rate }),
    onSuccess: (data) => {
      queryClient.setQueryData(queryKeys.settings(), data);
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailSendoutContext() });
      setDraft(null);
    },
  });

  return (
    <Section className="mt-0 mb-6" title={m.email_sendouts_rate()}>
      <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-end">
        <NumberField
          className="w-40"
          minValue={1}
          maxValue={600}
          value={value}
          onChange={(next) => setDraft(Number.isFinite(next) ? next : null)}
        >
          <Label>{m.email_sendouts_rate_label()}</Label>
          <NumberField.Group>
            <NumberField.DecrementButton />
            <NumberField.Input />
            <NumberField.IncrementButton />
          </NumberField.Group>
        </NumberField>
        <Button
          size="sm"
          variant="secondary"
          isPending={mutation.isPending}
          isDisabled={mutation.isPending || draft === null || draft === saved}
          onPress={() => draft !== null && mutation.mutate(draft)}
        >
          {m.email_sendouts_rate_save()}
        </Button>
        {mutation.isSuccess && draft === null ? (
          <span className="text-xs text-muted">{m.email_sendouts_rate_saved()}</span>
        ) : null}
      </div>
      <p className="px-4 pb-4 text-xs text-muted">{m.email_sendouts_rate_hint()}</p>
      <ErrorAlert
        message={mutation.error ? queryErrorMessage(mutation.error) : ""}
        className="mx-4 mb-4"
      />
    </Section>
  );
}
```

Check the `NumberField` compound API against its use in `src/routes/nodes/new.tsx` (search for `NumberField`) and copy that exact sub-component structure if it differs. The link to `/settings/emails/$sendoutId` does not typecheck until Task 5 creates that route. So create a minimal placeholder `src/routes/settings/emails/$sendoutId.tsx` now: `createFileRoute("/settings/emails/$sendoutId")` with `beforeLoad` `requireAdmin` and a component that returns `null`. Task 5 replaces it.

- [ ] **Step 5: Add the Settings entry**

In `frontend/src/routes/settings/index.tsx`:

1. Add `Envelope` to the `@gravity-ui/icons` import.
2. Directly before the `<div className="mt-8 mb-5">` block that holds `m.settings_database()`, insert:

```tsx
          <div className="mt-8 mb-5">
            <h1 className="text-base font-semibold tracking-tight">{m.settings_email()}</h1>
            <p className="mt-0.5 text-[13px] text-muted">{m.settings_email_desc()}</p>
          </div>

          <Link
            to="/settings/emails"
            className="group flex items-center gap-3 rounded-lg border bg-surface px-4 py-3.5 transition-colors duration-150 hover:bg-surface-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
          >
            <span className="grid size-8 shrink-0 place-items-center rounded-lg border bg-surface-secondary text-muted">
              <Envelope className="size-4" aria-hidden />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-medium text-foreground">
                {m.email_sendouts_title()}
              </span>
              <span className="block text-xs text-muted">{m.email_sendouts_desc()}</span>
            </span>
            <ChevronRight
              className="size-4 shrink-0 text-muted transition-colors duration-150 group-hover:text-foreground"
              aria-hidden
            />
          </Link>
```

This block sits inside the same admin-only branch as the Notifications and Database links. Confirm that before inserting, because non-admins must not see the link.

- [ ] **Step 6: Verify and commit**

```bash
pnpm build >/dev/null && pnpm typecheck && pnpm i18n:check && pnpm lint && pnpm format:check
git status --short src/routeTree.gen.ts
```
Expected: all checks pass, and `routeTree.gen.ts` shows as modified with the new routes.

Browser check: start the stack from the "Local stack for browser checks" appendix at the end of this plan, open `http://localhost:3000/settings`, follow the Email link, and confirm:

- the page shows an empty history;
- the rate field shows 30, and saving 45 persists after a reload;
- with SMTP disabled, the notice shows and "New sendout" is disabled.

```bash
git add src/components/email-sendouts.tsx src/routes/settings/emails src/routes/settings/index.tsx src/routeTree.gen.ts messages/en.json messages/zh-cn.json
git commit -m "feat(frontend): add email sendout history page and rate setting"
```

---

### Task 4: Composer page with review and send

**Files:**
- Create: `frontend/src/routes/settings/emails/new.tsx`
- Modify: `frontend/src/routeTree.gen.ts` (regenerated)
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes:
  - Task 1: `fetchEmailSendoutContext`, `fetchEmailSendoutCandidates`, `createEmailSendout`, `queryKeys.*`, and the `EmailSendoutCreateRequest` type.
  - Task 2: `SendoutEditor`, `SendoutEditorHandle`, `ComposedSendout`, `SendoutLanguage`.
  - Task 3: `languageLabel`, `audienceLabel`.
- Produces: the route `/settings/emails/new`. On success it navigates to `/settings/emails/$sendoutId`.

- [ ] **Step 1: Add the message keys**

Add to `messages/en.json`:

```json
  "email_compose_title": "New sendout",
  "email_compose_subject": "Subject",
  "email_compose_language": "Language",
  "email_compose_language_hint": "Sets the language of the fixed header and footer.",
  "email_compose_audience": "Recipients",
  "email_compose_audience_all_count": "All active and verified users ({count})",
  "email_compose_recipient": "User",
  "email_compose_recipient_placeholder": "Search by email",
  "email_compose_recipient_empty": "No active and verified user matches.",
  "email_compose_app_name_hint": "The header shows the Application name from PocketBase settings: {app}",
  "email_compose_review": "Review and send",
  "email_compose_review_title": "Review the email",
  "email_compose_review_to_all": "This email goes to {count} users. It cannot be unsent.",
  "email_compose_review_to_one": "This email goes to {email}. It cannot be unsent.",
  "email_compose_send": "Send",
  "email_compose_sending": "Queueing"
```

Add to `messages/zh-cn.json`:

```json
  "email_compose_title": "新建发送",
  "email_compose_subject": "主题",
  "email_compose_language": "语言",
  "email_compose_language_hint": "决定固定页眉页脚使用的语言。",
  "email_compose_audience": "收件人",
  "email_compose_audience_all_count": "所有启用且已验证的用户（{count} 位）",
  "email_compose_recipient": "用户",
  "email_compose_recipient_placeholder": "按邮箱搜索",
  "email_compose_recipient_empty": "没有匹配的启用且已验证用户。",
  "email_compose_app_name_hint": "页眉显示 PocketBase 设置中的 Application name：{app}",
  "email_compose_review": "检查并发送",
  "email_compose_review_title": "检查邮件",
  "email_compose_review_to_all": "这封邮件将发送给 {count} 位用户，发出后无法撤回。",
  "email_compose_review_to_one": "这封邮件将发送给 {email}，发出后无法撤回。",
  "email_compose_send": "发送",
  "email_compose_sending": "正在加入队列"
```

- [ ] **Step 2: Write the composer route**

Create `frontend/src/routes/settings/emails/new.tsx`:

```tsx
import { Suspense, lazy, useDeferredValue, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
  Button,
  ComboBox,
  Description,
  Input,
  Label,
  ListBox,
  Modal,
  Radio,
  RadioGroup,
  TextField,
  type Key,
} from "@heroui/react";
import {
  createEmailSendout,
  fetchEmailSendoutCandidates,
  fetchEmailSendoutContext,
  queryErrorMessage,
  queryKeys,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { BrandLink, ErrorAlert, PageShell, SelectField } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { languageLabel } from "~/components/email-sendouts";
import type { ComposedSendout, SendoutEditorHandle } from "~/emails/sendout-editor";
import type { SendoutLanguage } from "~/emails/service-email-frame";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { useMounted } from "~/lib/use-mounted";
import * as m from "~/paraglide/messages.js";
import { getLocale } from "~/paraglide/runtime.js";

// The editor touches the DOM and pulls in TipTap and React Email, so it loads
// only in the browser and only on this page.
const SendoutEditor = lazy(() =>
  import("~/emails/sendout-editor").then((mod) => ({ default: mod.SendoutEditor }))
);

type Audience = "all" | "single";

export const Route = createFileRoute("/settings/emails/new")({
  staticData: breadcrumbStaticData({ label: () => m.email_compose_title() }),
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData({
      queryKey: queryKeys.emailSendoutContext(),
      queryFn: fetchEmailSendoutContext,
    });
  },
  component: ComposeSendoutPage,
});

function ComposeSendoutPage() {
  const { auth } = Route.useRouteContext();
  const mounted = useMounted();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const editorRef = useRef<SendoutEditorHandle>(null);
  const contextQuery = useQuery({
    queryKey: queryKeys.emailSendoutContext(),
    queryFn: fetchEmailSendoutContext,
  });

  const [subject, setSubject] = useState("");
  const [language, setLanguage] = useState<SendoutLanguage>(
    getLocale() === "zh-cn" ? "zh-cn" : "en"
  );
  const [audience, setAudience] = useState<Audience>("all");
  const [recipientSearch, setRecipientSearch] = useState("");
  const [recipient, setRecipient] = useState<{ id: string; email: string } | null>(null);
  const [empty, setEmpty] = useState(true);
  const [review, setReview] = useState<ComposedSendout | null>(null);
  const [composeError, setComposeError] = useState("");

  const deferredSearch = useDeferredValue(recipientSearch.trim());
  const candidatesQuery = useQuery({
    queryKey: queryKeys.emailSendoutCandidates(deferredSearch),
    queryFn: () => fetchEmailSendoutCandidates(deferredSearch),
    enabled: audience === "single",
  });

  const createMutation = useMutation({
    mutationFn: createEmailSendout,
    onSuccess: (sendout) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailSendouts() });
      void navigate({
        to: "/settings/emails/$sendoutId",
        params: { sendoutId: sendout.id ?? "" },
      });
    },
  });

  const context = contextQuery.data;
  const frame = {
    language,
    appName: context?.app_name ?? "",
    frontendUrl: context?.frontend_url ?? "",
  };
  const canReview =
    subject.trim() !== "" && !empty && (audience === "all" || recipient !== null);

  async function openReview() {
    setComposeError("");
    try {
      const composed = await editorRef.current?.compose();
      if (composed) setReview(composed);
    } catch (error) {
      setComposeError(error instanceof Error ? error.message : String(error));
    }
  }

  // Send the exact bytes shown in the review, never a fresh composition.
  function send() {
    if (!review || createMutation.isPending) return;
    createMutation.mutate({
      subject: subject.trim(),
      language,
      audience,
      user_id: audience === "single" ? recipient?.id : undefined,
      html: review.html,
      text: review.text,
      content: review.content,
    });
  }

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <h1 className="mb-6 text-base font-semibold tracking-tight">{m.email_compose_title()}</h1>

      {context && !context.smtp_enabled ? (
        <div className="rounded-lg border bg-surface-secondary px-4 py-3 text-[13px] text-muted">
          {m.email_sendouts_smtp_off()}
        </div>
      ) : (
        <div className="flex flex-col gap-5">
          <TextField value={subject} onChange={setSubject} isRequired maxLength={200}>
            <Label>{m.email_compose_subject()}</Label>
            <Input autoComplete="off" data-1p-ignore data-lpignore="true" />
          </TextField>

          <SelectField
            label={m.email_compose_language()}
            value={language}
            onChange={(value) => setLanguage(value === "zh-cn" ? "zh-cn" : "en")}
            options={[
              { value: "en", label: languageLabel("en") },
              { value: "zh-cn", label: languageLabel("zh-cn") },
            ]}
            description={m.email_compose_language_hint()}
          />

          <RadioGroup
            name="audience"
            value={audience}
            onChange={(value) => setAudience(value === "single" ? "single" : "all")}
          >
            <Label>{m.email_compose_audience()}</Label>
            <Radio value="all">
              <Radio.Content>
                <Radio.Control>
                  <Radio.Indicator />
                </Radio.Control>
                {m.email_compose_audience_all_count({
                  count: String(context?.eligible_recipient_count ?? 0),
                })}
              </Radio.Content>
            </Radio>
            <Radio value="single">
              <Radio.Content>
                <Radio.Control>
                  <Radio.Indicator />
                </Radio.Control>
                {m.email_audience_single()}
              </Radio.Content>
            </Radio>
          </RadioGroup>

          {audience === "single" ? (
            <ComboBox
              allowsEmptyCollection
              inputValue={recipientSearch}
              onInputChange={setRecipientSearch}
              selectedKey={recipient?.id ?? null}
              onSelectionChange={(key: Key | null) => {
                const match = (candidatesQuery.data ?? []).find((c) => c.id === key);
                setRecipient(match?.id ? { id: match.id, email: match.email ?? "" } : null);
              }}
            >
              <Label>{m.email_compose_recipient()}</Label>
              <ComboBox.InputGroup>
                <Input placeholder={m.email_compose_recipient_placeholder()} />
                <ComboBox.Trigger />
              </ComboBox.InputGroup>
              <ComboBox.Popover>
                <ListBox
                  renderEmptyState={() => (
                    <p className="px-3 py-2 text-xs text-muted">
                      {m.email_compose_recipient_empty()}
                    </p>
                  )}
                >
                  {(candidatesQuery.data ?? []).map((candidate) => (
                    <ListBox.Item
                      key={candidate.id}
                      id={candidate.id}
                      textValue={candidate.email}
                    >
                      {candidate.email}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </ComboBox.Popover>
            </ComboBox>
          ) : null}

          <div>
            <p className="mb-2 text-xs text-muted">
              {m.email_compose_app_name_hint({ app: frame.appName })}
            </p>
            <div className="rounded-lg border bg-surface-secondary p-4">
              {mounted && context ? (
                <Suspense fallback={<div className="h-72 animate-pulse rounded-lg bg-surface" />}>
                  <SendoutEditor ref={editorRef} frame={frame} onEmptyChange={setEmpty} />
                </Suspense>
              ) : (
                <div className="h-72 animate-pulse rounded-lg bg-surface" />
              )}
            </div>
          </div>

          <ErrorAlert message={composeError} />
          <div className="flex justify-end">
            <Button variant="primary" isDisabled={!canReview} onPress={() => void openReview()}>
              {m.email_compose_review()}
            </Button>
          </div>
        </div>
      )}

      <Modal.Backdrop
        isOpen={review !== null}
        onOpenChange={(open) => !open && !createMutation.isPending && setReview(null)}
      >
        <Modal.Container size="lg" placement="auto">
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>{m.email_compose_review_title()}</Modal.Heading>
              <p className="mt-1.5 text-sm text-muted">
                {audience === "all"
                  ? m.email_compose_review_to_all({
                      count: String(context?.eligible_recipient_count ?? 0),
                    })
                  : m.email_compose_review_to_one({ email: recipient?.email ?? "" })}
              </p>
            </Modal.Header>
            <Modal.Body>
              <p className="mb-2 text-[13px] font-medium text-foreground">{subject.trim()}</p>
              <iframe
                title={m.email_compose_review_title()}
                sandbox=""
                srcDoc={review?.html ?? ""}
                className="h-[28rem] w-full rounded-lg border bg-white"
              />
              {createMutation.error ? (
                <Description className="mt-3 text-danger">
                  {queryErrorMessage(createMutation.error, m.error_email_sendout_create_network())}
                </Description>
              ) : null}
            </Modal.Body>
            <Modal.Footer>
              <Button
                size="sm"
                variant="secondary"
                isDisabled={createMutation.isPending}
                onPress={() => setReview(null)}
              >
                {m.common_cancel()}
              </Button>
              <Button
                size="sm"
                variant="primary"
                isPending={createMutation.isPending}
                isDisabled={createMutation.isPending}
                onPress={send}
              >
                {createMutation.isPending ? m.email_compose_sending() : m.email_compose_send()}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </PageShell>
  );
}
```

Notes for the implementer:
- `send()` returns early while a request is in flight, so a quick double click queues only one Sendout, even before `isDisabled` has re-rendered.
- If `Key` is not exported from `@heroui/react`, import it from `react-aria-components`, where HeroUI re-exports it. Check `src/routes/settings/notifications.tsx`, which imports `type Key` from `react`.
- If `Modal.Container` does not support `size="lg"`, use the largest size its type allows.
- Also add `Description` to the imports only if `FieldError` is not the better fit. The error must be visible and have `role` or `aria-live` semantics equivalent to `ErrorAlert`. Using `<ErrorAlert message=... className="mt-3" />` inside the body is acceptable and simpler.

- [ ] **Step 3: Verify and commit**

```bash
pnpm build >/dev/null && pnpm typecheck && pnpm i18n:check && pnpm lint && pnpm format:check
```
Expected: all pass.

Browser check with the local stack (appendix), SMTP enabled toward an unreachable host:

1. Hard-refresh `http://localhost:3000/settings/emails/new`. The page renders without a server error, and the editor appears after hydration. There are no hydration warnings in the console.
2. Type `/` in the editor. The slash menu lists only the 8 localized entries. Select text: the bubble menu offers block type (no Quote or Code), link, bold, italic, underline and strike.
3. Set the panel theme to Light while the OS is in dark mode (or the reverse). The menus follow the panel theme, and the canvas stays light.
4. Choose "One user", search for the admin's email, and select it. Then write a subject and body and open "Review and send". The iframe shows the header with the app name, the body, and the footer in the selected language. The iframe element has `sandbox=""`.
5. Close the review, change the body, open the review again, and click Send twice quickly. The app lands on the detail page. `GET /api/panel/email-sendouts` returns exactly one new Sendout, and its content matches the second review.

```bash
git add src/routes/settings/emails/new.tsx src/routeTree.gen.ts messages/en.json messages/zh-cn.json
git commit -m "feat(frontend): add email sendout composer with review and send"
```

---

### Task 5: Sendout detail page

**Files:**
- Modify (replace the placeholder): `frontend/src/routes/settings/emails/$sendoutId.tsx`
- Modify: `frontend/src/routeTree.gen.ts` (regenerated, if it changed)
- Modify: `frontend/messages/en.json`, `frontend/messages/zh-cn.json`

**Interfaces:**
- Consumes:
  - Task 1: `fetchEmailSendout`, `fetchEmailSendoutRecipients`, `cancelEmailSendout`, `resendEmailSendout`, `queryKeys.emailSendout`, `queryKeys.emailSendoutRecipients`, `queryKeys.emailSendouts`, and the types `EmailSendoutRecipient` and `RecipientStatus`.
  - Task 3: `SendoutStatusChip`, `RecipientStatusChip`, `RECIPIENT_STATUSES`, `recipientStatusLabel`, `reasonLabel`, `languageLabel`, `audienceLabel`.
- Produces: the route `/settings/emails/$sendoutId`.

- [ ] **Step 1: Add the message keys**

Add to `messages/en.json`:

```json
  "email_detail_fallback_title": "Sendout",
  "email_detail_meta": "{language} · {audience} · by {email} · {created}",
  "email_detail_counts": "{sent} sent, {failed} failed, {skipped} skipped, {pending} pending, {cancelled} cancelled of {total}",
  "email_detail_preview": "Email",
  "email_detail_recipients": "Recipients",
  "email_detail_filter": "Status",
  "email_detail_filter_all": "All statuses",
  "email_detail_th_reason": "Reason",
  "email_detail_th_attempts": "Attempts",
  "email_detail_th_last_attempt": "Last attempt",
  "email_detail_resend_failed": "Resend failed ({count})",
  "email_detail_resend_one": "Resend",
  "email_detail_resend_title": "Resend failed recipients",
  "email_detail_resend_confirm": "{count} failed recipients go back to the end of the queue and are sent once more.",
  "email_detail_resending": "Requeueing",
  "email_detail_cancel": "Cancel sendout",
  "email_detail_cancel_title": "Cancel this sendout",
  "email_detail_cancel_confirm": "Recipients that are still pending will not receive this email. Emails already sent are not affected.",
  "email_detail_cancelling": "Cancelling",
  "email_detail_no_recipients": "No recipients in this status."
```

Add to `messages/zh-cn.json`:

```json
  "email_detail_fallback_title": "发送详情",
  "email_detail_meta": "{language} · {audience} · 由 {email} 发起 · {created}",
  "email_detail_counts": "共 {total} 位：已发送 {sent}，失败 {failed}，已跳过 {skipped}，待发送 {pending}，已取消 {cancelled}",
  "email_detail_preview": "邮件内容",
  "email_detail_recipients": "收件人",
  "email_detail_filter": "状态",
  "email_detail_filter_all": "全部状态",
  "email_detail_th_reason": "原因",
  "email_detail_th_attempts": "尝试次数",
  "email_detail_th_last_attempt": "最近尝试",
  "email_detail_resend_failed": "重发失败项（{count}）",
  "email_detail_resend_one": "重发",
  "email_detail_resend_title": "重发失败的收件人",
  "email_detail_resend_confirm": "{count} 位失败的收件人将重新排到队尾，再发送一次。",
  "email_detail_resending": "正在重新排队",
  "email_detail_cancel": "取消发送",
  "email_detail_cancel_title": "取消这次发送",
  "email_detail_cancel_confirm": "仍在待发送的收件人将不会收到这封邮件，已发出的邮件不受影响。",
  "email_detail_cancelling": "正在取消",
  "email_detail_no_recipients": "没有处于该状态的收件人。"
```

- [ ] **Step 2: Write the detail route**

Replace `frontend/src/routes/settings/emails/$sendoutId.tsx` with:

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import {
  cancelEmailSendout,
  fetchEmailSendout,
  fetchEmailSendoutRecipients,
  queryErrorMessage,
  queryKeys,
  resendEmailSendout,
  type EmailSendoutRecipient,
  type RecipientStatus,
} from "~/api/queries";
import { requireAdmin } from "~/api/guards";
import { markResponsePrivate } from "~/api/ssr";
import { SetBreadcrumbTitle } from "~/components/breadcrumbs";
import {
  RECIPIENT_STATUSES,
  RecipientStatusChip,
  SendoutStatusChip,
  audienceLabel,
  languageLabel,
  reasonLabel,
  recipientStatusLabel,
} from "~/components/email-sendouts";
import {
  BrandLink,
  DestructiveConfirmModal,
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  SelectField,
  TableSkeleton,
  Td,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { formatLocaleDateTime, relTimeFromISO } from "~/lib/format";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import { useActiveTimeZone } from "~/lib/use-timezone";
import * as m from "~/paraglide/messages.js";

const LIVE_REFRESH_MS = 5000;

export const Route = createFileRoute("/settings/emails/$sendoutId")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.email_detail_fallback_title(),
    dynamic: true,
  }),
  loader: ({ context, params }) => {
    markResponsePrivate();
    return Promise.allSettled([
      context.queryClient.ensureQueryData({
        queryKey: queryKeys.emailSendout(params.sendoutId),
        queryFn: () => fetchEmailSendout(params.sendoutId),
      }),
      context.queryClient.ensureQueryData({
        queryKey: queryKeys.emailSendoutRecipients(params.sendoutId, ""),
        queryFn: () => fetchEmailSendoutRecipients(params.sendoutId, ""),
      }),
    ]);
  },
  component: SendoutDetailPage,
});

function SendoutDetailPage() {
  const { sendoutId } = Route.useParams();
  const { auth } = Route.useRouteContext();
  const queryClient = useQueryClient();
  const now = useHydratedNow();
  const tz = useActiveTimeZone();
  const [statusFilter, setStatusFilter] = useState<RecipientStatus | "">("");
  const [confirm, setConfirm] = useState<"cancel" | "resend" | null>(null);

  const sendoutQuery = useQuery({
    queryKey: queryKeys.emailSendout(sendoutId),
    queryFn: () => fetchEmailSendout(sendoutId),
    refetchInterval: (query) =>
      query.state.data?.status === "sending" ? LIVE_REFRESH_MS : false,
  });
  const sending = sendoutQuery.data?.status === "sending";
  const recipientsQuery = useQuery({
    queryKey: queryKeys.emailSendoutRecipients(sendoutId, statusFilter),
    queryFn: () => fetchEmailSendoutRecipients(sendoutId, statusFilter),
    refetchInterval: sending ? LIVE_REFRESH_MS : false,
  });

  const refresh = () => void queryClient.invalidateQueries({ queryKey: queryKeys.emailSendouts() });
  const cancelMutation = useMutation({
    mutationFn: () => cancelEmailSendout(sendoutId),
    onSuccess: () => {
      setConfirm(null);
      refresh();
    },
  });
  const resendMutation = useMutation({
    mutationFn: (recipientIds: string[]) => resendEmailSendout(sendoutId, recipientIds),
    onSuccess: () => {
      setConfirm(null);
      refresh();
    },
  });

  const sendout = sendoutQuery.data;
  const counts = sendout?.counts;
  const cancelled = sendout?.status === "cancelled";
  const failed = counts?.failed ?? 0;
  const recipients = recipientsQuery.data ?? [];
  const created = sendout?.created ? Date.parse(sendout.created) : Number.NaN;

  return (
    <PageShell
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <SetBreadcrumbTitle title={sendout?.subject} />
      <ErrorAlert
        message={sendoutQuery.error ? queryErrorMessage(sendoutQuery.error) : ""}
        icon
        className="mb-4"
      />

      {sendout ? (
        <>
          <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h1 className="truncate text-base font-semibold tracking-tight">{sendout.subject}</h1>
                <SendoutStatusChip status={sendout.status} />
              </div>
              <p className="mt-1 text-[13px] text-muted">
                {m.email_detail_meta({
                  language: languageLabel(sendout.language ?? ""),
                  audience: audienceLabel(sendout.audience ?? ""),
                  email: sendout.created_by_email || m.common_unknown(),
                  created: Number.isNaN(created)
                    ? m.common_em_dash()
                    : formatLocaleDateTime(created, undefined, tz),
                })}
              </p>
              <p className="mt-1 text-[13px] tabular-nums text-foreground">
                {m.email_detail_counts({
                  total: String(counts?.total ?? 0),
                  sent: String(counts?.sent ?? 0),
                  failed: String(failed),
                  skipped: String(counts?.skipped ?? 0),
                  pending: String(counts?.pending ?? 0),
                  cancelled: String(counts?.cancelled ?? 0),
                })}
              </p>
            </div>
            <div className="flex shrink-0 gap-2">
              {!cancelled && failed > 0 ? (
                <Button size="sm" variant="secondary" onPress={() => setConfirm("resend")}>
                  {m.email_detail_resend_failed({ count: String(failed) })}
                </Button>
              ) : null}
              {sending ? (
                <Button size="sm" variant="secondary" onPress={() => setConfirm("cancel")}>
                  {m.email_detail_cancel()}
                </Button>
              ) : null}
            </div>
          </div>

          <Section className="mt-0" title={m.email_detail_preview()}>
            <iframe
              title={m.email_detail_preview()}
              sandbox=""
              srcDoc={sendout.html ?? ""}
              className="h-[32rem] w-full bg-white"
            />
          </Section>

          <Section
            title={m.email_detail_recipients()}
            action={
              <SelectField
                label={m.email_detail_filter()}
                value={statusFilter || "all"}
                onChange={(value) =>
                  setStatusFilter(value === "all" ? "" : (value as RecipientStatus))
                }
                options={[
                  { value: "all", label: m.email_detail_filter_all() },
                  ...RECIPIENT_STATUSES.map((status) => ({
                    value: status,
                    label: recipientStatusLabel(status),
                  })),
                ]}
              />
            }
          >
            <ErrorAlert
              message={resendMutation.error ? queryErrorMessage(resendMutation.error) : ""}
              className="m-3"
            />
            {recipientsQuery.isPending ? (
              <TableSkeleton />
            ) : recipients.length === 0 ? (
              <PanelMessage>{m.email_detail_no_recipients()}</PanelMessage>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full border-collapse text-[13px]">
                  <thead>
                    <tr className="border-b border-border bg-surface-secondary text-left">
                      <Th>{m.common_email()}</Th>
                      <Th>{m.common_status()}</Th>
                      <Th>{m.email_detail_th_reason()}</Th>
                      <Th className="text-right">{m.email_detail_th_attempts()}</Th>
                      <Th className="text-right">{m.email_detail_th_last_attempt()}</Th>
                      <Th />
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-separator">
                    {recipients.map((recipient) => (
                      <RecipientRow
                        key={recipient.id}
                        recipient={recipient}
                        now={now}
                        canResend={!cancelled}
                        resending={
                          resendMutation.isPending &&
                          resendMutation.variables?.[0] === recipient.id
                        }
                        onResend={() => recipient.id && resendMutation.mutate([recipient.id])}
                      />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Section>
        </>
      ) : sendoutQuery.isPending ? (
        <TableSkeleton />
      ) : null}

      <DestructiveConfirmModal
        isOpen={confirm === "cancel"}
        title={m.email_detail_cancel_title()}
        body={m.email_detail_cancel_confirm()}
        confirmLabel={m.email_detail_cancel()}
        pendingLabel={m.email_detail_cancelling()}
        pending={cancelMutation.isPending}
        error={cancelMutation.error ? queryErrorMessage(cancelMutation.error) : ""}
        onOpenChange={(open) => !open && setConfirm(null)}
        onConfirm={() => cancelMutation.mutate()}
      />
      <DestructiveConfirmModal
        isOpen={confirm === "resend"}
        destructive={false}
        title={m.email_detail_resend_title()}
        body={m.email_detail_resend_confirm({ count: String(failed) })}
        confirmLabel={m.email_detail_resend_one()}
        pendingLabel={m.email_detail_resending()}
        pending={resendMutation.isPending}
        error={resendMutation.error ? queryErrorMessage(resendMutation.error) : ""}
        onOpenChange={(open) => !open && setConfirm(null)}
        onConfirm={() => resendMutation.mutate([])}
      />
    </PageShell>
  );
}

function RecipientRow({
  recipient,
  now,
  canResend,
  resending,
  onResend,
}: {
  recipient: EmailSendoutRecipient;
  now: number | null;
  canResend: boolean;
  resending: boolean;
  onResend: () => void;
}) {
  return (
    <tr>
      <Td className="font-mono text-xs">{recipient.email}</Td>
      <Td>
        <RecipientStatusChip status={recipient.status} />
      </Td>
      <Td className="text-muted">{reasonLabel(recipient.reason)}</Td>
      <Td className="text-right tabular-nums">{recipient.attempts ?? 0}</Td>
      <Td className="whitespace-nowrap text-right text-muted">
        {recipient.last_attempt_at ? relTimeFromISO(recipient.last_attempt_at, now) : m.common_never()}
      </Td>
      <Td className="text-right">
        {canResend && recipient.status === "failed" ? (
          <Button
            size="sm"
            variant="ghost"
            isPending={resending}
            isDisabled={resending}
            onPress={onResend}
          >
            {m.email_detail_resend_one()}
          </Button>
        ) : null}
      </Td>
    </tr>
  );
}
```

If `Button` has no `variant="ghost"`, use `"tertiary"` or `"secondary"`, whichever `Button`'s props type accepts. Check with typecheck.

- [ ] **Step 3: Verify and commit**

```bash
pnpm build >/dev/null && pnpm typecheck && pnpm i18n:check && pnpm lint && pnpm format:check
```
Expected: all pass.

Browser check with the local stack (SMTP enabled toward an unreachable host, so deliveries fail):

1. Open the Sendout created in Task 4. The preview iframe has `sandbox=""` and shows the reviewed content. Within a few seconds the recipient turns `failed` with the reason "SMTP delivery failed", without a manual reload.
2. Click "Resend" on the row. The row goes back to pending, then to failed again, and its attempts count rises to 2.
3. Create a Sendout to all users while at least 3 users exist, with the rate set to 1 per minute. Open it and use "Cancel sendout". The remaining pending rows turn `cancelled`, and the Sendout status becomes "Cancelled". The resend button then no longer shows.
4. Filter by "Failed". Only failed rows show.

```bash
git add 'src/routes/settings/emails/$sendoutId.tsx' src/routeTree.gen.ts messages/en.json messages/zh-cn.json
git commit -m "feat(frontend): add email sendout detail page with cancel and resend"
```

---

### Task 6: Documentation and final checks

**Files:**
- Modify: `AGENTS.md` (repo root; `CLAUDE.md` is a symlink to it)

**Interfaces:**
- Consumes: all earlier tasks.

- [ ] **Step 1: Update `AGENTS.md`**

1. In the directory tree under `frontend/src/`:
   - add the line `│       ├── emails/             Email Sendout 编辑器（@react-email/editor）+ 固定服务邮件外框（serializerPlugin.BaseTemplate）`
   - change the `routes/` line so that the route list includes `settings/emails`.
2. In the `## 前端（./frontend）` section, add this bullet after the i18n bullet:
   `- **Email Sendout 编辑器**：\`src/emails/\` 用 \`@react-email/editor\`（TipTap）在浏览器里生成最终邮件 HTML/text（自定义 \`serializerPlugin.BaseTemplate\` 套固定外框，en/zh-cn 文案走 Paraglide 的 \`{ locale }\` 参数），后端原样存储发送（ADR-0008）。编辑器只在客户端加载（\`lazy\` + \`useMounted\`），画布恒为浅色，浮层菜单的 \`--re-*\` 变量在 \`globals.css\` 映射到面板 token。邮件 HTML 只在 \`sandbox=""\` 的 iframe 中展示。`

- [ ] **Step 2: Run the full frontend and backend checks**

```bash
cd frontend && pnpm build >/dev/null && pnpm check
cd ../backend && go build ./... && go vet ./... && go test ./...
```
Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: describe the email sendout editor in the project guide"
```

---

## Appendix: Local stack for browser checks

Run from the repo root in a scratch shell. It uses a throwaway data dir, so the developer's `backend/pb_data` stays untouched.

```bash
DATA=$(mktemp -d)
cd backend
PANEL_MASTER_KEY=dev go run . superuser upsert root@example.com rootpassword123 --dir "$DATA"
PANEL_MASTER_KEY=dev go run . serve --dir "$DATA" --http 127.0.0.1:8090 &
sleep 3
SU=$(curl -s -X POST http://127.0.0.1:8090/api/collections/_superusers/auth-with-password \
  -H 'Content-Type: application/json' \
  -d '{"identity":"root@example.com","password":"rootpassword123"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
# Panel admin (the create request hook adds the Current Auth String).
curl -s -X POST http://127.0.0.1:8090/api/collections/users/records \
  -H "Authorization: $SU" -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"adminpassword123","passwordConfirm":"adminpassword123","role":"admin","status":"active","verified":true}'
# Two more eligible users for the broadcast and cancel checks.
for u in a b; do
  curl -s -X POST http://127.0.0.1:8090/api/collections/users/records \
    -H "Authorization: $SU" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$u@example.com\",\"password\":\"userpassword123\",\"passwordConfirm\":\"userpassword123\",\"role\":\"user\",\"status\":\"active\",\"verified\":true}"
done
# SMTP enabled toward a port nothing listens on, so every delivery fails fast.
curl -s -X PATCH http://127.0.0.1:8090/api/settings \
  -H "Authorization: $SU" -H 'Content-Type: application/json' \
  -d '{"meta":{"appName":"Hysterical Panel","senderName":"Panel","senderAddress":"panel@example.com"},"smtp":{"enabled":true,"host":"127.0.0.1","port":2599}}'
cd ../frontend && pnpm dev
```

Log in at `http://localhost:3000/login` as `admin@example.com` / `adminpassword123`. For the "SMTP disabled" checks in Task 3, send the same settings PATCH with `"smtp":{"enabled":false}`. Afterwards, stop the backend (`kill %1`) and remove `$DATA`.
