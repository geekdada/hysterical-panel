import { Suspense, lazy, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "@tanstack/react-form";
import {
  Button,
  ComboBox,
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
  emailSendoutContextQueryOptions,
  fetchEmailSendoutCandidates,
  queryErrorMessage,
  queryKeys,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import { BrandLink, ErrorAlert, LabeledSwitch, PageShell, SelectField } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { languageLabel } from "~/components/email-sendouts";
import type { ComposedSendout, SendoutEditorHandle } from "~/emails/sendout-editor";
import type { SendoutLanguage } from "~/emails/service-email-frame";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import { useDebouncedValue } from "~/lib/use-debounced-value";
import { useMounted } from "~/lib/use-mounted";
import * as m from "~/paraglide/messages.js";
import { getLocale } from "~/paraglide/runtime.js";

// The editor touches the DOM and pulls in TipTap and React Email, so it loads
// only in the browser and only on this page.
const SendoutEditor = lazy(() =>
  import("~/emails/sendout-editor").then((mod) => ({ default: mod.SendoutEditor }))
);

type Audience = "all" | "single";

const RECIPIENT_SEARCH_DEBOUNCE_MS = 300;

type Recipient = { id: string; email: string };

type ComposeValues = {
  subject: string;
  language: SendoutLanguage;
  audience: Audience;
  recipientSearch: string;
  recipient: Recipient | null;
  bodyEmpty: boolean;
  showPanelLink: boolean;
};

// The review keeps the values it was composed with, so the send matches it.
type Review = { composed: ComposedSendout; values: ComposeValues };

export const Route = createFileRoute("/settings/emails/new")({
  staticData: breadcrumbStaticData({ label: () => m.email_compose_title() }),
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData(emailSendoutContextQueryOptions());
  },
  component: ComposeSendoutPage,
});

function ComposeSendoutPage() {
  const { auth } = Route.useRouteContext();
  const mounted = useMounted();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const editorRef = useRef<SendoutEditorHandle>(null);
  const sendingRef = useRef(false);
  const contextQuery = useQuery(emailSendoutContextQueryOptions());

  const composeMutation = useMutation({
    mutationFn: async (values: ComposeValues): Promise<Review | null> => {
      const composed = await editorRef.current?.compose();
      return composed ? { composed, values } : null;
    },
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
    onError: () => {
      sendingRef.current = false;
    },
  });

  const defaultValues: ComposeValues = {
    subject: "",
    language: getLocale() === "zh-cn" ? "zh-cn" : "en",
    audience: "all",
    recipientSearch: "",
    recipient: null,
    bodyEmpty: true,
    showPanelLink: true,
  };
  const form = useForm({
    defaultValues,
    onSubmit: async ({ value }) => {
      createMutation.reset();
      try {
        await composeMutation.mutateAsync(value);
      } catch {
        // The mutation owns rendering the compose error.
      }
    },
  });

  const context = contextQuery.data;
  const appName = context?.app_name ?? "";
  const frontendUrl = context?.frontend_url ?? "";
  const review = composeMutation.data ?? null;
  const composeError = composeMutation.error
    ? composeMutation.error instanceof Error
      ? composeMutation.error.message
      : String(composeMutation.error)
    : "";

  // Send the exact bytes shown in the review, never a fresh composition.
  // isPending only updates on the next render, so a fast double click would
  // pass it twice; the ref closes that gap and stays set once a send succeeds.
  function send() {
    if (!review || sendingRef.current) return;
    sendingRef.current = true;
    const { composed, values } = review;
    createMutation.mutate({
      subject: values.subject.trim(),
      language: values.language,
      audience: values.audience,
      user_id: values.audience === "single" ? values.recipient?.id : undefined,
      html: composed.html,
      text: composed.text,
      content: composed.content,
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
        // Not a <form>: some editor toolbar buttons omit type="button" and
        // would submit it.
        <div className="flex flex-col gap-5">
          <form.Field name="subject">
            {(field) => (
              <TextField
                name="subject"
                value={field.state.value}
                onChange={field.handleChange}
                onBlur={field.handleBlur}
                isRequired
                maxLength={200}
              >
                <Label>{m.email_compose_subject()}</Label>
                <Input autoComplete="off" data-1p-ignore data-lpignore="true" />
              </TextField>
            )}
          </form.Field>

          <form.Field name="language">
            {(field) => (
              <SelectField
                label={m.email_compose_language()}
                value={field.state.value}
                onChange={(value) => field.handleChange(value === "zh-cn" ? "zh-cn" : "en")}
                options={[
                  { value: "en", label: languageLabel("en") },
                  { value: "zh-cn", label: languageLabel("zh-cn") },
                ]}
                description={m.email_compose_language_hint()}
              />
            )}
          </form.Field>

          <form.Field name="audience">
            {(field) => (
              <RadioGroup
                name="audience"
                value={field.state.value}
                onChange={(value) => field.handleChange(value === "single" ? "single" : "all")}
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
            )}
          </form.Field>

          <form.Subscribe selector={(s) => s.values.audience}>
            {(audience) =>
              audience === "single" ? (
                <form.Field name="recipientSearch">
                  {(searchField) => (
                    <form.Field name="recipient">
                      {(recipientField) => (
                        <RecipientComboBox
                          search={searchField.state.value}
                          onSearchChange={searchField.handleChange}
                          selectedId={recipientField.state.value?.id ?? null}
                          onSelect={recipientField.handleChange}
                        />
                      )}
                    </form.Field>
                  )}
                </form.Field>
              ) : null
            }
          </form.Subscribe>

          <div>
            <div className="rounded-lg border bg-surface-secondary p-4">
              {mounted && context ? (
                <Suspense fallback={<div className="h-72 animate-pulse rounded-lg bg-surface" />}>
                  <form.Subscribe
                    selector={(s) => ({
                      language: s.values.language,
                      showPanelLink: s.values.showPanelLink,
                    })}
                  >
                    {({ language, showPanelLink }) => (
                      <form.Field name="bodyEmpty">
                        {(field) => (
                          <SendoutEditor
                            ref={editorRef}
                            frame={{ language, appName, frontendUrl, showPanelLink }}
                            onEmptyChange={field.handleChange}
                          />
                        )}
                      </form.Field>
                    )}
                  </form.Subscribe>
                </Suspense>
              ) : (
                <div className="h-72 animate-pulse rounded-lg bg-surface" />
              )}
            </div>
            <p className="mt-2 text-xs text-muted">{m.email_compose_app_name_hint()}</p>
          </div>

          {/* The frame omits the link without a frontend URL, so the switch would do nothing. */}
          {frontendUrl ? (
            <form.Field name="showPanelLink">
              {(field) => (
                <LabeledSwitch
                  label={m.email_compose_panel_link()}
                  isSelected={field.state.value}
                  onChange={field.handleChange}
                />
              )}
            </form.Field>
          ) : null}

          <ErrorAlert message={composeError} />
          <div className="flex justify-end">
            <form.Subscribe
              selector={(s) => ({
                reviewable: canReview(s.values),
                isSubmitting: s.isSubmitting,
              })}
            >
              {({ reviewable, isSubmitting }) => (
                <Button
                  variant="primary"
                  isDisabled={!reviewable || isSubmitting}
                  onPress={() => void form.handleSubmit()}
                >
                  {m.email_compose_review()}
                </Button>
              )}
            </form.Subscribe>
          </div>
        </div>
      )}

      <Modal.Backdrop
        isOpen={review !== null}
        onOpenChange={(open) => !open && !createMutation.isPending && composeMutation.reset()}
      >
        <Modal.Container size="lg" placement="auto">
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>{m.email_compose_review_title()}</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <p className="mb-2 text-[13px] font-medium text-foreground">
                {m.email_compose_review_subject({ subject: review?.values.subject.trim() ?? "" })}
              </p>
              <iframe
                title={m.email_compose_review_title()}
                sandbox=""
                srcDoc={review?.composed.html ?? ""}
                className="h-[28rem] w-full rounded-lg border bg-white"
              />
              <ErrorAlert
                className="mt-3"
                message={
                  createMutation.error
                    ? queryErrorMessage(
                        createMutation.error,
                        m.error_email_sendout_create_network()
                      )
                    : null
                }
              />
            </Modal.Body>
            <Modal.Footer>
              <Button
                size="sm"
                variant="secondary"
                isDisabled={createMutation.isPending}
                onPress={() => composeMutation.reset()}
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

function RecipientComboBox({
  search,
  onSearchChange,
  selectedId,
  onSelect,
}: {
  search: string;
  onSearchChange: (search: string) => void;
  selectedId: string | null;
  onSelect: (recipient: Recipient | null) => void;
}) {
  const debouncedSearch = useDebouncedValue(search.trim(), RECIPIENT_SEARCH_DEBOUNCE_MS);
  const candidatesQuery = useQuery({
    queryKey: queryKeys.emailSendoutCandidates(debouncedSearch),
    queryFn: () => fetchEmailSendoutCandidates(debouncedSearch),
  });
  const candidates = candidatesQuery.data ?? [];

  return (
    <ComboBox
      allowsEmptyCollection
      inputValue={search}
      onInputChange={onSearchChange}
      selectedKey={selectedId}
      onSelectionChange={(key: Key | null) => {
        const match = candidates.find((c) => c.id === key);
        onSelect(match?.id ? { id: match.id, email: match.email ?? "" } : null);
        // With both inputValue and selectedKey controlled, React Aria
        // leaves the input text to us, so show the chosen email.
        if (match?.email) onSearchChange(match.email);
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
            <p className="px-3 py-2 text-xs text-muted">{m.email_compose_recipient_empty()}</p>
          )}
        >
          {candidates.map((candidate) => (
            <ListBox.Item key={candidate.id} id={candidate.id} textValue={candidate.email}>
              {candidate.email}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </ComboBox.Popover>
    </ComboBox>
  );
}

function canReview(values: ComposeValues): boolean {
  return (
    values.subject.trim() !== "" &&
    !values.bodyEmpty &&
    (values.audience === "all" || values.recipient !== null)
  );
}
