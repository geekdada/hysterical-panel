import { Suspense, lazy, useDeferredValue, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
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
  const sendingRef = useRef(false);
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
    onError: () => {
      sendingRef.current = false;
    },
  });

  const context = contextQuery.data;
  const frame = {
    language,
    appName: context?.app_name ?? "",
    frontendUrl: context?.frontend_url ?? "",
  };
  const canReview = subject.trim() !== "" && !empty && (audience === "all" || recipient !== null);

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
  // isPending only updates on the next render, so a fast double click would
  // pass it twice; the ref closes that gap and stays set once a send succeeds.
  function send() {
    if (!review || sendingRef.current) return;
    sendingRef.current = true;
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
                // With both inputValue and selectedKey controlled, React Aria
                // leaves the input text to us, so show the chosen email.
                if (match?.email) setRecipientSearch(match.email);
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
                    <ListBox.Item key={candidate.id} id={candidate.id} textValue={candidate.email}>
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
