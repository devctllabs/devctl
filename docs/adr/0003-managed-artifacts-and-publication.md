# ADR 0003: Separate Managed Outputs from Scaffold Seeds

- Status: accepted
- Date: 2026-09-03

## Context

Sync, generation, and scaffold all publish files, but they do not have the
same ownership contract. Treating user-edited seeds as generated output risks
data loss, while treating managed trees as shared directories prevents safe
replacement and stale cleanup. Multi-Target workflows also need precise and
race-free change reporting.

## Decision

A Managed Output is a file or complete Target tree owned by one Devctl
workflow. It is rendered or materialized in temporary storage and atomically
published at the Target boundary. The publication adapter returns a structured
result that distinguishes `created`, `updated`, and `unchanged` as part of the
same operation; workflows do not infer this with a read-before-publish race.
This race-free publication guarantee applies to Managed Outputs only.

Targets execute sequentially and deterministically. A later failure does not
roll back completed Targets and returns their partial result. Generation
replaces only each selected Target tree and removes stale files inside that
tree; it never prunes unrelated Targets. The external-contract namespace
belongs entirely to `sync`; a full family sync may prune stale Target children,
while targeted sync never prunes unrelated Targets.

Observed changes are `created`, `updated`, `unchanged`, and `removed`.
Side-effect-free preview reports `planned_publish` and `planned_remove`.
Previewing removal uses a read-only repository operation with the same path,
symlink, and file-type validation as actual publication. Dry-run performs no
network acquisition, generator execution, publication, or pruning.

A Scaffold Seed is created once and is thereafter user-owned. Scaffold first
preflights its complete artifact plan. For each Seed it then checks the current
entry, leaves an existing regular file unchanged, rejects symlinks and
non-regular entries, and uses ordinary file publication only when the path is
absent. Scaffold never deliberately overwrites or deletes an existing Seed,
including during refresh. Managed Outputs may be refreshed independently.

The Seed check and publication are not a cross-process atomic
create-if-absent operation. An external writer can create the same path in the
narrow interval between them and have its file replaced. We accept this rare
race as a deliberate simplicity tradeoff rather than add a separate filesystem
primitive. Consequently `init scaffold` has no `--force` option;
`init manifest --force` remains a separate whole-manifest replacement command
that rebuilds the Manifest from its arguments or preset rather than merging.

This is a pre-v1 ownership cutover. Old scaffold filenames receive no
compatibility shim or automatic migration; users review and move existing
custom code explicitly.

## Consequences

Users can predict which files may be replaced and review dry-run cleanup before
it occurs. A workflow can leave a valid partially updated Project after a late
failure, so callers must inspect the returned changes before retrying. Every
scaffold plan must classify each artifact as managed or create-once.

Callers that require a hard cross-process create-only guarantee need a
different explicit publication operation; Scaffold does not claim that
guarantee for Seeds.

We reject workflow-wide rollback, read-before-publish change detection for
Managed Outputs, automatic deletion or intentional overwrite of Scaffold
Seeds, and shared managed directories containing arbitrary user files. We also
reject adding a dedicated atomic create-only filesystem primitive for Scaffold
Seeds while the practical risk remains limited to the accepted external-writer
race.
