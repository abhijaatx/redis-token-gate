# Proposal: optional OpenTelemetry tracing

Related issue: #4

This proposal adds optional spans around HTTP validation, Redis latency, and
the final admission outcome. Tracing must be disabled by default and should use
the existing request ID for correlation without duplicating sensitive data.

The design should explicitly prohibit raw identities, hashed keys, and bearer
tokens from span attributes. Exporter and sampling configuration belongs in
deployment documentation rather than in the rate-limit policy itself.
