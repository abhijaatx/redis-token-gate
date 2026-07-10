# Proposal: configurable identity normalization

Related issue: #6

This proposal makes identity normalization an explicit, opt-in policy for
callers that need case folding or tenant-aware composition. The default must
remain byte-preserving so existing buckets do not change meaning on upgrade.

Normalization must happen before hashing and be documented with its collision
and migration implications. Tests should cover whitespace, Unicode, and
tenant-prefixed identities without ever persisting the raw value.
