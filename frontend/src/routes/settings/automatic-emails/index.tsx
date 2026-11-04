import { useForm } from "@tanstack/react-form";
import { Link, createFileRoute } from "@tanstack/react-router";
import { Alert, Description, Label, NumberField, Switch } from "@heroui/react";
import { Pencil } from "@gravity-ui/icons";
import {
  automaticEmailsQueryOptions,
  queryErrorMessage,
  useAutomaticEmails,
  useUpdateAutomaticEmailKind,
  type AutomaticEmailKind,
  type AutomaticEmailsResponse,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  automaticKindDescription,
  automaticKindLabel,
  isAutomaticEmailKind,
} from "~/components/email-sendouts";
import { BrandLink, ErrorAlert, PageShell, Section, TableSkeleton } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import * as m from "~/paraglide/messages.js";

const MIN_THRESHOLD_PERCENT = 1;
const MAX_THRESHOLD_PERCENT = 99;
const DEFAULT_THRESHOLD_PERCENT = 10;

type KindSummary = AutomaticEmailsResponse["kinds"][number] & { kind: AutomaticEmailKind };

export const Route = createFileRoute("/settings/automatic-emails/")({
  loader: ({ context }) => {
    markResponsePrivate();
    return context.queryClient.ensureQueryData(automaticEmailsQueryOptions());
  },
  component: AutomaticEmailsPage,
});

function AutomaticEmailsPage() {
  const { auth } = Route.useRouteContext();
  const listQuery = useAutomaticEmails();
  const kinds = (listQuery.data?.kinds ?? []).filter((row): row is KindSummary =>
    isAutomaticEmailKind(row.kind ?? "")
  );

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <h1 className="text-base font-semibold tracking-tight">{m.automatic_emails_title()}</h1>
      <p className="mt-0.5 mb-6 max-w-2xl text-[13px] text-muted">{m.automatic_emails_desc()}</p>

      {listQuery.data && !listQuery.data.smtp_enabled ? (
        <Alert status="warning" className="mb-4">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{m.automatic_emails_smtp_off()}</Alert.Title>
          </Alert.Content>
        </Alert>
      ) : null}
      <ErrorAlert
        message={listQuery.error ? queryErrorMessage(listQuery.error) : ""}
        className="mb-4"
      />

      <Section className="mt-0" title={m.automatic_emails_section()}>
        {listQuery.isPending ? (
          <TableSkeleton rows={3} />
        ) : (
          <div className="divide-y divide-separator">
            {kinds.map((row) => (
              <KindRow key={row.kind} row={row} />
            ))}
          </div>
        )}
      </Section>
    </PageShell>
  );
}

function KindRow({ row }: { row: KindSummary }) {
  const mutation = useUpdateAutomaticEmailKind(row.kind);
  const enabled = row.enabled ?? false;
  const hasTemplate = row.has_template ?? false;
  const label = automaticKindLabel(row.kind);

  return (
    <div className="flex flex-col gap-3 px-4 py-3.5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium text-foreground">{label}</p>
          <p className="mt-0.5 text-xs text-muted">{automaticKindDescription(row.kind)}</p>
          <p className="mt-1 truncate text-xs text-muted">
            {hasTemplate ? row.subject : m.automatic_emails_no_template()}
          </p>
        </div>
        <div className="flex shrink-0 items-center justify-between gap-4">
          <Link
            to="/settings/automatic-emails/$kind"
            params={{ kind: row.kind }}
            className="inline-flex items-center gap-1.5 rounded-sm text-[13px] font-medium text-accent hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
          >
            <Pencil className="size-3.5" aria-hidden />
            {m.automatic_emails_edit_template()}
          </Link>
          <Switch
            aria-label={label}
            isSelected={enabled}
            isDisabled={!hasTemplate || mutation.isPending}
            onChange={(next) => mutation.mutate({ enabled: next })}
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <span className="text-xs text-muted">
                {enabled ? m.automatic_emails_on() : m.automatic_emails_off()}
              </span>
            </Switch.Content>
          </Switch>
        </div>
      </div>
      {hasTemplate ? null : (
        <p className="text-xs text-muted">{m.automatic_emails_needs_template()}</p>
      )}
      {row.kind === "low_allowance" ? (
        <ThresholdField saved={row.threshold_percent ?? DEFAULT_THRESHOLD_PERCENT} />
      ) : null}
      <ErrorAlert message={mutation.error ? queryErrorMessage(mutation.error) : ""} />
    </div>
  );
}

// React Aria commits a NumberField value on blur or Enter, not per keystroke,
// so saving in onChange saves when the admin leaves the field. The stepper
// buttons are left out because each press would commit and save on its own.
function ThresholdField({ saved }: { saved: number }) {
  const mutation = useUpdateAutomaticEmailKind("low_allowance");
  const form = useForm({ defaultValues: { threshold: saved } });

  function commit(next: number) {
    const value = Number.isFinite(next) ? next : saved;
    form.setFieldValue("threshold", value);
    if (value !== saved) mutation.mutate({ threshold_percent: value });
  }

  return (
    <div>
      <form.Field name="threshold">
        {(field) => (
          <NumberField
            minValue={MIN_THRESHOLD_PERCENT}
            maxValue={MAX_THRESHOLD_PERCENT}
            step={1}
            value={field.state.value}
            onChange={commit}
          >
            <Label>{m.automatic_emails_threshold()}</Label>
            <NumberField.Group className="w-24">
              <NumberField.Input />
            </NumberField.Group>
            <Description>{m.automatic_emails_threshold_hint()}</Description>
          </NumberField>
        )}
      </form.Field>
      <ErrorAlert
        message={mutation.error ? queryErrorMessage(mutation.error) : ""}
        className="mt-2"
      />
    </div>
  );
}
