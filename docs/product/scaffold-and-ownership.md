# Scaffold and ownership

`init scaffold` materializes the foundation described by a semantically valid
Manifest. It plans and preflights artifacts before publishing them in stable
order. Scaffold does not install dependencies, synchronize Contracts, run
external generators, or apply migrations.

## File ownership

| Owner | Examples | Refresh behavior |
| --- | --- | --- |
| Managed Output | `go.mod`, `.mise.toml`, `.golangci.yml`, `.env.example`, generated config, `internal/deps/*.gen.go`, canonical native generator configs | Devctl may replace contents |
| Scaffold Seed | Application commands, `internal/deps/application.go`, Provider Bindings, handlers, server OpenAPI seed, scaffolded README, migration placeholders | Created if absent; existing regular files remain user-owned |
| User-owned input | Migration SQL, alternate native generator configs, handwritten application behavior | Not refreshed by scaffold |

Devctl-owned Go scaffold files use `*.gen.go` and a generated-file header.
Ownership also applies to non-Go files: manual dependency or task edits can be
replaced when `go.mod` or `.mise.toml` is refreshed. Review those diffs after
scaffold and dependency resolution.

Managed Contract and generated-code trees belong entirely to their owning
workflow. Handwritten files inside those trees can be removed by replacement.
The distinction and partial-publication policy are recorded in
[PDR 0003](decisions/0003-managed-outputs-and-scaffold-seeds.md).

## Go quality checks

Scaffold provides a pinned golangci-lint toolchain, `gofmt` formatting, and Go
lint tasks. The lint configuration checks handwritten production and test code
with cyclomatic complexity limited to 10, cognitive complexity limited to 15,
at most four function parameters, and at most three results. Existing checks
for errors, resources, context usage, and test practices remain enabled.

The Project's Mise tasks expose Go linting as `lint:go` and Contract linting
as `lint:contracts`; `lint` runs both. The `devctl lint` command checks
Contracts. The `fmt` task formats Go code, and `fmt:check` checks formatting.

In importable production packages, exported types, functions, and methods
require Go doc, including exported methods on private receivers and public
interface methods. The production Go doc check skips `package main`.
Production constants and variables do not require documentation, and name
stuttering is not checked. In test files, package-level declarations and methods
require Go doc, including private helpers. Standard `Test`, `TestMain`,
`Benchmark`, `Fuzz`, and `Example` entrypoints do not. Interface method inputs
must be named, including methods with a single input.

Generated Go files with the standard generated-file marker and the `gen/` tree
are excluded. Refresh replaces the lint configuration but preserves existing
Scaffold Seeds, so existing application code may need documentation when the
quality checks are strengthened.

## Native generator configs

Canonical HTTP config paths are `tools/oapi/server.yaml` and
`tools/oapi/clients.<name>.yaml`. Canonical consumer Buf configs are
`tools/buf/grpc.gen.yaml` and `tools/buf/kafka.gen.yaml`. These remain managed
even when the Manifest explicitly spells out their canonical value.

An alternate config path is user-owned. Scaffold does not create or overwrite
it; Readiness requires a regular, non-symlink file at a safe path, and `gen`
consumes it. Filesystem access enforces containment for alternate paths.
Several HTTP Targets may share a user-owned config. A custom HTTP config path
that equals a planned scaffold artifact is rejected before publication so one
file cannot acquire conflicting owners.

## Refresh and composition

Refresh creates missing Seeds and updates current Managed Outputs. It does not
delete stale scaffold files after a Component or Resource is removed. This
differs from cleanup inside managed `sync` and `gen` trees; users review and
remove obsolete scaffold files themselves.

`internal/deps/application.go` contains the user-owned `provideApplication`
composition list. Scaffold can add infrastructure and new binding Seeds but
does not edit that list. Users explicitly call new providers and connect
application handlers, generated clients, and message types. See
[generated application behavior](generated-application.md) and
[PDR 0007](decisions/0007-explicit-application-composition.md).

## Failures and concurrency

Unsafe symlinks, non-regular destinations, and detectable plan conflicts fail
before publication. A later I/O failure can leave previously published files;
the result reports that progress and no workflow-wide rollback occurs.

Seed creation preserves existing files but is not an atomic cross-process
create-if-absent operation. An external writer can race the existence check.
`init scaffold` has no force option; migration SQL creation has its own stronger
[version reservation and exclusive creation](migrations.md#sql-ownership-and-creation)
rules.
