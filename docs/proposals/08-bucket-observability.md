# Proposal: bucket lifecycle observability

Related issue: #8

This proposal improves visibility into bucket expiry, refill behavior, and
decision latency without creating high-cardinality metrics. Any new labels
must be bounded and must never contain raw identities or hashed bucket keys.

The implementation should add dashboard and alert examples for Redis errors,
latency, and sustained denials while keeping the existing Prometheus endpoint
compatible with current scrapers.
