---
status: accepted
---

# Grandfather existing Users until their first subscription

Migrating an installation with existing Users directly to mandatory subscriptions would interrupt every User's node access before admins could assign grants. Set `subscription_required=false` only for Users already present when this feature is introduced; all newly created Users start with it set to true. A legacy unmetered User may continue connecting without a subscription while the existing Status and Verified gates still apply, and their Traffic continues to accumulate in User totals. Before the first grant, show an exemption marker in the existing attributes area of the User detail page, without adding a subscription section or a marker to the User list.

The first successful grant begins immediately and sets `subscription_required=true` in the same operation. This change is permanent: later exhaustion, expiry, cancellation, or termination cannot restore legacy unlimited access. The field represents a lasting node-access policy rather than migration completion, so it must not be exposed as an editable User setting. See [subscription design](../subscriptions.md) for the grant and display rules.
