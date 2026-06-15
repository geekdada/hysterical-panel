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
  "invalid login credentials.": () => m.error_invalid_login_credentials(),
  "failed to authenticate.": () => m.error_failed_to_authenticate(),
  "failed to authenticate": () => m.error_failed_to_authenticate(),
  "the email is invalid or already in use.": () => m.error_email_invalid_or_taken(),
  "the email is invalid or already in use": () => m.error_email_invalid_or_taken(),
  "not found": () => m.error_not_found(),
};

export function localizeApiError(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) return trimmed;
  const mapped = API_ERROR_MAP[trimmed.toLowerCase()];
  return mapped ? mapped() : trimmed;
}
