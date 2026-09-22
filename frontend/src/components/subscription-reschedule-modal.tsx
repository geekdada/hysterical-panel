import { useState } from "react";
import { Button, DateField, FieldError, Label, Modal } from "@heroui/react";
import { fromAbsolute, type DateValue } from "@internationalized/date";
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
  const [value, setValue] = useState<DateValue | null>(() => fromAbsolute(oldStart, timeZone));
  const startMs = value ? value.toDate(timeZone).getTime() : null;
  const problem = startMs === null ? null : rescheduleProblem(startMs, Date.now(), previousEnd);
  const format = (ms: number) => formatLocaleDateTime(ms, undefined, timeZone);
  const shift = startMs === null ? 0 : startMs - oldStart;
  const canSubmit = startMs !== null && problem === null && !pending;

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (canSubmit) onConfirm(new Date(startMs).toISOString());
            }}
          >
            <Modal.Header>
              <Modal.Heading>{m.subscription_reschedule()}</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-3">
                <DateField
                  fullWidth
                  value={value}
                  onChange={setValue}
                  granularity="second"
                  hideTimeZone
                  isRequired
                  isInvalid={startMs === null || problem !== null}
                  isDisabled={pending}
                >
                  <Label>{m.subscription_reschedule_start()}</Label>
                  <DateField.Group>
                    <DateField.Input>
                      {(segment) => <DateField.Segment segment={segment} />}
                    </DateField.Input>
                  </DateField.Group>
                  {startMs === null ? (
                    <FieldError>{m.subscription_reschedule_incomplete()}</FieldError>
                  ) : problem ? (
                    <FieldError>
                      {PROBLEM_MESSAGES[problem](previousEnd === null ? "" : format(previousEnd))}
                    </FieldError>
                  ) : null}
                </DateField>
                {startMs !== null && problem === null ? (
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
                ) : null}
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
              <Button type="submit" variant="primary" isDisabled={!canSubmit} isPending={pending}>
                {pending ? m.subscription_reschedule_pending() : m.subscription_reschedule()}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
