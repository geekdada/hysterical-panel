import { Chip } from "@heroui/react";
import type { RecipientStatus } from "~/api/queries";
import * as m from "~/paraglide/messages.js";

export const RECIPIENT_STATUSES: RecipientStatus[] = [
  "pending",
  "sending",
  "sent",
  "failed",
  "skipped",
  "cancelled",
];

export function languageLabel(language: string): string {
  return language === "zh-cn" ? m.email_language_zh_cn() : m.email_language_en();
}

export function audienceLabel(audience: string): string {
  return audience === "all" ? m.email_audience_all() : m.email_audience_single();
}

export function recipientStatusLabel(status: string): string {
  const labels: Record<string, () => string> = {
    pending: m.email_recipient_status_pending,
    sending: m.email_recipient_status_sending,
    sent: m.email_recipient_status_sent,
    failed: m.email_recipient_status_failed,
    skipped: m.email_recipient_status_skipped,
    cancelled: m.email_recipient_status_cancelled,
  };
  return (labels[status] ?? m.common_unknown)();
}

export function reasonLabel(reason: string | undefined): string {
  if (!reason) return "";
  const labels: Record<string, () => string> = {
    delivery_failed: m.email_reason_delivery_failed,
    smtp_disabled: m.email_reason_smtp_disabled,
    interrupted: m.email_reason_interrupted,
    user_ineligible: m.email_reason_user_ineligible,
    user_deleted: m.email_reason_user_deleted,
  };
  return (labels[reason] ?? m.common_unknown)();
}

export function SendoutStatusChip({ status }: { status: string | undefined }) {
  const tone = status === "sending" ? "accent" : status === "completed" ? "success" : "default";
  const label =
    status === "sending"
      ? m.email_status_sending()
      : status === "completed"
        ? m.email_status_completed()
        : m.email_status_cancelled();
  return (
    <Chip size="sm" variant="soft" color={tone}>
      {label}
    </Chip>
  );
}

export function RecipientStatusChip({ status }: { status: string | undefined }) {
  const tone =
    status === "sent"
      ? "success"
      : status === "failed"
        ? "danger"
        : status === "pending" || status === "sending"
          ? "accent"
          : "default";
  return (
    <Chip size="sm" variant="soft" color={tone}>
      {recipientStatusLabel(status ?? "")}
    </Chip>
  );
}
