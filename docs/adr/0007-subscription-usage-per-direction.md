---
status: accepted
---

# Record subscription usage per direction for display only

A User Subscription stores Allowance Window usage as separate tx and rx byte counts instead of one combined count. The allowance, remaining amount, over-allowance state and exhaustion Kicks still use tx + rx. We considered letting a Subscription Type meter one direction, weight the two, or give each direction its own allowance. We rejected all three: each changes Type configuration, Allowance Top-up semantics and the Kick trigger, and no current requirement needs it.

Each grant also keeps Grant Usage, the tx and rx summed over every window of the grant. Window rollover resets window usage to zero but does not reset Grant Usage, and Allowance Top-ups and Type allowance edits do not change it. Past grants show Grant Usage in the user detail history. The stored window usage would only show the last window that received Traffic, which for a 30-day Type is at most one twelfth of the grant.

## Consequences

- The user list replaces lifetime User Traffic (`used_tx`/`used_rx`) with current window usage in three columns: used / allowance, tx, rx. A Legacy Unmetered User has no window, so their row shows the exemption marker and their lifetime tx and rx; the list returns `used_tx`/`used_rx` only for those rows. Lifetime totals remain on the user detail endpoint and the Management API. The list no longer sorts by lifetime usage. It sorts by current window used, tx or rx instead, computing the current window in SQL. Users without a current grant, including Legacy Unmetered Users, follow in either direction ordered by creation, because their row has no window to rank.
- The API keeps `used_bytes` as the server-computed sum, so clients do not add the two directions themselves.
- A new migration replaces `used_bytes` with `used_tx_bytes`, `used_rx_bytes`, `grant_tx_bytes` and `grant_rx_bytes`. The feature had not launched, so the migration discards existing window usage.
