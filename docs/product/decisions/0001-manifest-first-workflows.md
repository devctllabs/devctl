# Keep Project changes and workflows explicit

Status: Accepted
Date: 2026-09-03

One checked-in Manifest defines desired Project shape, and each Devctl workflow
has explicit filesystem, tool, and network effects. Manifest mutation, scaffold,
sync, lint, generation, and migration execution do not silently compose one
another, so users can review changes and run only the operations a change needs.
Stable effective Targets keep selection consistent across workflows instead of
giving the same declaration different meanings in different commands.

The accepted rationale is recorded in
[ADR 0001](../../adr/0001-canonical-project-model-and-command-boundaries.md),
with migration execution added by
[ADR 0009](../../adr/0009-explicit-migration-execution.md). Current behavior is
described in [Project lifecycle](../project-lifecycle.md).
