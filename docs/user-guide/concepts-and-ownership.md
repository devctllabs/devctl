# Concepts and ownership

## Project and Manifest

A **Project** is the repository rooted at the selected Manifest. The
**Manifest** is the canonical desired-state document, normally `devctl.yaml`.
It declares Components, Resources, Sources, paths, Runtime Config, and the Go
generator policy.

When `--file` is absent, Devctl searches upward from the current directory for
`devctl.yaml`. Use `--file <path>` when you need an explicit Manifest.

## Components, Capabilities, and Resources

A **Component** is an application-facing area such as HTTP, gRPC, Kafka,
logging, or telemetry. A **Capability** is enableable runtime behavior inside a
Component. A **Resource** is a named infrastructure dependency such as a
database, Redis Connection, or S3 bucket.

`enable` and `add` mutate only the Manifest. After a structural change, run
`init scaffold` to materialize the corresponding Project foundation and `gen`
when generated code is required.

## Contracts, Sources, and Targets

A **Source** is a bounded origin for Contracts. A Contract Reference selects an
Entrypoint or named Export from that Source. External Contracts become
committed **Contract Snapshots** after `sync`, so later lint and generation can
run offline and produce reviewable changes.

A **Target** is the effective unit addressed by `sync`, `lint`, or `gen`, for
example `http-client:billing`. `inspect` shows the complete, deterministically
ordered Target Catalog and the inputs, outputs, and operations of each Target.

## Managed Outputs and Scaffold Seeds

Every scaffolded path has one owner:

- A **Managed Output** belongs entirely to Devctl. Its owning workflow may
  atomically replace the file or Target tree and remove stale files inside it.
- A **Scaffold Seed** is created once and belongs to you afterwards. Devctl
  does not deliberately overwrite or delete an existing Seed.

Devctl-owned Go scaffold files end in `*.gen.go` and carry a generated-file
header. Ordinary `.go` files, application entrypoints, Provider Bindings,
handlers, and the scaffolded README are Seeds.

`init scaffold` has no `--force`. It refreshes Managed Outputs, creates missing
Seeds, and rejects unsafe symlinks or non-regular entries. Adding a Component
may create a new Provider Binding, but you must add its `provideX` call to
`internal/deps/application.go` yourself.

## Explicit workflows

Devctl deliberately avoids hidden orchestration:

- `init manifest`, `enable`, and `add` do not install tools, scaffold files, or
  generate code.
- `sync`, `lint`, and `gen` never invoke one another.
- A later Target failure does not roll back Targets that already completed.
- `--dry-run` plans from committed local state without network acquisition,
  generator execution, publication, or pruning.

This makes command side effects predictable and repository diffs reviewable.
