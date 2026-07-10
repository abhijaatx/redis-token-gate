# Proposal: Kubernetes Helm deployment

Related issue: #5

This proposal packages the service for Kubernetes with a small Helm chart. The
chart should expose configuration through a Secret and ConfigMap, include
liveness/readiness probes, and preserve the container's non-root,
read-only-filesystem posture.

Redis remains an explicit dependency rather than an accidentally bundled
subchart. The README should document whether operators provide managed Redis
or deploy a separate chart and how upgrades are rolled back safely.
