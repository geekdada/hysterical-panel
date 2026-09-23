import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@heroui/react";
import {
  cancelEmailSendout,
  emailSendoutQueryOptions,
  emailSendoutRecipientsQueryOptions,
  queryErrorMessage,
  queryKeys,
  resendEmailSendout,
  type EmailSendoutDetail,
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
      context.queryClient.ensureQueryData(emailSendoutQueryOptions(params.sendoutId)),
      context.queryClient.ensureQueryData(emailSendoutRecipientsQueryOptions(params.sendoutId, "")),
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
  const { sendoutQuery, recipientsQuery } = useLiveSendout(sendoutId, statusFilter);
  const sending = sendoutQuery.data?.status === "sending";

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

  // Opening or closing a dialog clears earlier errors, so one failure is not
  // shown both in the dialog and above the table.
  const toggleConfirm = (next: "cancel" | "resend" | null) => {
    if (!cancelMutation.isPending) cancelMutation.reset();
    if (!resendMutation.isPending) resendMutation.reset();
    setConfirm(next);
  };

  const sendout = sendoutQuery.data;
  const counts = sendout?.counts;
  const cancelled = sendout?.status === "cancelled";
  const failed = counts?.failed ?? 0;
  const recipients = recipientsQuery.data ?? [];
  const created = sendout?.created ? Date.parse(sendout.created) : Number.NaN;

  return (
    <PageShell headerLeft={<BrandLink />} headerRight={auth ? <UserMenu auth={auth} /> : undefined}>
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
                <h1 className="truncate text-base font-semibold tracking-tight">
                  {sendout.subject}
                </h1>
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
                <Button size="sm" variant="secondary" onPress={() => toggleConfirm("resend")}>
                  {m.email_detail_resend_failed({ count: String(failed) })}
                </Button>
              ) : null}
              {sending ? (
                <Button size="sm" variant="secondary" onPress={() => toggleConfirm("cancel")}>
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
                          resendMutation.isPending && resendMutation.variables?.[0] === recipient.id
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
        onOpenChange={(open) => !open && toggleConfirm(null)}
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
        onOpenChange={(open) => !open && toggleConfirm(null)}
        onConfirm={() => resendMutation.mutate([])}
      />
    </PageShell>
  );
}

function hasPendingRecipients(sendout: EmailSendoutDetail | undefined): boolean {
  return (sendout?.counts?.pending ?? 0) > 0;
}

// Polls while any recipient is pending or sending, not while the status is
// `sending`: a cancelled Sendout can still have a row in flight. The two polls
// run on separate timers, so the last change can land between them; both are
// refetched once when polling stops.
function useLiveSendout(sendoutId: string, statusFilter: RecipientStatus | "") {
  const queryClient = useQueryClient();
  const sendoutQuery = useQuery({
    ...emailSendoutQueryOptions(sendoutId),
    refetchInterval: (query) => (hasPendingRecipients(query.state.data) ? LIVE_REFRESH_MS : false),
  });
  const live = hasPendingRecipients(sendoutQuery.data);
  const recipientsQuery = useQuery({
    ...emailSendoutRecipientsQueryOptions(sendoutId, statusFilter),
    refetchInterval: live ? LIVE_REFRESH_MS : false,
  });

  const wasLive = useRef(live);
  useEffect(() => {
    if (wasLive.current && !live) {
      // Prefix match: the detail and every filter's recipient list.
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailSendout(sendoutId) });
    }
    wasLive.current = live;
  }, [live, queryClient, sendoutId]);

  return { sendoutQuery, recipientsQuery };
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
        {recipient.last_attempt_at
          ? relTimeFromISO(recipient.last_attempt_at, now)
          : m.common_never()}
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
