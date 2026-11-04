import { Suspense, lazy, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { createFileRoute, notFound } from "@tanstack/react-router";
import { useForm } from "@tanstack/react-form";
import { Button, Input, Label, TextField } from "@heroui/react";
import {
  automaticEmailQueryOptions,
  automaticEmailsQueryOptions,
  queryErrorMessage,
  useAutomaticEmail,
  useAutomaticEmails,
  useSaveAutomaticEmailTemplate,
  type AutomaticEmailDetail,
  type AutomaticEmailKind,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { SetBreadcrumbTitle } from "~/components/breadcrumbs";
import {
  automaticKindDescription,
  automaticKindLabel,
  isAutomaticEmailKind,
} from "~/components/email-sendouts";
import { BrandLink, CopyButton, ErrorAlert, PageShell, Section } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { automaticEmailDefaults } from "~/emails/automatic-email-defaults";
import type { SendoutEditorHandle } from "~/emails/sendout-editor";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { useMounted } from "~/lib/use-mounted";
import * as m from "~/paraglide/messages.js";

// The editor touches the DOM and pulls in TipTap and React Email, so it loads
// only in the browser and only on this page.
const SendoutEditor = lazy(() =>
  import("~/emails/sendout-editor").then((mod) => ({ default: mod.SendoutEditor }))
);

// Tokens stay literal in every locale; only their descriptions are translated.
const PLACEHOLDERS: { name: string; description: () => string }[] = [
  { name: "email", description: m.automatic_email_placeholder_email },
  { name: "remaining", description: m.automatic_email_placeholder_remaining },
  { name: "allowance", description: m.automatic_email_placeholder_allowance },
  { name: "used", description: m.automatic_email_placeholder_used },
  { name: "remaining_percent", description: m.automatic_email_placeholder_remaining_percent },
  { name: "window_starts_at", description: m.automatic_email_placeholder_window_starts_at },
  { name: "window_ends_at", description: m.automatic_email_placeholder_window_ends_at },
  {
    name: "subscription_ends_at",
    description: m.automatic_email_placeholder_subscription_ends_at,
  },
  { name: "panel_url", description: m.automatic_email_placeholder_panel_url },
];

type InitialTemplate = { subject: string; content: Record<string, unknown> };

function kindFromParam(kind: string): AutomaticEmailKind {
  if (!isAutomaticEmailKind(kind)) throw notFound();
  return kind;
}

export const Route = createFileRoute("/settings/automatic-emails/$kind")({
  staticData: breadcrumbStaticData({ label: () => m.automatic_email_template() }),
  beforeLoad: ({ params }) => {
    kindFromParam(params.kind);
  },
  loader: ({ context, params }) => {
    markResponsePrivate();
    const kind = kindFromParam(params.kind);
    return Promise.allSettled([
      context.queryClient.ensureQueryData(automaticEmailQueryOptions(kind)),
      context.queryClient.ensureQueryData(automaticEmailsQueryOptions()),
    ]);
  },
  component: AutomaticEmailTemplatePage,
});

function AutomaticEmailTemplatePage() {
  const { auth } = Route.useRouteContext();
  const kind = kindFromParam(Route.useParams().kind);
  const detailQuery = useAutomaticEmail(kind);
  const listQuery = useAutomaticEmails();
  const loadFailure = detailQuery.error ?? listQuery.error;
  const label = automaticKindLabel(kind);

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <SetBreadcrumbTitle title={label} />
      <h1 className="text-base font-semibold tracking-tight">{label}</h1>
      <p className="mt-0.5 mb-6 max-w-2xl text-[13px] text-muted">
        {automaticKindDescription(kind)}
      </p>
      <ErrorAlert message={loadFailure ? queryErrorMessage(loadFailure) : ""} className="mb-4" />

      {/* TipTap reads its content only at mount, so the form waits for the saved template. */}
      {detailQuery.data && listQuery.data ? (
        <TemplateForm
          key={kind}
          kind={kind}
          detail={detailQuery.data}
          appName={listQuery.data.app_name}
          frontendUrl={listQuery.data.frontend_url}
        />
      ) : loadFailure ? null : (
        <div className="h-96 animate-pulse rounded-lg bg-surface-secondary" />
      )}
    </PageShell>
  );
}

// A Kind without a template still returns `content: {}`, so has_template
// decides between the saved template and the default one.
function initialTemplate(kind: AutomaticEmailKind, detail: AutomaticEmailDetail): InitialTemplate {
  if (!detail.has_template) return automaticEmailDefaults[kind];
  return { subject: detail.subject, content: detail.content ?? {} };
}

function TemplateForm({
  kind,
  detail,
  appName,
  frontendUrl,
}: {
  kind: AutomaticEmailKind;
  detail: AutomaticEmailDetail;
  appName: string;
  frontendUrl: string;
}) {
  const mounted = useMounted();
  const editorRef = useRef<SendoutEditorHandle>(null);
  // Frozen at mount: a refetch after saving must not feed the editor new content.
  const [initial] = useState(() => initialTemplate(kind, detail));
  const saveMutation = useSaveAutomaticEmailTemplate(kind);
  const composeMutation = useMutation({
    mutationFn: async () => {
      const composed = await editorRef.current?.compose();
      if (!composed) throw new Error(m.email_editor_not_ready());
      return composed;
    },
  });

  const form = useForm({
    defaultValues: { subject: initial.subject, bodyEmpty: false },
    onSubmit: async ({ value }) => {
      clearOutcome();
      try {
        const composed = await composeMutation.mutateAsync();
        await saveMutation.mutateAsync({ subject: value.subject.trim(), ...composed });
      } catch {
        // Each mutation owns rendering its error.
      }
    },
  });

  // Any edit makes the last outcome stale, so its message clears.
  function clearOutcome() {
    composeMutation.reset();
    saveMutation.reset();
  }

  const failure = composeMutation.error ?? saveMutation.error;
  const frame = { language: "bilingual", appName, frontendUrl, showPanelLink: true } as const;

  return (
    // Not a <form>: some editor toolbar buttons omit type="button" and would submit it.
    <div className="flex flex-col gap-5">
      {detail.has_template ? null : (
        <p className="rounded-lg border bg-surface-secondary px-4 py-3 text-[13px] text-muted">
          {m.automatic_email_default_notice()}
        </p>
      )}

      <form.Field name="subject">
        {(field) => (
          <TextField
            name="subject"
            value={field.state.value}
            onChange={(next) => {
              field.handleChange(next);
              clearOutcome();
            }}
            onBlur={field.handleBlur}
            isRequired
            maxLength={200}
          >
            <Label>{m.email_compose_subject()}</Label>
            <Input autoComplete="off" data-1p-ignore data-lpignore="true" />
          </TextField>
        )}
      </form.Field>

      <div>
        <div className="rounded-lg border bg-surface-secondary p-4">
          {mounted ? (
            <Suspense fallback={<div className="h-72 animate-pulse rounded-lg bg-surface" />}>
              <form.Field name="bodyEmpty">
                {(field) => (
                  <SendoutEditor
                    ref={editorRef}
                    frame={frame}
                    initialContent={initial.content}
                    onEmptyChange={(empty) => {
                      field.handleChange(empty);
                      clearOutcome();
                    }}
                  />
                )}
              </form.Field>
            </Suspense>
          ) : (
            <div className="h-72 animate-pulse rounded-lg bg-surface" />
          )}
        </div>
        <p className="mt-2 text-xs text-muted">{m.automatic_email_frame_hint()}</p>
      </div>

      <PlaceholderList />

      <ErrorAlert message={failure ? queryErrorMessage(failure) : ""} />
      <div className="flex items-center justify-end gap-3">
        {saveMutation.isSuccess ? (
          <output className="text-xs text-muted">{m.automatic_email_saved()}</output>
        ) : null}
        <form.Subscribe
          selector={(s) => ({
            savable: s.values.subject.trim() !== "" && !s.values.bodyEmpty,
            isSubmitting: s.isSubmitting,
          })}
        >
          {({ savable, isSubmitting }) => (
            <Button
              variant="primary"
              isPending={isSubmitting}
              isDisabled={!savable || isSubmitting}
              onPress={() => void form.handleSubmit()}
            >
              {m.automatic_email_save()}
            </Button>
          )}
        </form.Subscribe>
      </div>
    </div>
  );
}

function PlaceholderList() {
  return (
    <Section className="mt-0" title={m.automatic_email_placeholders()}>
      <p className="border-b border-separator px-4 py-3 text-xs text-muted">
        {m.automatic_email_placeholders_hint()}
      </p>
      <dl className="divide-y divide-separator">
        {PLACEHOLDERS.map(({ name, description }) => {
          const token = `{{${name}}}`;
          return (
            <div
              key={name}
              className="flex flex-col gap-1 px-4 py-2.5 sm:flex-row sm:items-center sm:gap-4"
            >
              <dt className="flex items-center gap-1.5 sm:w-56 sm:shrink-0">
                <code className="font-mono text-xs text-foreground">{token}</code>
                <CopyButton value={token} label={token} alwaysVisible />
              </dt>
              <dd className="text-xs text-muted">{description()}</dd>
            </div>
          );
        })}
      </dl>
    </Section>
  );
}
