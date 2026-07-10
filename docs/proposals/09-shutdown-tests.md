# Proposal: graceful-shutdown integration tests

Related issue: #9

This proposal adds deterministic integration coverage for SIGTERM handling.
The test should run the real HTTP server with a controllable slow dependency,
send a signal while a request is in flight, and verify that the shutdown
deadline is honored without admitting new work after shutdown begins.

The test must stay portable on Linux CI runners and avoid timing-only sleeps;
explicit channels or barriers should coordinate the request lifecycle.
