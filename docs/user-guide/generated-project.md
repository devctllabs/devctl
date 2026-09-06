# Working in a generated Project

## Refresh safely

Run `devctl init scaffold` whenever the Manifest gains a Component, Resource,
or generator configuration. Review the reported `created`, `updated`, and
`unchanged` paths.

Managed files may change on every refresh. Scaffold Seeds are yours after
creation. Do not put handwritten files inside a Target directory owned by
`sync` or `gen`, because publication may replace that complete tree.

## Compose application behavior

`internal/deps/application.go` is the user-owned composition list. Generated
`provideX` functions register infrastructure; call the providers your
application needs from `provideApplication`. Refresh may add new binding Seeds
but does not edit this list.

HTTP and gRPC behavior is registered through `HTTPRegistrar` and
`GRPCRegistrar`. Generated outbound providers expose raw HTTP transports/base
URLs or gRPC connections; application code constructs the generated client or
stub it needs.

## Run Scenarios

The generated `Scenario` owns one lazy dependency graph and orderly shutdown:

- `deps.NewAPI(ctx)` resolves the API runtime and its HTTP, gRPC, health, and
  pprof work.
- `deps.NewConsumer(ctx, name)` validates the selected Kafka consumer and
  resolves only that runner.

Unknown or disabled consumers fail before their Kafka client is constructed.
The initial Kafka handler Seed returns a permanent not-implemented error so an
unfinished consumer cannot acknowledge messages silently.

## Configure runtime behavior

Generated Runtime Config is derived from the Manifest. `.env.example`,
`inspect`, generated config, and scaffold consumers share the same catalog.

No `start` block means a Capability is always on. A present `start` block
creates an environment toggle; when its `default` is omitted, the effective
default is `false`. Secret entries never receive rendered defaults.

## Manage migrations

For each database Variant with migrations, scaffold creates the directory and
pinned Mise tasks. Devctl does not write SQL or apply migrations.

```sh
mise run migrate:primary:postgres:create create_orders
mise run migrate:primary:postgres:up
mise run migrate:primary:postgres:down
```

Runtime DSNs and migration URLs are independent. ClickHouse migrations are
non-transactional; a failed multi-statement migration may require manual
inspection and recovery before forcing a version.
