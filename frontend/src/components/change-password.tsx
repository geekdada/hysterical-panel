import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useForm } from "@tanstack/react-form";
import { Button, FieldError, Input, Label, Modal, TextField, toast } from "@heroui/react";
import { listPasskeys } from "~/api/auth";
import {
  canQueryPanelApi,
  changeOwnPassword,
  fetchPanelConfigQuery,
  queryErrorMessage,
  queryKeys,
} from "~/api/queries";
import { SettingsSectionHeader } from "~/components/settings-nav";
import { ErrorAlert } from "~/components/ui";
import * as m from "~/paraglide/messages.js";

/** Self-service password change for the signed-in user. */
export function PasswordSection({ userId }: { userId: string }) {
  const [open, setOpen] = useState(false);
  const hasPasskeys = useHasPasskeys(userId);

  return (
    <>
      <SettingsSectionHeader id="password" title={m.settings_password_title()} />

      <div className="flex flex-col gap-3 rounded-lg border bg-surface px-5 py-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-[13px] text-muted">
          {hasPasskeys ? m.settings_password_passkey_note() : m.settings_password_row()}
        </p>
        <div className="shrink-0">
          <Button size="sm" variant="secondary" onPress={() => setOpen(true)}>
            {m.settings_password_change()}
          </Button>
        </div>
      </div>

      {open && <ChangePasswordModal userId={userId} onClose={() => setOpen(false)} />}
    </>
  );
}

// Shares the passkeys section's cache, so it costs no extra request.
function useHasPasskeys(userId: string): boolean {
  const configQuery = useQuery({
    queryKey: queryKeys.config(),
    queryFn: fetchPanelConfigQuery,
    enabled: canQueryPanelApi(),
    staleTime: Infinity,
  });
  const enabled = configQuery.data?.passkeys_enabled ?? false;
  const passkeysQuery = useQuery({
    queryKey: queryKeys.userPasskeys(userId),
    queryFn: () => listPasskeys(userId),
    enabled: canQueryPanelApi() && enabled,
  });
  return enabled && (passkeysQuery.data?.length ?? 0) > 0;
}

// Mounted only while open, so every opening starts with an empty form.
function ChangePasswordModal({ userId, onClose }: { userId: string; onClose: () => void }) {
  const mutation = useMutation({
    mutationFn: (value: { password: string; passwordConfirm: string }) =>
      changeOwnPassword(userId, value.password, value.passwordConfirm),
    onSuccess: () => {
      toast.success(m.settings_password_changed());
      onClose();
    },
  });

  const form = useForm({
    defaultValues: { password: "", passwordConfirm: "" },
    onSubmit: async ({ value }) => {
      mutation.reset();
      await mutation.mutateAsync(value).catch(() => undefined);
    },
  });

  const error = mutation.error
    ? queryErrorMessage(mutation.error, m.error_password_change_network())
    : "";

  return (
    <Modal.Backdrop isOpen onOpenChange={(next) => !next && onClose()}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void form.handleSubmit();
            }}
          >
            <Modal.Header>
              <Modal.Heading>{m.settings_password_change()}</Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-muted">
                {m.settings_password_change_body()}
              </p>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-4">
              <form.Field
                name="password"
                validators={{
                  onChange: ({ value }) => (value ? undefined : m.settings_password_required()),
                }}
              >
                {(field) => (
                  <TextField
                    name="new-password"
                    type="password"
                    value={field.state.value}
                    onChange={field.handleChange}
                    onBlur={field.handleBlur}
                    isInvalid={field.state.meta.errors.length > 0}
                    isRequired
                  >
                    <Label>{m.auth_new_password()}</Label>
                    <Input autoComplete="new-password" />
                    <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                  </TextField>
                )}
              </form.Field>

              <form.Field
                name="passwordConfirm"
                validators={{
                  onChangeListenTo: ["password"],
                  onChange: ({ value, fieldApi }) =>
                    value === fieldApi.form.getFieldValue("password")
                      ? undefined
                      : m.settings_password_mismatch(),
                }}
              >
                {(field) => (
                  <TextField
                    name="confirm-new-password"
                    type="password"
                    value={field.state.value}
                    onChange={field.handleChange}
                    onBlur={field.handleBlur}
                    isInvalid={field.state.meta.errors.length > 0}
                    isRequired
                  >
                    <Label>{m.auth_confirm_new_password()}</Label>
                    <Input autoComplete="new-password" />
                    <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                  </TextField>
                )}
              </form.Field>

              <ErrorAlert message={error} />
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {m.common_cancel()}
              </Button>
              <form.Subscribe
                selector={(s) => ({ canSubmit: s.canSubmit, isSubmitting: s.isSubmitting })}
              >
                {({ canSubmit, isSubmitting }) => (
                  <Button
                    type="submit"
                    variant="primary"
                    isPending={isSubmitting}
                    isDisabled={!canSubmit || isSubmitting}
                  >
                    {isSubmitting ? m.settings_password_submitting() : m.settings_password_submit()}
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
