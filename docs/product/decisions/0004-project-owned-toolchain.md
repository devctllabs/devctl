# Keep generator versions and configuration in the Project

Status: Accepted
Date: 2026-09-03

Generated code must be reproducible from versioned Project inputs, so generator
versions and native configuration belong to the consuming Project rather than
an independently changing global or Devctl-bundled policy. Go tool declarations
and Mise describe the selected toolchain, making upgrades reviewable dependency
changes. Canonical generator configs are managed, while alternate configs stay
user-owned and are consumed without scaffold replacement.

The rationale is recorded in
[ADR 0004](../../adr/0004-project-owned-generator-toolchain.md). Current
behavior is described in [Contracts and generation](../contracts-and-generation.md)
and [Scaffold and ownership](../scaffold-and-ownership.md).
