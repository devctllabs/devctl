# Generated application

Scaffold provides Go infrastructure and executable foundations. Application
behavior stays in user-owned code, including provider composition, protocol
registrations, handlers, and repositories. Adding a Manifest Component does not
silently wire new application behavior into existing composition.

## Composition and Scenarios

`provideApplication` is the explicit user-owned provider list. Generated
infrastructure registers lazy dependencies; users connect those providers to
their application types. Devctl does not rewrite Go syntax or discover plugins
at runtime. See [PDR 0007](decisions/0007-explicit-application-composition.md).

An API Scenario owns its dependency graph, selected HTTP/gRPC/health/pprof work,
and shutdown. A consumer Scenario owns one named Kafka runner. Construction is
lazy and shutdown closes resources constructed by that Scenario. Selecting an
unknown or disabled consumer fails before its Kafka client is constructed.
These are application errors, separate from Devctl's CLI error categories.

The `cli` preset supplies a root command foundation; application-specific
commands and behavior remain user-owned. Scaffold creates API or consumer
command Seeds when those capabilities are present.

## Protocol boundaries

| Capability | Generated contract and user responsibility |
| --- | --- |
| HTTP server | Echo runtime with a typed `HTTPRegistrar`; users implement and register application routes |
| gRPC server | Runtime with a typed `GRPCRegistrar`; users register generated services |
| HTTP client | Raw `*http.Client` and base URL; users construct the generated OpenAPI client |
| gRPC client | Raw `*grpc.ClientConn`; users construct Proto stubs; the generated connection uses plaintext credentials |
| Kafka producer | Raw byte producer with separately available topic configuration; users compose application publishing behavior |
| Kafka consumer | Named config, decoder, batch handler, retry policy, and runner; users implement the handler and select message types |

The generated gRPC client provider has no Manifest TLS/auth surface. Secure
transport composition requires user-owned wiring. Devctl does not add facades
around generated HTTP clients or Proto stubs.

Kafka supports raw bytes, JSON Schema, and Proto, with binary or JSON Proto
encoding. Omitting `format` selects raw; schema-backed endpoints explicitly
choose `json` or `proto`. Schema-backed consumers initially bind `[]byte` too;
users opt into generated message types in their Provider Binding. Raw batch
handlers must not retain batch data. The initial handler returns a permanent
not-implemented error, so unfinished behavior cannot acknowledge messages
silently.

## Resources and operations

| Resource | Supported behavior |
| --- | --- |
| SQLite / PostgreSQL | One selected Variant per logical Connection, transactional endpoints, reader/writer access and transaction manager |
| ClickHouse | Native connection and eager ping; separate from transactional Variants, without transaction manager or reader/writer abstraction |
| Redis | Named client and owned close behavior; addresses exclude credential-bearing URLs; no generated health probe |
| S3 | Named AWS SDK connections with ambient/static credentials, endpoint, region and path-style options; named bucket wrappers |
| Logging / telemetry | Shared graph resources with environment-backed configuration and owned cleanup |
| Health / pprof | Configured operational server tasks; generated database probes are non-critical |

Backend-specific ClickHouse semantics are recorded in
[ADR 0008](../adr/0008-native-clickhouse-runtime-and-migrations.md). Migration
execution is separate from runtime construction and application startup.

Scaffold does not produce application Dockerfiles, Compose files, Helm charts,
or Kubernetes manifests. The repository's own container image packages the
Devctl CLI. Projects own their application deployment packaging.

See [runtime configuration](runtime-configuration.md),
[scaffold ownership](scaffold-and-ownership.md), and the
[generated Project guide](../user-guide/generated-project.md) for the related
contracts and application workflow.
