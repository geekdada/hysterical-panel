import * as m from "~/paraglide/messages.js";

const API_ERROR_MAP: Record<string, () => string> = {
  "account is disabled": () => m.error_account_disabled(),
  "email not verified": () => m.error_email_not_verified(),
  "invalid token": () => m.error_invalid_token(),
  "invalid auth": () => m.error_invalid_auth(),
  "passkeys are not configured": () => m.error_passkeys_not_configured(),
  "passkey authentication failed": () => m.error_passkey_auth_failed(),
  "password login is disabled for accounts with passkeys": () => m.error_password_login_disabled(),
  "active account required": () => m.error_active_account_required(),
  "user not found": () => m.error_user_not_found(),
  "passkey not found": () => m.error_passkey_not_found(),
  "invalid body": () => m.error_invalid_body(),
  "invitations are disabled": () => m.error_invitations_disabled(),
  "node not found": () => m.error_node_not_found(),
  "invitation not found": () => m.error_invitation_not_found(),
  "invalid login credentials": () => m.error_invalid_login_credentials(),
  "failed to authenticate": () => m.error_failed_to_authenticate(),
  "the email is invalid or already in use": () => m.error_email_invalid_or_taken(),
  "not found": () => m.error_not_found(),
  "email sendouts are unavailable: smtp is not configured": () => m.error_email_smtp_unavailable(),
  "no users are active and verified": () => m.error_email_no_recipients(),
  "recipient must be active and verified": () => m.error_email_recipient_ineligible(),
  "recipient not found": () => m.error_email_recipient_ineligible(),
  "email sendout is cancelled": () => m.error_email_sendout_cancelled(),
  "email sendout has no pending recipients": () => m.error_email_nothing_to_cancel(),
  "subject must be between 1 and 200 characters": () => m.error_email_subject_invalid(),
  "subject cannot contain control characters": () => m.error_email_subject_invalid(),
  "email content is too large": () => m.error_email_content_too_large(),
  "content is too large": () => m.error_email_content_too_large(),
  "email_sendout_rate_per_minute must be between 1 and 600": () => m.error_email_rate_invalid(),
};

export function localizeApiError(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) return trimmed;
  // PocketBase capitalises API error messages and appends a period.
  const mapped = API_ERROR_MAP[trimmed.toLowerCase().replace(/\.$/, "")];
  return mapped ? mapped() : trimmed;
}
