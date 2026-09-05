# Contracts and generation

## Choose a Source

Declare where a Contract originates, then reference it from a client or Kafka
endpoint. Source containment prevents references from escaping the selected
root.

```sh
devctl add source contracts --type local --path api/contracts
devctl add http-client billing \
  --source contracts \
  --path billing/openapi.yaml
```

Ordinary Sources select a relative `--path`. A Devctl Source selects a named
upstream `--export` instead. See [recipes](recipes.md) for the Source matrix.

## Synchronize external state

```sh
devctl sync
devctl sync http --target http-client:billing
```

`sync` materializes external Contract closures into Project-owned paths.
Review and commit those Snapshots. Local Sources are supported no-ops.

A full family sync may remove stale Target children. Targeted sync publishes
only the selected Target and never prunes another Target. Use `--dry-run` to
preview publication and pruning without network access or writes.

## Lint committed inputs

```sh
devctl lint
devctl lint grpc
```

Linting reads local Contracts or committed external Snapshots. It never
contacts the supplier and never runs generation. Findings are normal results:
stdout contains the complete result, stderr remains empty, and the process
exits `1` when invalid.

## Generate Managed Outputs

```sh
devctl gen
devctl gen http --target http-client:billing
```

Generation invokes versions and native configuration owned by the Project. It
publishes one Target atomically, removes stale files only inside that Target
tree, and never prunes siblings. Execution order is config, HTTP, gRPC, then
Kafka.

After a generator changes Go imports, finish with:

```sh
go mod tidy
mise run check
```

## Supplier and consumer Buf files

For Proto, the supplier's `buf.yaml` and adjacent `buf.lock` travel with the
Contract Snapshot. The consuming Project owns `*.gen.yaml`. Canonical consumer
configs are Managed Outputs; an explicitly selected `buf_gen_config` is
user-owned and must already exist before validation or generation.

## When to run each command

| Change | Scaffold | Sync | Lint | Gen |
|---|:---:|:---:|:---:|:---:|
| Manifest-only default | maybe | no | no | maybe |
| New Component or Resource | yes | if external | if Contract | if generated output |
| Local server Contract | no | no | yes | yes |
| External Contract revision | no | yes | yes | yes |
| Generator configuration | maybe | no | no | yes |
