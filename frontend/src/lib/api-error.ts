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
  "no account matches this sign-in": () => m.error_no_account_for_oauth(),
  "failed to fetch oauth2 token": () => m.error_oauth_exchange_failed(),
  "failed to fetch oauth2 user": () => m.error_oauth_exchange_failed(),
  "this social account is linked to another user": () => m.error_oauth_linked_to_other_user(),
  "invalid or expired otp": () => m.error_email_code_invalid(),
  "too many attempts, please try again later with a new otp": () => m.error_email_code_too_many(),
  "the collection is not configured to allow otp authentication": () =>
    m.error_email_code_disabled(),
  "email sign-in is unavailable: smtp is not configured": () =>
    m.error_email_code_smtp_unavailable(),
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
  "connected app name must be between 1 and 128 characters": () =>
    m.error_oidc_client_name_invalid(),
  "connected app name cannot contain control characters": () => m.error_oidc_client_name_invalid(),
  "connected app not found": () => m.error_oidc_client_not_found(),
  "redirect_uris must contain between 1 and 10 entries": () => m.error_oidc_redirect_uris_count(),
  "redirect uri must be an absolute https url, or http on a loopback host, with a lowercase ascii host and no spaces, fragment or credentials":
    () => m.error_oidc_redirect_uri_invalid(),
  "redirect uris must be unique": () => m.error_oidc_redirect_uris_duplicate(),
  "allowed_scopes must include openid and only contain openid or email": () =>
    m.error_oidc_scopes_invalid(),
  "oidc provider requires an https panel_backend_url_base and panel_frontend_url_base": () =>
    m.error_oidc_unavailable(),
};

export function localizeApiError(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) return trimmed;
  // PocketBase capitalises API error messages and appends a period.
  const mapped = API_ERROR_MAP[trimmed.toLowerCase().replace(/\.$/, "")];
  return mapped ? mapped() : trimmed;
}
