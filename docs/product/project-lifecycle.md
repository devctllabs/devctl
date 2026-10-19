# Project lifecycle

Devctl is a non-interactive control plane for Go Projects. A checked-in
`devctl.yaml` Manifest declares desired Project shape; application behavior and
protocol definitions remain explicit user-owned inputs.

## Initialization and mutation

Manifest v1 supports Go with `cli` and `http-service` initialization presets.
`init manifest` writes only the Manifest. `enable` adds runtime Capabilities;
`add` declares Resources, Sources, clients, and Kafka endpoints. These commands
do not install tools, scaffold an application, acquire Contracts, or run code
generation.

An identical declaration succeeds unchanged. Replacing a different existing
declaration requires `--force`. `init manifest --force` rebuilds the complete
Manifest from its arguments and preset; it does not merge with existing content.
Other mutations operate on the selected semantically valid Manifest and publish
the updated document atomically.

`init scaffold` is a separate operation that refreshes Managed Outputs and
creates missing Scaffold Seeds. It has no `--force` option. After a structural
change, users review the generated foundation and explicitly register new
Provider Bindings. See [scaffold and ownership](scaffold-and-ownership.md).

## Discovery and paths

Without `--file`, Devctl searches upward from the invoking directory for
`devctl.yaml`. An explicit file selects that Manifest and its directory as the
Project root; `--file` may be absolute. Manifest-declared paths are
Project-relative and must remain contained. Semantic validation checks path
declarations and managed-output conflicts; filesystem adapters enforce
containment when inputs and outputs are accessed.

Migration env-file paths and SQLite database URLs have their own
[working-directory rules](migrations.md#database-configuration-and-paths).

## Validation and inspection

| Layer | What it establishes |
| --- | --- |
| Structural decoding | Valid YAML shape, known fields, correct scalar/container types, no duplicate keys |
| Semantic validity | Supported version/language, valid identity and paths, references, defaults, Resource rules, Runtime Config consistency |
| Readiness | Required local inputs, migration directories, module/tool declarations, and native generator configurations |

`validate` checks all three layers. Findings are a normal result with exit code
`1`; loading or I/O failures use the execution-error contract. Other workflows
load a semantically valid Project and check their own prerequisites, without
requiring unrelated application or generator readiness.

`inspect` reports the effective Project: paths, Targets, Resources, environment
keys and safe defaults. It is an inspection operation, not source acquisition.
Missing or invalid committed Snapshot Metadata simply omits `resolved_input`;
it does not prevent inspection of the Manifest.

## Addressable operations

The Target Catalog gives `sync`, `lint`, and `gen` stable selectors:
`config`, `http-server`, `http-client:<name>`, `grpc-server`,
`grpc-client:<name>`, `kafka-consumer:<name>`, and `kafka-producer:<name>`.
Every Go Project has an implicit config Target. Each Target declares which
operations it supports, and `inspect` lists the effective facts in stable order.

An unknown family produces `invalid_input`; a known family with no applicable
Targets succeeds with an empty result. An unknown explicit Target produces
`not_found`; an existing Target without the requested operation produces
`unsupported`. Local Contract sync is a supported no-op. Raw Kafka Targets
have no sync operation. Migration Targets use their separate
`connection:variant` selection rules.

Workflows remain explicit: `sync`, `lint`, and `gen` never invoke one another,
and applying migrations requires `migrate`. This boundary is recorded in
[PDR 0001](decisions/0001-manifest-first-workflows.md). See the
[Project workflow guide](../user-guide/project-workflow.md) for command sequences
and [CLI automation](cli-automation.md) for result and failure handling.
