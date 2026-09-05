# ADR 0004: Keep generator toolchains project-owned

- Status: accepted
- Date: 2026-09-03

## Context

Checked-in generated code must be reproducible from a standalone Project. A
Devctl-bundled or globally installed generator can drift independently of the
Project and makes upgrades invisible in code review. Native generator configs
also have different ownership depending on whether the canonical path or an
explicit override is selected.

## Decision

The consuming Project owns generator versions and native configuration. Go
generators are pinned to exact published module versions with `go.mod` tool
directives. Mise owns exact executable runtime and task versions that are not
Go tools, including Node 24, quicktype 26.0.0, and golang-migrate 4.19.1 when
their capabilities are used. Generator objects in the Manifest do not expose a
competing `tool` selector.

Devctl invokes tools through the Project environment, writes into temporary
storage, validates expected output, and hands publication to the owning
workflow. Canonical shared native generator configs are Managed Outputs. An
explicit custom config path is user-owned: it must already exist, validation
checks it, generation consumes it, and scaffold never creates, overwrites, or
deletes it. Every distinct gRPC Target config is planned and validated.

Proto input ownership is split deliberately. A supplier's Contract Snapshot
contains its declared Buf config and adjacent lock file; the consuming Project
owns its `*.gen.yaml` generation config. Buf paths must remain contained
project-relative regular files and each file is included exactly once.

Quicktype 26.0.0 is the Kafka JSON Schema generator. Every input Schema has a
non-empty root `title`, which owns the generated top-level type name. The Go
output includes serialization helpers and marks non-required fields with
`omitempty`. Devctl remains Go-only in v1; choosing quicktype does not add a
multi-language Manifest surface or a `go:generate` workflow.

## Consequences

A fresh checkout can reconstruct its toolchain from versioned Project files.
Generator upgrades are explicit dependency changes and may update checked-in
Managed Outputs. Devctl reports missing Project tools with installation
guidance rather than silently substituting bundled versions.

We reject global PATH lookup as the version policy, generated custom override
files, generator-specific version fields in the Manifest, and heuristic Buf
config discovery.
