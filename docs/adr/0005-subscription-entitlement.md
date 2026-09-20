---
status: accepted
---

# Gate node access with subscriptions, not payments

Node Client Auth requires a usable User Subscription in addition to the existing Status and Verified checks. This replaces the earlier decision to exclude subscriptions. It does not introduce payments or billing. Preexisting Users keep a limited exemption under ADR-0006.

Each User Subscription keeps a reference to its Subscription Type. Editing the Type's allowance sets the current window's allowance to the new amount, even if an admin had previously topped it up. The reset interval cannot change once the Type has been assigned. Admins can create another Type when existing grants must keep their terms. A Type with any grant in its history can be hidden, but cannot be deleted.

The Collector can only enforce the allowance after reading Node counters. Traffic may exceed the allowance before the panel rejects new connections and sends best-effort Kicks. The existing User lifetime totals continue independently of subscription windows. See [subscription design](../subscriptions.md) for the agreed behavior.
