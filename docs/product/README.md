# Product documentation

Devctl gives Project authors explicit workflows for defining, validating, and
materializing Go projects. Product documents describe implemented capabilities,
their observable behavior, and operational constraints.

## Capabilities

- [Project lifecycle](project-lifecycle.md): Manifest initialization and mutation,
  discovery, validation, inspection, and effective Targets.
- [Contracts and generation](contracts-and-generation.md): bounded Sources,
  committed Snapshots, linting, generators, and publication.
- [Scaffold and ownership](scaffold-and-ownership.md): Managed Outputs, Seeds,
  custom configurations, and refresh behavior.
- [Runtime configuration](runtime-configuration.md): environment policy,
  generated config, Start Policy, and migration separation.
- [Generated application](generated-application.md): explicit composition,
  Scenarios, protocol integrations, and Resource contracts.
- [CLI automation](cli-automation.md): invocation, JSONL, findings, failures,
  previews, and partial progress.
- [Migrations](migrations.md): create SQL pairs, apply or roll back migrations,
  and inspect one database Variant's migration state.

## Product decisions

Accepted Product Decision Records preserve the reasons for significant behavior:

- [PDR 0001: Manifest-first workflows](decisions/0001-manifest-first-workflows.md).
- [PDR 0002: Committed Contract Snapshots](decisions/0002-committed-contract-snapshots.md).
- [PDR 0003: Managed Outputs and Scaffold Seeds](decisions/0003-managed-outputs-and-scaffold-seeds.md).
- [PDR 0004: Project-owned toolchain](decisions/0004-project-owned-toolchain.md).
- [PDR 0005: Unified runtime configuration](decisions/0005-unified-runtime-configuration.md).
- [PDR 0006: Structured CLI contract](decisions/0006-structured-cli-contract.md).
- [PDR 0007: Explicit application composition](decisions/0007-explicit-application-composition.md).
- [PDR 0008: Explicit migration target selection](decisions/0008-explicit-migration-target-selection.md).
- [PDR 0009: Migration status and recovery](decisions/0009-migration-status-and-recovery.md).

Capability documents state current behavior; PDRs preserve decision history.
Proposals, open questions, System Designs, and tickets belong in the
[local issue tracker](../agents/issue-tracker.md). They become current product
documentation after implementation.

Use the [user guide](../user-guide/README.md) for command walkthroughs, the
[domain glossary](../../CONTEXT.md) for terms, and the
[architecture overview](../architecture/overview.md) for technical boundaries.
