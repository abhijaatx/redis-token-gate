# Proposal: API contract fixtures

Related issue: #10

This proposal publishes canonical request and response fixtures for success,
denial, authentication failure, malformed JSON, and Redis outage cases. Each
fixture should show the JSON body, status, and relevant rate-limit headers so
client authors can implement the contract without guessing.

An automated contract test should validate the fixtures against the handler,
and the README examples should link to the canonical source to prevent drift.
