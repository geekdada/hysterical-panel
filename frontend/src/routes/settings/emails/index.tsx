import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Description, Label, NumberField } from "@heroui/react";
import {
  emailSendoutContextQueryOptions,
  emailSendoutsQueryOptions,
  queryErrorMessage,
  queryKeys,
  settingsQueryOptions,
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
      context.queryClient.ensureQueryData(emailSendoutContextQueryOptions()),
      context.queryClient.ensureQueryData(emailSendoutsQueryOptions()),
      context.queryClient.ensureQueryData(settingsQueryOptions()),
    ]);
  },
  component: EmailSendoutsPage,
});

function EmailSendoutsPage() {
  const { auth } = Route.useRouteContext();
  const now = useHydratedNow();
  const contextQuery = useQuery(emailSendoutContextQueryOptions());
  const sendoutsQuery = useQuery({
    ...emailSendoutsQueryOptions(),
    refetchInterval: (query) =>
      query.state.data?.some((s) => s.status === "sending") ? LIVE_REFRESH_MS : false,
  });
  const smtpEnabled = contextQuery.data?.smtp_enabled ?? false;
  const sendouts = sendoutsQuery.data ?? [];
  const loadFailure = sendoutsQuery.error ?? contextQuery.error;
  const loadError = loadFailure ? queryErrorMessage(loadFailure) : "";

  return (
    <PageShell headerLeft={<BrandLink />} headerRight={auth ? <UserMenu auth={auth} /> : undefined}>
      <h1 className="mb-6 text-base font-semibold tracking-tight">{m.settings_email_manage()}</h1>

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
        ) : sendoutsQuery.error && sendouts.length === 0 ? null : sendouts.length === 0 ? (
          <PanelMessage>
            <span className="block font-medium text-foreground">
              {m.email_sendouts_empty_title()}
            </span>
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
    // The link's ::after overlay stretches over the positioned row, so the whole
    // row is one click target while the subject stays the accessible link name.
    <tr className="relative cursor-pointer transition-colors duration-150 hover:bg-surface-secondary has-[a:focus-visible]:bg-surface-secondary">
      <Td>
        <Link
          to="/settings/emails/$sendoutId"
          params={{ sendoutId: sendout.id ?? "" }}
          className="font-medium text-foreground after:absolute after:inset-0 focus-visible:underline focus-visible:outline-none"
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
  const settingsQuery = useQuery(settingsQueryOptions());
  const saved = settingsQuery.data?.email_sendout_rate_per_minute;

  return (
    <Section className="mt-0 mb-6" title={m.email_sendouts_rate()}>
      {saved !== undefined ? (
        <RateForm saved={saved} />
      ) : settingsQuery.error ? (
        <ErrorAlert message={queryErrorMessage(settingsQuery.error)} className="m-4" />
      ) : (
        <RateFormSkeleton />
      )}
    </Section>
  );
}

function RateForm({ saved }: { saved: number }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<number | null>(null);
  const mutation = useMutation({
    mutationFn: (rate: number) => updateSettings({ email_sendout_rate_per_minute: rate }),
    onSuccess: (data) => {
      queryClient.setQueryData(queryKeys.settings(), data);
      void queryClient.invalidateQueries({ queryKey: queryKeys.emailSendoutContext() });
      setDraft(null);
    },
  });

  const dirty = draft !== null && draft !== saved;

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (dirty && !mutation.isPending) mutation.mutate(draft);
      }}
    >
      <div className="p-4">
        <NumberField
          minValue={1}
          maxValue={600}
          value={draft ?? saved}
          onChange={(next) => setDraft(Number.isFinite(next) ? next : null)}
        >
          <Label>{m.email_sendouts_rate_label()}</Label>
          <NumberField.Group className="w-40">
            <NumberField.DecrementButton />
            <NumberField.Input />
            <NumberField.IncrementButton />
          </NumberField.Group>
          <Description>{m.email_sendouts_rate_hint()}</Description>
        </NumberField>
      </div>
      <ErrorAlert
        message={mutation.error ? queryErrorMessage(mutation.error) : ""}
        className="mx-4 mb-4"
      />
      <div className="flex items-center justify-end gap-3 border-t border-separator px-4 py-3">
        {mutation.isSuccess && draft === null ? (
          <span className="text-xs text-muted" role="status">
            {m.email_sendouts_rate_saved()}
          </span>
        ) : null}
        <Button
          type="submit"
          size="sm"
          variant="primary"
          isPending={mutation.isPending}
          isDisabled={mutation.isPending || !dirty}
        >
          {m.email_sendouts_rate_save()}
        </Button>
      </div>
    </form>
  );
}

function RateFormSkeleton() {
  return (
    <div aria-hidden>
      <div className="flex flex-col gap-1.5 p-4">
        <div className="h-4 w-28 animate-pulse rounded bg-surface-secondary" />
        <div className="h-9 w-40 animate-pulse rounded-lg bg-surface-secondary" />
        <div className="h-3 w-72 max-w-full animate-pulse rounded bg-surface-secondary" />
      </div>
      <div className="flex justify-end border-t border-separator px-4 py-3">
        <div className="h-8 w-16 animate-pulse rounded-full bg-surface-secondary" />
      </div>
    </div>
  );
}
