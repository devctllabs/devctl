# Devctl user guide

Devctl turns one checked-in `devctl.yaml` Manifest into a reproducible Go
Project. This guide is organized around work you need to complete rather than
the internal packages that implement it.

## Start here

1. [Build an HTTP API with PostgreSQL](getting-started.md).
2. Learn the [core concepts and ownership rules](concepts-and-ownership.md).
3. Use the [Project workflow](project-workflow.md) when changing a Manifest.
4. Follow the [Contract and generation workflow](contracts-and-generation.md)
   when an API or event schema changes.

## Continue by task

- [Work inside the generated Project](generated-project.md)
- [Add databases, storage, clients, and event endpoints](recipes.md)
- [Diagnose and recover from failures](troubleshooting.md)

## Reference

- [Commands and flags](reference/commands.md)
- [Manifest v1](reference/manifest/README.md)
- [JSONL, result shapes, errors, and exit codes](reference/output-and-errors.md)

Documentation on `main` follows the current source tree. When using a released
binary, select the matching Git tag before reading these files.
