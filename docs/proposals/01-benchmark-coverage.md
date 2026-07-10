# Proposal: token-bucket benchmark coverage

Related issue: #1

This proposal adds reproducible Go benchmarks for the HTTP validation boundary
and the Redis-backed decision path. The benchmark suite should report both
throughput and allocations for allowed, denied, and weighted-cost decisions.

Redis-backed benchmarks must be opt-in so ordinary unit tests remain fast and
deterministic. Results should include the Redis version, policy, concurrency,
and command used to make comparisons meaningful during future optimizations.
