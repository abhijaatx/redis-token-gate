# Proposal: explicit outage-policy integration guidance

Related issue: #7

This proposal documents how callers should handle a `503` when Redis cannot
answer. It should cover bounded exponential backoff, circuit breakers, and the
trade-off between fail-closed and an explicitly caller-owned fail-open path.

The limiter itself must continue returning `503` rather than silently changing
behavior during an outage. Examples should make retry budgets and idempotency
requirements visible to application teams.
