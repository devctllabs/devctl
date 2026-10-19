# Select one migration target independently of runtime defaults

Status: Accepted
Date: 2026-10-19

A Project can declare several database Connections and Variants, so using its
runtime default for migrations could change the destination when application
wiring changes. Migrations select one `connection:variant`, with automatic
selection only when exactly one migration-enabled Variant exists across the
Manifest, keeping database changes tied to an unambiguous declared target.
The migration URL comes from its dedicated environment key, an explicit
env-file, or its Manifest default, so deployment environments can supply their
own database location without changing target identity.

## Consequences

CI specifies the target explicitly. Runtime database configuration and implicit
dotenv loading are not migration fallbacks; a present empty URL fails rather
than selecting a lower-priority destination. Adding another migration-enabled
Variant makes previously implicit selection require `--target`.

Current behavior is described in [Migrations](../migrations.md).
