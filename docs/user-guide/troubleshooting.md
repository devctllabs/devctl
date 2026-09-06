# Troubleshooting

Start with verbose diagnostics and an effective Project view:

```sh
devctl validate --verbose
devctl inspect --json
```

## Manifest not found

Without `--file`, Devctl searches upward from the current directory for
`devctl.yaml`. Run from inside the Project or pass an explicit path.

## Invalid Manifest

`validate` distinguishes malformed YAML, unknown or duplicate fields, semantic
conflicts, invalid references, unsafe paths, and missing Readiness. Fix every
reported issue before retrying the workflow that needs that state.

Unknown fields are rejected; they are not ignored for forward compatibility.
Manifest `version` must currently be `1` and the supported language is `go`.

## Missing Project tools or configs

Run scaffold and install the Project-owned toolchain:

```sh
devctl init scaffold
mise install
go mod download all
go mod tidy
devctl validate
```

An explicit custom generator config is user-owned. Scaffold will not create or
replace it; create the file at the configured path or return to the canonical
managed config.

## Stale Snapshot Metadata

Devctl-sourced gRPC and schema-backed Kafka Snapshots need valid root
`.devctl-contract.json` metadata. Missing, unsafe, or inconsistent metadata
returns `invalid_input` with reason `snapshot_metadata_invalid`.

While the supplier is available, run the suggested targeted or full `devctl
sync`, review the refreshed Snapshot, and commit it. `inspect` remains usable
before the refresh but omits `resolved_input`.

## Target selection failures

- Unknown family: `invalid_input`.
- Known family with no applicable Targets: successful empty result.
- Unknown explicit Target ID: `not_found`.
- Existing Target without the requested operation: `unsupported`.

Local Contract Targets support sync as a no-op. Raw Kafka Targets do not
support sync or generation.

## A command failed after making progress

Targets execute sequentially. A later failure does not roll back completed
Targets. In JSON mode, inspect `details.partial_result` on the final stderr
event, review the listed changes, correct the cause, and retry. Planning-only
failures do not contain a partial result.

## Dry-run differs from execution

Dry-run is intentionally static: it performs no network acquisition, external
tool execution, publication, or pruning. It can show intended Target paths and
stale removal candidates, but it cannot promise that a later network request
or generator will succeed.

## ClickHouse migration failed partway

ClickHouse migrations are non-transactional. For files containing several
statements, use a migration DSN with `x-multi-statement=true`. Driver statement
splitting can still leave a partial apply. Inspect the database, complete or
revert the intended changes manually, and only then force the migration
version. Devctl does not alter the DSN or perform recovery.
