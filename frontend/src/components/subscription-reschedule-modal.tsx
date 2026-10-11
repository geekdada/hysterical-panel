import { Button, DateField, FieldError, Label, Modal } from "@heroui/react";
import { fromAbsolute, type DateValue } from "@internationalized/date";
import { useForm } from "@tanstack/react-form";
import type { UserSubscription } from "~/api/queries";
import { formatLocaleDateTime } from "~/lib/format";
import { GRANT_MS, rescheduleProblem, type RescheduleProblem } from "~/lib/subscription-reschedule";
import * as m from "~/paraglide/messages.js";

const PROBLEM_MESSAGES: Record<RescheduleProblem, (previousEnd: string) => string> = {
  future: () => m.subscription_reschedule_future(),
  ended: () => m.subscription_reschedule_ended(),
  overlap: (end) => m.subscription_reschedule_overlap({ end }),
};

export function RescheduleModal({
  isOpen,
  item,
  queued,
  previousEnd,
  timeZone,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: {
  isOpen: boolean;
  item: UserSubscription;
  queued?: UserSubscription;
  previousEnd: number | null;
  timeZone: string;
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: (startsAt: string) => void;
}) {
  const oldStart = Date.parse(item.starts_at);
  const format = (ms: number) => formatLocaleDateTime(ms, undefined, timeZone);
  const toMs = (value: DateValue | null) => (value ? value.toDate(timeZone).getTime() : null);

  function startError(value: DateValue | null) {
    const startMs = toMs(value);
    if (startMs === null) return m.subscription_reschedule_incomplete();
    const problem = rescheduleProblem(startMs, Date.now(), previousEnd);
    if (!problem) return undefined;
    return PROBLEM_MESSAGES[problem](previousEnd === null ? "" : format(previousEnd));
  }

  const form = useForm({
    defaultValues: {
      startsAt: fromAbsolute(oldStart, timeZone) as DateValue | null,
    },
    onSubmit: ({ value }) => {
      const startMs = toMs(value.startsAt);
      if (startMs === null || pending) return;
      onConfirm(new Date(startMs).toISOString());
    },
  });

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              event.stopPropagation();
              void form.handleSubmit();
            }}
            onKeyDown={(event) => {
              // Date segments are contentEditable spinbuttons, so the browser
              // never submits the form implicitly on Enter.
              const target = event.target as HTMLElement;
              if (event.key !== "Enter" || target.getAttribute("role") !== "spinbutton") return;
              event.preventDefault();
              event.currentTarget.requestSubmit();
            }}
          >
            <Modal.Header>
              <Modal.Heading>{m.subscription_reschedule()}</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-3">
                <form.Field
                  name="startsAt"
                  validators={{
                    onChange: ({ value }) => startError(value),
                    onSubmit: ({ value }) => startError(value),
                  }}
                >
                  {(field) => {
                    const fieldError = field.state.meta.errors[0];
                    const startMs = toMs(field.state.value);
                    return (
                      <>
                        <DateField
                          fullWidth
                          value={field.state.value}
                          onChange={field.handleChange}
                          onBlur={field.handleBlur}
                          granularity="second"
                          hideTimeZone
                          isRequired
                          isInvalid={Boolean(fieldError)}
                          isDisabled={pending}
                        >
                          <Label>{m.subscription_reschedule_start()}</Label>
                          <DateField.Group>
                            <DateField.Input>
                              {(segment) => <DateField.Segment segment={segment} />}
                            </DateField.Input>
                          </DateField.Group>
                          {fieldError ? <FieldError>{fieldError}</FieldError> : null}
                        </DateField>
                        {startMs !== null && !fieldError ? (
                          <RescheduleHints
                            startMs={startMs}
                            oldStart={oldStart}
                            queued={queued}
                            format={format}
                          />
                        ) : null}
                      </>
                    );
                  }}
                </form.Field>
                {error ? (
                  <p className="text-[13px] text-danger" role="alert">
                    {error}
                  </p>
                ) : null}
              </div>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary" isDisabled={pending}>
                {m.common_cancel()}
              </Button>
              <form.Subscribe selector={(state) => state.canSubmit}>
                {(canSubmit) => (
                  <Button
                    type="submit"
                    variant="primary"
                    isDisabled={!canSubmit || pending}
                    isPending={pending}
                  >
                    {pending ? m.subscription_reschedule_pending() : m.subscription_reschedule()}
                  </Button>
                )}
              </form.Subscribe>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}

function RescheduleHints({
  startMs,
  oldStart,
  queued,
  format,
}: {
  startMs: number;
  oldStart: number;
  queued?: UserSubscription;
  format: (ms: number) => string;
}) {
  const shift = startMs - oldStart;
  return (
    <div className="flex flex-col gap-1 text-xs leading-5 text-muted">
      <p>{m.subscription_reschedule_hint({ end: format(startMs + GRANT_MS) })}</p>
      {queued ? (
        <p>
          {m.subscription_reschedule_queued_hint({
            start: format(Date.parse(queued.starts_at) + shift),
            end: format(Date.parse(queued.ends_at) + shift),
          })}
        </p>
      ) : null}
    </div>
  );
}
