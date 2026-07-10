# Drive monitoring from independent Observations

The Collector persists generic timestamped Observation intervals, while a separate Monitor evaluator reads those points through each Monitor's evaluation window and owns Alert state transitions. Monitor kinds use strictly validated typed JSON configuration, so future dimensions can be added without coupling evaluators to one another or widening the Monitor table for every new condition. Missing points reduce coverage, a completely empty window is unknown and preserves existing Alert state, and no Monitor infers another Monitor's condition.

## Consequences

Offline and high-traffic evaluation share scheduling, Alert history, and Notification delivery without sharing condition semantics. Observations are retained for 25 hours, resolved or cancelled Alerts for 30 days, and Notification delivery is best effort without retries or reminders.
