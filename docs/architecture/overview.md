# Architecture overview

Devctl is a non-interactive CLI over a canonical Project Manifest. Explicit
workflows change the Manifest, publish Managed Outputs, or execute selected
operations. Generated applications have their own composition and runtime;
Devctl does not run them or apply their schema at startup. The
[domain glossary](../../CONTEXT.md) defines the vocabulary and the
[product index](../product/README.md) links observable behavior.

## Components and boundaries

| Component | Responsibility |
| --- | --- |
| CLI adapters | Parse and validate input, invoke one service root, map results and errors to the CLI contract |
| Services | Own workflow policy and coordinate consumer-owned ports |
| Domain | Define the Manifest, effective projections, operation data, pure invariants, and typed failures |
| Repositories | Own Manifest and workspace persistence, rooted file publication, migration files and database mechanics |
| Clients | Own HTTP/Git acquisition, generator routing, and external tool execution |
| Protocol analyzers | Validate OpenAPI and JSON Schema independently of workflow orchestration |
| Production composition | Register concrete providers and lazily resolve the graph needed by the selected command |

Dependencies point `cmd -> service -> domain`. Business owners depend on their
ports; concrete adapters and integration resources are composed in
`internal/deps`. The Project service is the common gateway for discovery,
decoding, semantic validation, inspection, and Manifest mutation.
[ADR 0001](../adr/0001-canonical-project-model-and-command-boundaries.md)
records these boundaries.

## Manifest and effective projections

Discovery selects a Manifest and Project root. Structural decoding rejects
unknown/duplicate fields and wrong YAML types; domain validation establishes
semantic validity. `validate` additionally calls the Readiness checker for local
inputs, module/tool declarations, and native configs. Other workflows load a
semantically valid Project and check only their own execution prerequisites.

The Target Catalog is the shared source of identities, references, Logical
Inputs, output locations, and supported operations for contract/config workflows.
Selection is deterministic; generation has an explicit dependency-sensitive
execution order. The Runtime Config catalog separately owns effective keys,
defaults, secret policy, and Go field paths. Its shared renderer serves scaffold
and config generation, while inspection consumes the same catalog.

A separate ordered projection derives Migration Targets and their effective
environment keys. Inspect, scaffold, and migration execution consume that
projection. Migration-only settings appear in example/inspect output and stay
outside application Runtime Config; see
[ADR 0005](../adr/0005-canonical-runtime-config.md).

## Contract acquisition and consumption

`sync` selects Targets, materializes each external Source, then publishes a
normalized Snapshot. Source strategies isolate local containment, bounded HTTP
closures, temporary Git checkouts, and named Exports from upstream Devctl
Projects. The materialization result carries files, Entrypoint or Module Root,
and metadata needed to interpret the selected surface.

Downstream workflows use a shared committed Snapshot loader and Target input
resolver. The loader validates metadata against the Target, and resolution maps
its Logical Input to local input files or a committed Resolved Input. Missing
required metadata fails lint/generation with sync guidance, while inspect omits
unavailable resolution and still describes the Project.

Lint combines protocol analysis and Project-owned Buf checks. Generation routes
effective Targets to the shared config renderer, oapi-codegen, Buf, or quicktype.
Generator adapters own tool declarations, arguments, bounded diagnostics, and
expected output validation; shared process and temporary-output mechanics stay
behind those adapters. Tools render into temporary storage before publication.

Only sync acquires configured Contract suppliers. Lint and generation consume
local or committed inputs, but external tools can require their own module or
plugin downloads. The architecture does not impose process-level network
isolation. See [ADR 0002](../adr/0002-contract-sources-exports-and-snapshots.md)
and [ADR 0004](../adr/0004-project-owned-generator-toolchain.md).

## Publication and scaffold

The workspace repository owns rooted containment and atomic file/tree
publication. Publication reports created, updated, unchanged, or removed as part
of the operation. Dry-run uses read-only inspection and performs no Source
acquisition, tool execution, publication, or pruning.

Generation owns each selected output tree; sync owns the external-contract
namespace and can prune stale children during full/family execution. Atomicity
stops at a publication boundary: separate config/environment files, multiple
Targets, and scaffold artifacts are not one transaction. Later failures preserve
completed changes. See [ADR 0003](../adr/0003-managed-artifacts-and-publication.md).

Scaffold compiles a private typed projection, renders its artifact plan, assigns
ownership, and preflights conflicts before publishing in stable order. Canonical
generator configs are managed; custom HTTP configs are excluded from publication
and checked for collisions with planned artifacts. Shared custom configs are
allowed. Scaffold preserves existing Seeds, uses a best-effort existence check
for their creation, and does not prune artifacts removed from the Manifest.

## Generated application runtime

Generated infrastructure and user-owned Provider Bindings form a second
composition boundary. Managed `provideX` functions register infrastructure;
`provideApplication` explicitly selects bindings and remains a Scaffold Seed.
Typed HTTP/gRPC registrars connect user behavior to runtime servers. Outbound
providers expose raw transport handles, leaving generated client/stub composition
to application code.

An API Scenario resolves enabled server tasks. A consumer Scenario validates
its selected name and Start Policy before resolving one named Kafka runner.
Providers are registered eagerly and constructed lazily; shutdown owns resources
actually constructed by that graph. Consumer bindings select decoder, handler,
message types, and retry policy. The unfinished handler Seed fails closed.
These contracts are recorded in
[ADR 0007](../adr/0007-explicit-application-composition-and-runtime-scenarios.md).

Database providers open only the selected Variant of a logical Connection.
SQLite/PostgreSQL expose transactional endpoints; ClickHouse retains its native
API and separate migration driver. Redis, S3, logging, and telemetry are owned
graph resources; health and pprof contribute operational tasks. Generated gRPC
clients currently use plaintext transport. Supported interfaces and constraints
are detailed in [Generated application](../product/generated-application.md).

## Migration execution flow

1. The CLI validates arguments before constructing dependencies. The migration
   service loads the Project and selects one Migration Target from its Manifest.
2. `create` goes directly to the migration repository for contained file
   creation. Database operations first resolve the migration URL through a
   separate environment adapter, which reads an optional dotenv file without
   modifying process state.
3. The repository opens a filesystem rooted at the Project and constructs
   `go-libs/dbmigrate` with the selected SQL directory and resolved database URL.
   Registered upstream drivers implement PostgreSQL, SQLite, and ClickHouse
   migration behavior. The same constructor serves status and execution.
4. The repository reads state, performs the requested operation, and reads the
   resulting state when applicable. Results preserve observed progress on
   failure. The migrator closes before its source filesystem; close and operation
   errors are joined rather than losing either failure.
5. The CLI emits one success result or a final error with an available partial
   result. Migration errors retain their causes internally while rendering safe
   reasons, target identities, and paths instead of SQL or credentials.

[ADR 0009](../adr/0009-explicit-migration-execution.md) records the integration
choice. The observable selection, configuration, status, and recovery rules live
in the [migration product document](../product/migrations.md).

## Operational constraints

The CLI composition root owns resource cleanup after each command. Signals
cancel its context, and the final reporter preserves typed categories and
available partial progress. Findings remain normal results; execution errors go
to stderr. Migration errors retain causes without rendering SQL or credentials,
even under verbose logging. See
[ADR 0006](../adr/0006-machine-readable-cli-contract.md).

Migration SQL is user-owned. File creation uses exclusive operations and a
per-version reservation; a pair is not atomic across crashes. Migration SQL and
URLs pass through unchanged. ClickHouse migrations remain non-transactional and
separate from the native application connection in
[ADR 0008](../adr/0008-native-clickhouse-runtime-and-migrations.md).

The drivers support Devctl's builds with CGO disabled. Context checks precede
migration operations, but in-flight SQL and closing follow the library and
driver's waiting semantics; connection and query timeouts belong to the driver
URL.

Release workflows package Devctl as a CLI archive or its own toolchain
container. Scaffold does not generate application deployment packaging. The
build/platform matrix, release and image workflows, temporary sibling-module
requirement, and automated versus live-database checks are described in the
[development guide](../development.md).
