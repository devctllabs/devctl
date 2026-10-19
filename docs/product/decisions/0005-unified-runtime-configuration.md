# Share one effective runtime environment contract

Status: Accepted
Date: 2026-09-03

Independent defaults and environment-key rules previously let generated config,
scaffold consumers, and inspection disagree. All projections now share the
Manifest's effective keys, types, defaults, field groups, and secret handling so
the settings users inspect match the settings their application consumes.
Start Policy explicitly separates constructing a dependency from starting its
background work, while migration-only URLs remain outside application config.

The rationale is recorded in
[ADR 0005](../../adr/0005-canonical-runtime-config.md). Current behavior is
described in [Runtime configuration](../runtime-configuration.md).
