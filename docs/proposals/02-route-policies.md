# Proposal: per-route rate-limit policies

Related issue: #2

This proposal introduces a stable way to select capacity, refill, and maximum
cost by operation or route. The selection order must be explicit, with a
documented default policy for callers that do not provide a route policy.

Each decision must still map to one Redis key and one atomic Lua invocation.
Configuration errors should fail at startup, while an unknown route should
return a clear client error rather than silently using an unexpected policy.
