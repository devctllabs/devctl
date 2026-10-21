# Devctl documentation

These documents describe the current source tree. For a released binary, use
the documentation at its matching Git tag.

## Product and architecture

- [Product documentation](product/README.md): scope and capability index.
- [Project lifecycle](product/project-lifecycle.md): Manifest changes, validation,
  inspection, and addressable operations.
- [Contracts and generation](product/contracts-and-generation.md): Sources,
  Snapshots, synchronization, linting, and generated outputs.
- [Scaffold and ownership](product/scaffold-and-ownership.md): refresh rules,
  Go quality checks, user-owned inputs, and partial publication.
- [Runtime configuration](product/runtime-configuration.md): environment keys,
  defaults, secrets, and Start Policy.
- [Generated application](product/generated-application.md): composition,
  Scenarios, protocol boundaries, and Resources.
- [CLI automation](product/cli-automation.md): invocation, structured outcomes,
  previews, and failure handling.
- [Migrations](product/migrations.md): target selection, execution, and recovery
  rules.
- [Architecture overview](architecture/overview.md): components, boundaries,
  workflow interactions, generated runtime, and operational constraints.
- [Domain glossary](../CONTEXT.md): canonical Project vocabulary.

## Decisions

- [Product Decision Records](product/README.md#product-decisions): accepted
  workflow, ownership, reproducibility, runtime, automation, and migration choices.
- [Architecture Decision Records](adr/): technical decisions and their reasons.

## Using and developing Devctl

- [User guide](user-guide/README.md).
- [Command reference](user-guide/reference/commands.md).
- [Manifest reference](user-guide/reference/manifest/README.md).
- [Output and error contract](user-guide/reference/output-and-errors.md).
- [Development guide](development.md), including migration verification.

Specs and implementation issues live in the local Markdown tracker under
`.scratch/`; see the [tracker conventions](agents/issue-tracker.md).
