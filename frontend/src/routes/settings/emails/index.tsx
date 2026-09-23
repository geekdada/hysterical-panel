import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Button, Label, NumberField } from "@heroui/react";
import { Plus } from "@gravity-ui/icons";
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
      <p className="px-4 pb-4 text-xs text-muted">{m.email_sendouts_rate_hint()}</p>
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

  return (
    <>
      <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-end">
        <NumberField
          className="w-40"
          minValue={1}
          maxValue={600}
          value={draft ?? saved}
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
      <ErrorAlert
        message={mutation.error ? queryErrorMessage(mutation.error) : ""}
        className="mx-4 mb-4"
      />
    </>
  );
}

function RateFormSkeleton() {
  return (
    <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-end" aria-hidden>
      <div className="flex w-40 flex-col gap-1.5">
        <div className="h-4 w-28 animate-pulse rounded bg-surface-secondary" />
        <div className="h-9 animate-pulse rounded-lg bg-surface-secondary" />
      </div>
      <div className="h-8 w-20 animate-pulse rounded-full bg-surface-secondary" />
    </div>
  );
}
