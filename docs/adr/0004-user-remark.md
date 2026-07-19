# Per-user admin remark

Add a plain-text `remark` field to the `users` collection so administrators can store a short private note on each account. It is admin-only, exposed only on the user detail endpoint, and capped at 2000 characters.

## Consequences

- Remarks are readable and writable only by admins through the existing `GET/PATCH /api/panel/users/{id}` endpoints. The response shape for these endpoints remains `UserDetail`; `PanelUser` is unchanged, so user list, registration, and auth-key-reset responses do not carry the field.
- The field is a non-required `TextField` with `Max: 2000`. Newlines are preserved; no markup or HTML is supported.
- On the user detail page, the remark lives inside the existing **Manage** section as a 2-3 line textarea with an explicit Save button. The button is disabled until the draft differs from the saved value.
- A new migration adds the column; rolling it back removes the column. Because the migration changes the migration count, the monitoring-language test was updated to roll back four migrations instead of three when reverting to the pre-notification-language schema.
