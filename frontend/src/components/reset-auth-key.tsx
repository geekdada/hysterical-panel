import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button, Modal } from "@heroui/react";
import type { components } from "~/api/schema";
import { queryErrorMessage, queryKeys, resetUserAuthString } from "~/api/queries";
import { CopyButton, Dot } from "~/components/ui";
import * as m from "~/paraglide/messages.js";

type PanelUser = components["schemas"]["PanelUser"];

/** Reset button plus the confirm modal that reveals the new key once. */
export function ResetAuthKeyButton({ userId }: { userId: string }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<PanelUser | null>(null);

  const mutation = useMutation({
    mutationFn: () => resetUserAuthString(userId),
    onSuccess: (updated) => {
      setResult(updated);
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.all, "users"] });
    },
  });

  const error = mutation.error
    ? queryErrorMessage(mutation.error, m.error_user_reset_auth_network())
    : "";

  function handleOpenChange(next: boolean) {
    setOpen(next);
    if (!next) {
      mutation.reset();
      setResult(null);
    }
  }

  return (
    <>
      <Button
        size="sm"
        variant="secondary"
        onPress={() => handleOpenChange(true)}
        className="border-danger text-danger hover:bg-danger-soft"
      >
        {m.user_manage_reset_auth_key()}
      </Button>

      <ResetAuthKeyModal
        isOpen={open}
        onOpenChange={handleOpenChange}
        pending={mutation.isPending}
        error={result ? "" : error}
        result={result}
        onConfirm={() => mutation.mutate()}
      />
    </>
  );
}

function ResetAuthKeyModal({
  isOpen,
  onOpenChange,
  pending,
  error,
  result,
  onConfirm,
}: {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  result: PanelUser | null;
  onConfirm: () => void;
}) {
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{m.user_manage_reset_confirm_title()}</Modal.Heading>
            <p className="mt-1.5 text-sm leading-5 text-muted">
              {m.user_manage_reset_confirm_body()}
            </p>
          </Modal.Header>
          <Modal.Body>
            {error && (
              <p className="text-[13px] text-danger" role="alert">
                {error}
              </p>
            )}

            {result && (
              <div className="rounded-lg border bg-surface-secondary p-3">
                <div className="flex items-center gap-2 text-[13px]">
                  <Dot tone="ok" />
                  <span className="font-medium">{m.user_manage_reset_done()}</span>
                </div>
                <p className="mt-1 text-xs text-muted">{m.user_manage_reset_new_key_hint()}</p>
                {result.auth_string && (
                  <div className="group/key mt-2 flex items-center gap-1.5">
                    <span className="min-w-0 truncate font-mono text-[12px] text-muted">
                      {result.auth_string}
                    </span>
                    <CopyButton value={result.auth_string} label={m.common_copy_auth_key()} />
                  </div>
                )}
              </div>
            )}
          </Modal.Body>
          <Modal.Footer>
            <Button slot="close" variant="secondary">
              {result ? m.common_cancel() : m.user_manage_reset_cancel()}
            </Button>
            {!result && (
              <Button variant="danger" isPending={pending} isDisabled={pending} onPress={onConfirm}>
                {pending ? m.user_manage_reset_submitting() : m.user_manage_reset_submit()}
              </Button>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
