# Consume external Contracts from committed Snapshots

Status: Accepted
Date: 2026-09-03

External suppliers can change or become unavailable between builds, so `sync`
publishes bounded, reviewable Contract Snapshots into the consuming Project.
Lint and generation consume local or committed state and metadata without
reacquiring the supplier, trading automatic freshness for reproducible inputs
and explicit updates. Named Exports identify exact upstream surfaces rather
than relying on schema discovery heuristics.

## Consequences

Users commit Snapshots and refresh stale metadata through `sync`. Running
without network access also requires preparing tool and module dependencies;
committing supplier state does not sandbox external tools.

The rationale is recorded in
[ADR 0002](../../adr/0002-contract-sources-exports-and-snapshots.md). Current
behavior is described in [Contracts and generation](../contracts-and-generation.md).
