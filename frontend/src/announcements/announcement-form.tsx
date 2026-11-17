import { Suspense, lazy, type ReactNode } from "react";
import { useForm } from "@tanstack/react-form";
import { Button, Input, Label, TextField } from "@heroui/react";
import { LabeledSwitch } from "~/components/ui";
import { useMounted } from "~/lib/use-mounted";
import * as m from "~/paraglide/messages.js";
import type { AnnouncementDocument } from "./announcement-editor";

// TipTap touches the DOM, so the editor loads only in the browser and only on
// the pages that edit announcements.
const AnnouncementEditor = lazy(() =>
  import("./announcement-editor").then((mod) => ({ default: mod.AnnouncementEditor }))
);

const TITLE_MAX_LENGTH = 200;

export type AnnouncementFormValues = {
  title: string;
  document: AnnouncementDocument | null;
  pinned: boolean;
  hidden: boolean;
  uploading: boolean;
};

export type SubmittedAnnouncement = {
  title: string;
  html: string;
  content: Record<string, unknown>;
  pinned: boolean;
  hidden: boolean;
};

function canSubmit(values: AnnouncementFormValues): boolean {
  return (
    values.title.trim() !== "" &&
    values.document !== null &&
    !values.document.empty &&
    !values.uploading
  );
}

export function AnnouncementForm({
  initial,
  initialContent,
  showHidden,
  submitLabel,
  pending,
  footer,
  onSubmit,
}: {
  initial: Pick<AnnouncementFormValues, "title" | "pinned" | "hidden">;
  initialContent?: Record<string, unknown>;
  showHidden: boolean;
  submitLabel: string;
  pending: boolean;
  footer?: ReactNode;
  onSubmit: (announcement: SubmittedAnnouncement) => void;
}) {
  const mounted = useMounted();
  const form = useForm({
    defaultValues: { ...initial, document: null, uploading: false } as AnnouncementFormValues,
    onSubmit: ({ value }) => {
      if (!value.document || !canSubmit(value)) return;
      onSubmit({
        title: value.title.trim(),
        html: value.document.html,
        content: value.document.content,
        pinned: value.pinned && !value.hidden,
        hidden: value.hidden,
      });
    },
  });

  return (
    // Not a <form>: Enter in the editor and its toolbar buttons must never submit.
    <div className="flex flex-col gap-5">
      <form.Field name="title">
        {(field) => (
          <TextField
            name="title"
            value={field.state.value}
            onChange={field.handleChange}
            onBlur={field.handleBlur}
            isRequired
            maxLength={TITLE_MAX_LENGTH}
          >
            <Label>{m.announcement_form_title()}</Label>
            <Input autoComplete="off" data-1p-ignore data-lpignore="true" />
          </TextField>
        )}
      </form.Field>

      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-foreground">{m.announcement_form_body()}</span>
        {mounted ? (
          <Suspense fallback={<EditorSkeleton />}>
            <form.Field name="document">
              {(documentField) => (
                <form.Field name="uploading">
                  {(uploadingField) => (
                    <AnnouncementEditor
                      initialContent={initialContent}
                      onChange={documentField.handleChange}
                      onBusyChange={uploadingField.handleChange}
                    />
                  )}
                </form.Field>
              )}
            </form.Field>
          </Suspense>
        ) : (
          <EditorSkeleton />
        )}
      </div>

      {/* Hiding unpins, and a hidden announcement cannot be pinned. Unhiding
          leaves the pin off, matching the backend rule. */}
      <div className="flex flex-col gap-3">
        <form.Subscribe selector={(state) => state.values.hidden}>
          {(hidden) => (
            <form.Field name="pinned">
              {(field) => (
                <LabeledSwitch
                  label={m.announcement_form_pinned()}
                  description={
                    hidden ? m.announcement_form_pinned_hidden() : m.announcement_form_pinned_hint()
                  }
                  isSelected={field.state.value && !hidden}
                  isDisabled={hidden}
                  onChange={field.handleChange}
                />
              )}
            </form.Field>
          )}
        </form.Subscribe>
        {showHidden ? (
          <form.Field name="hidden">
            {(field) => (
              <LabeledSwitch
                label={m.announcement_form_hidden()}
                description={m.announcement_form_hidden_hint()}
                isSelected={field.state.value}
                onChange={(hidden) => {
                  field.handleChange(hidden);
                  if (hidden) form.setFieldValue("pinned", false);
                }}
              />
            )}
          </form.Field>
        ) : null}
      </div>

      <div className="flex items-center justify-between gap-3">
        <div>{footer}</div>
        <form.Subscribe selector={(state) => canSubmit(state.values)}>
          {(ready) => (
            <Button
              variant="primary"
              isPending={pending}
              isDisabled={!ready || pending}
              onPress={() => void form.handleSubmit()}
            >
              {submitLabel}
            </Button>
          )}
        </form.Subscribe>
      </div>
    </div>
  );
}

function EditorSkeleton() {
  return <div className="h-80 animate-pulse rounded-lg border bg-surface-secondary" />;
}
