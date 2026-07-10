# Proposal: Redis Cluster integration coverage

Related issue: #3

This proposal adds an opt-in integration path for Redis Cluster-compatible
deployments. The implementation should document hash-slot behavior for the
single bucket key and verify that concurrent callers still receive exactly the
configured number of admissions.

CI should keep the standalone Redis test as the fast default and run the
cluster topology only in a dedicated job or manually requested workflow.
