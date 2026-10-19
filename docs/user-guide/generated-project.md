# Working in a generated Project

## Refresh safely

Run `devctl init scaffold` whenever the Manifest gains a Component, Resource,
or generator configuration. Review the reported `created`, `updated`, and
`unchanged` paths.

Managed files may change on every refresh. Scaffold Seeds are yours after
creation. Do not put handwritten files inside a Target directory owned by
`sync` or `gen`, because publication may replace that complete tree.

Managed files also include `go.mod`, `.mise.toml`, `.golangci.yml`,
`.env.example`, and canonical generator configs. Review refreshed dependencies
and task definitions as well as Go code. Removing a Component or Resource from
the Manifest does not prune its old scaffold files; review and remove obsolete
files yourself. See [Scaffold and ownership](../product/scaffold-and-ownership.md)
for the complete contract.

## Compose application behavior

`internal/deps/application.go` is the user-owned composition list. Generated
`provideX` functions register infrastructure; call the providers your
application needs from `provideApplication`. Refresh may add new binding Seeds
but does not edit this list.

HTTP and gRPC behavior is registered through `HTTPRegistrar` and
`GRPCRegistrar`. Generated outbound providers expose raw HTTP transports/base
URLs or gRPC connections; application code constructs the generated client or
stub it needs.

Generated gRPC client providers use plaintext transport. Use user-owned provider
composition when TLS or authentication is required; the Manifest does not expose
those options.

## Run Scenarios

The generated `Scenario` owns one lazy dependency graph and orderly shutdown:

- `deps.NewAPI(ctx)` resolves the API runtime and its HTTP, gRPC, health, and
  pprof work.
- `deps.NewConsumer(ctx, name)` validates the selected Kafka consumer and
  resolves only that runner.

Unknown or disabled consumers fail before their Kafka client is constructed.
The initial Kafka handler Seed returns a permanent not-implemented error so an
unfinished consumer cannot acknowledge messages silently.

Application deployment packaging remains Project-owned. Scaffold does not
generate Dockerfiles, Compose files, Helm charts, or Kubernetes manifests.

## Configure runtime behavior

Generated Runtime Config is derived from the Manifest. `.env.example`,
`inspect`, generated config, and scaffold consumers share the same catalog.

No `start` block means a Capability is always on. A present `start` block
creates an environment toggle; when its `default` is omitted, the effective
default is `false`. Secret entries never receive rendered defaults.

## Manage migrations

For each database Variant with migrations, scaffold creates the directory and
Mise tasks that call `devctl migrate`. SQL files remain user-owned.

```sh
mise run migrate:primary:postgres:create create_orders
mise run migrate:primary:postgres:up
mise run migrate:primary:postgres:up --steps 1
mise run migrate:primary:postgres:down
mise run migrate:primary:postgres:status
```

The equivalent direct commands are:

```sh
devctl migrate create create_orders --target primary:postgres
devctl migrate up --target primary:postgres
devctl migrate up --target primary:postgres --steps 1
devctl migrate down --target primary:postgres --steps 1
devctl migrate status --target primary:postgres
```

`--target` selects a Connection and Variant from the Manifest. It may be omitted
only when exactly one Variant has migrations enabled; the Connection's runtime
default does not affect selection. Use `inspect` to list migration resources.

All four operations use that Variant's `migrations.path`, relative to the Project
root, including when `--file` selects a Manifest elsewhere. There is no directory
override on `migrate`. Set the path in the Manifest or with `add db
--migrations-path` when adding the Variant.

`create` needs no database connection. It creates the directory if missing and
writes an empty UTC timestamped pair, such as
`20261019120000_create_orders.up.sql` and `.down.sql`. Names must start with an
ASCII letter or digit and contain only ASCII letters, digits, `_`, or `-`.
Existing versions and files are never overwritten. Ordinary creation errors
clean up files created by that invocation; a process crash can leave a partial
pair or `.devctl-create-<timestamp>` reservation to inspect and remove manually.

`up` applies all pending migrations by default and accepts positive
`--steps N` when a bounded application is needed. Repeating an unbounded `up`
when all migrations are applied succeeds with `no_change: true`; an empty
migration directory is an error. `down` rolls back one migration by default,
accepts positive `--steps N`, and never prompts. A bounded `up` or rollback
larger than the available history can fail after partial execution; rollback
of a fresh database is an error.

`status` returns `target`, `version` (null when nothing is applied), and `dirty`.
Dirty state is a successful status result, but prevents `up` and `down`.
Opening a target may initialize its version table; for SQLite it may also
create the database file. `status` therefore requires initialization privileges
on a fresh target.

### Migration configuration and CI

Runtime DSNs and migration URLs are independent. Devctl reads the selected
`migrations.database_env` with the Project environment prefix. Its precedence is
process environment, then an explicitly supplied `--env-file`, then Manifest
`migrations.database_default`. A present empty value is an error, and does not
fall through. Missing or malformed explicit env-files are errors even when the
process environment supplies the URL. No `.env` file is loaded automatically
and process environment is never modified. Dotenv parsing supports quoted
values, comments and `export`; variable expansion uses earlier entries in that
file, independently of process environment.

```sh
devctl migrate up --target primary:postgres --env-file .env.local
mise run migrate:primary:postgres:up --env-file .env.local
```

Relative env-file paths and relative SQLite database URLs resolve against the
directory of invocation. SQL paths always resolve against the selected Project.
Migration commands load the semantically valid Manifest without requiring the
Project's generator tools, generated application code, or go.mod readiness.

In a deployment job, provide the binary, Manifest, and SQL files. Set the migration
URL through the CI environment, for example
`ORDERS_API_DB_PRIMARY_POSTGRES_MIGRATIONS_URL` for the orders-api Project:

```sh
devctl migrate up --file devctl.yaml --target primary:postgres --json
```

Staging and production can use the same Manifest and target with different URL
values. Explicit targets keep jobs stable when another Variant is added.
The command emits the standard single JSONL result event, exits 0 on success or
no-change, and exits nonzero on failure. Failed operations that observed progress
attach state to `details.partial_result`; they do not undo applied SQL.

Cancellation before execution prevents migration work. Once SQL starts,
`dbmigrate` waits for its actual result, and closing resources may also wait for
the driver. Configure driver/statement timeouts in the migration URL; Devctl
does not provide an interrupting `--timeout`.

ClickHouse migrations are non-transactional. Supply `x-multi-statement=true`
when needed; driver semicolon splitting still has limitations and can leave a
partial apply. Devctl leaves the URL and SQL unchanged. Inspect and repair dirty
databases before using external recovery tooling; this CLI has no `force`.
