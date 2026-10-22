---
status: accepted
---

# An Allowance Window keeps its allowance while in effect

This replaces the allowance-edit rule in ADR-0005. Before, editing a Subscription Type's allowance replaced the current Allowance Window's allowance and erased its Allowance Top-ups. That meant an admin who changed a Type could take away top-ups they had already given a User. Now a window takes the Type's allowance when it starts and keeps it until it ends, whether or not it has counted Traffic. An edit therefore applies only to windows that have not started: later windows of current grants, and queued grants. Top-ups belong to their window and no Type edit removes them.

## Considered options

- Keep the old allowance only in windows that have counted Traffic, and let an edit change a current window with no usage. We rejected it because the result of an edit would depend on each User's usage. With the chosen rule, the window's start time is enough to tell which allowance applies.
- Fix the allowance per User Subscription, so only new grants get the new value. We rejected it because a 30-day Type would keep the old allowance for up to eleven more windows. Admins can already create a new Type when existing grants must keep their terms.
- Apply increases at once and defer only decreases. We rejected it because the rule stops being symmetric. An admin who wants to relieve an exhausted User now can add a top-up.

## Consequences

- A Type edit never changes a window in effect, so the edit no longer Kicks anyone.
- Subscription Reschedule moves the window's allowance together with its usage and top-ups, so a reschedule still never changes access.
- The top-up dialog no longer prefills the Type allowance, because the window's allowance and the Type's current value may differ. The admin enters an amount each time.
