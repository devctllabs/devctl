# Project workflow

## Create a Manifest

Choose the minimal CLI application or an HTTP service foundation:

```sh
devctl init manifest \
  --lang go \
  --preset http-service \
  --name billing-api \
  --module example.com/billing-api
```

The command creates only `devctl.yaml`. `--force` replaces the complete
Manifest from the supplied arguments; it does not merge with the old file.

## Change desired state

Use `enable` for runtime Capabilities and `add` for named Resources, Sources,
clients, and Kafka endpoints:

```sh
devctl enable grpc
devctl add db primary --kind postgres
devctl add redis cache
```

Mutation commands preserve a valid existing declaration unless `--force` is
given. They never install dependencies or change handwritten Go.

## Materialize the foundation

```sh
devctl init scaffold
mise install
go mod tidy
```

Run scaffold again after adding Components or Resources. Review created files,
then register new user-owned Provider Bindings in
`internal/deps/application.go`. See [generated Project](generated-project.md)
for the ownership contract.

## Validate and inspect

```sh
devctl validate
devctl inspect
```

`validate` checks three distinct layers: YAML/schema structure, semantic
validity, and Project Readiness. Findings are returned as a normal result and
produce exit status `1`.

`inspect` shows the effective Project rather than repeating raw YAML. Use it to
find effective paths, Target IDs, Runtime Config keys and defaults, Resources,
and resolved Contract inputs. Missing or stale external Snapshot Metadata does
not make inspection fail; `resolved_input` is simply absent until `sync`
publishes valid metadata.

## Preview destructive publication

Before generation or a full external synchronization, inspect the plan:

```sh
devctl gen --dry-run
devctl sync --dry-run
```

Dry runs report `planned_publish` and, for full synchronization, possible
`planned_remove` actions. A targeted sync never removes sibling Target trees.
