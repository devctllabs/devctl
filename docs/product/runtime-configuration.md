# Runtime configuration

The Manifest defines one effective environment contract for generated Go config,
`.env.example`, inspection, and application consumers. Users can inspect its
keys, types, defaults, and secret markers before starting the application.
The decision to keep these projections consistent is recorded in
[PDR 0005](decisions/0005-unified-runtime-configuration.md).

## Keys, values, and generated code

The default environment prefix derives from the Project name:
`orders-api` becomes `ORDERS_API_`. `env.prefix` overrides it. Keys beginning
with `OTEL_` retain their ecosystem prefix.

Built-in Component settings and custom environment declarations share key and
type rules. Custom values support string, bool, int, and duration types.
Effective key conflicts and generated Go field-path collisions are reported as
`runtime_config_conflict`, rather than producing inconsistent projections.

Generated fields are grouped by meaning, such as `cfg.HTTP.Address` or
`cfg.DBPrimary.PostgresDSN`. Runtime values come from environment with declared
defaults. Secret fields have no rendered defaults: generated tags omit them,
`.env.example` leaves them blank, and inspection exposes a secret marker.

Every Go Project has a config Target. Its default output is
`gen/config/config.gen.go`; `languages.go.generators.config.out` selects the
output directory while the filename remains `config.gen.go`. Both `gen config`
and scaffold render that config and the root `.env.example` from the same
effective settings. The example file is a managed template, not an implicit
request to load dotenv files into application processes.

## Runtime Start Policy

An absent `start` block means a Capability is always enabled. A present block
requires an environment key and creates a toggle; omitted `default` means
`false`. Runtime settings distinguish constructing a dependency from starting
its background work: a disabled Capability may still be constructed but does
not contribute a runtime task.

Kafka consumer selection is stricter: an unknown or disabled selected consumer
fails before its client graph is constructed. API Scenarios run the enabled
HTTP, gRPC, health, and pprof tasks. Logging and telemetry are supporting
resources rather than additional API server roots.

## Resource configuration

Database Runtime Config selects one Variant of a logical Connection and its
runtime DSN. A single Variant can omit the Connection default; multiple
Variants require a valid default. ClickHouse remains separate from
transactional Variants in a Connection.

Redis settings identify named connection addresses. S3 settings describe named
connections, ambient or static credentials, endpoint/region options, and bucket
selection. HTTP and gRPC clients have environment-backed base URLs or addresses.
Kafka settings cover brokers, topics, groups, batching, retry, rebalance, drain,
and shutdown behavior; Manifest topics are defaults and can be overridden
through environment.

Kafka's default commit retry inherits the main retry policy, and rejection stops
the consumer. Additional library-specific choices remain in user-owned custom
settings and Provider Bindings until exposed by the Manifest.

## Migration boundary

Migration-only URLs appear in `.env.example` and inspection but are absent from
application Runtime Config. `migrate` resolves its selected environment key
separately, with its own explicit env-file and fallback rules. Runtime DSNs are
not migration fallbacks. See [migrations](migrations.md) and the
[environment reference](../user-guide/reference/manifest/project-and-language.md#env).
