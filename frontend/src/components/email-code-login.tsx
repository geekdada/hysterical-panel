import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "@tanstack/react-form";
import { Button, Input, Label, TextField } from "@heroui/react";
import { loginWithEmailCode, requestEmailCode } from "~/api/auth";
import { localizeApiError } from "~/lib/api-error";
import * as m from "~/paraglide/messages.js";

const RESEND_COOLDOWN_MS = 30_000;

interface SentCode {
  otpId: string;
  email: string;
  sentAt: number;
}

/**
 * Owns the two PocketBase OTP requests. The login page reads `isPending` to
 * disable its other sign-in methods while a request runs.
 */
export function useEmailCodeLogin(onSignedIn: () => void) {
  const [sent, setSent] = useState<SentCode | null>(null);
  const requestMutation = useMutation({
    mutationFn: (email: string) => requestEmailCode(email),
    onSuccess: (otpId, email) => setSent({ otpId, email, sentAt: Date.now() }),
  });
  const verifyMutation = useMutation({
    mutationFn: ({ otpId, code }: { otpId: string; code: string }) =>
      loginWithEmailCode(otpId, code),
    onSuccess: onSignedIn,
  });

  function resetErrors() {
    requestMutation.reset();
    verifyMutation.reset();
  }

  const failure = requestMutation.error ?? verifyMutation.error;
  return {
    sent,
    isPending: requestMutation.isPending || verifyMutation.isPending,
    requesting: requestMutation.isPending,
    verifying: verifyMutation.isPending,
    error:
      failure instanceof Error
        ? localizeApiError(failure.message)
        : failure
          ? m.auth_email_code_failed()
          : "",
    resetErrors,
    requestCode: (email: string) => {
      resetErrors();
      requestMutation.mutate(email);
    },
    verifyCode: (code: string) => {
      if (sent) verifyMutation.mutate({ otpId: sent.otpId, code });
    },
    changeEmail: () => {
      resetErrors();
      setSent(null);
    },
  };
}

type EmailCodeLoginState = ReturnType<typeof useEmailCodeLogin>;

// The countdown stops ticking once the deadline passes.
function useSecondsUntil(deadline: number | null): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (deadline === null) return;
    setNow(Date.now());
    const timer = window.setInterval(() => {
      const current = Date.now();
      setNow(current);
      if (current >= deadline) window.clearInterval(timer);
    }, 1000);
    return () => window.clearInterval(timer);
  }, [deadline]);
  return deadline === null ? 0 : Math.max(0, Math.ceil((deadline - now) / 1000));
}

export function EmailCodeLoginForm({
  state,
  initialEmail,
  durationSeconds,
  disabled,
}: {
  state: EmailCodeLoginState;
  initialEmail: string;
  durationSeconds: number;
  disabled: boolean;
}) {
  const { sent } = state;
  const resendIn = useSecondsUntil(sent ? sent.sentAt + RESEND_COOLDOWN_MS : null);
  const form = useForm({
    defaultValues: { email: initialEmail, code: "" },
    onSubmit: ({ value }) => {
      if (sent) state.verifyCode(value.code);
      else state.requestCode(value.email.trim());
    },
  });

  function resend() {
    if (!sent) return;
    form.setFieldValue("code", "");
    state.requestCode(sent.email);
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        e.stopPropagation();
        void form.handleSubmit();
      }}
      className="flex flex-col gap-4"
    >
      {sent ? (
        <div className="flex flex-col gap-1 text-[13px]">
          <p className="text-foreground">{m.auth_email_code_sent({ email: sent.email })}</p>
          {durationSeconds > 0 && (
            <p className="text-muted">
              {m.auth_email_code_expires({
                minutes: String(Math.max(1, Math.round(durationSeconds / 60))),
              })}
            </p>
          )}
        </div>
      ) : (
        <p className="text-[13px] text-muted">{m.auth_email_code_intro()}</p>
      )}

      {sent ? (
        <form.Field key="code" name="code">
          {(field) => (
            <TextField
              name="code"
              value={field.state.value}
              // PocketBase codes are digits only, so pasted spaces or dashes are dropped.
              onChange={(value) => {
                state.resetErrors();
                field.handleChange(value.replace(/\D/g, ""));
              }}
              onBlur={field.handleBlur}
              isRequired
            >
              <Label>{m.auth_email_code_label()}</Label>
              <Input
                autoFocus
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={71}
                className="font-mono tracking-widest"
              />
            </TextField>
          )}
        </form.Field>
      ) : (
        <form.Field key="email" name="email">
          {(field) => (
            <TextField
              name="email"
              value={field.state.value}
              onChange={(value) => {
                state.resetErrors();
                field.handleChange(value);
              }}
              onBlur={field.handleBlur}
              isRequired
            >
              <Label>{m.auth_email()}</Label>
              <Input type="email" autoComplete="username" />
            </TextField>
          )}
        </form.Field>
      )}

      {state.error && (
        <p className="text-sm text-danger" role="alert">
          {state.error}
        </p>
      )}

      <Button type="submit" variant="primary" fullWidth isDisabled={disabled} className="mt-1">
        {sent
          ? state.verifying
            ? m.auth_signing_in()
            : m.auth_sign_in()
          : state.requesting
            ? m.auth_sending()
            : m.auth_email_code_send()}
      </Button>

      {sent && (
        <div className="-mt-2 flex items-center justify-between gap-3">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="-ml-3 text-[13px] text-muted"
            isDisabled={disabled}
            onPress={() => {
              form.setFieldValue("code", "");
              state.changeEmail();
            }}
          >
            {m.auth_email_code_change_email()}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="-mr-3 text-[13px] text-muted tabular-nums"
            isDisabled={disabled || resendIn > 0}
            onPress={resend}
          >
            {state.requesting
              ? m.auth_sending()
              : resendIn > 0
                ? m.auth_email_code_resend_in({ seconds: String(resendIn) })
                : m.auth_email_code_resend()}
          </Button>
        </div>
      )}
    </form>
  );
}
