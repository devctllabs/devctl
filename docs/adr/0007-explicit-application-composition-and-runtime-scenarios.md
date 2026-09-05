# ADR 0007: Use explicit application composition and runtime Scenarios

- Status: accepted
- Date: 2026-09-03

## Context

A generated dependency registry would keep scaffold refresh automatic, but it
would hide application composition, require AST-aware mutation or runtime
registration, and blur ownership between generated infrastructure and user
behavior. API and Kafka processes also need different root lifecycles without
constructing every runnable branch eagerly.

## Decision

All Devctl-owned Go scaffold files use the `*.gen.go` suffix and the standard
generated-file header. User-owned files use ordinary `.go` names and are
Scaffold Seeds. Managed `provideX` functions register infrastructure. A
create-once `provideApplication` function calls them explicitly and is the
user-owned composition list. Later refreshes may add new per-component Seeds
but never edit that list; the user adds each new provider call deliberately.
Missing registration fails normally during dependency resolution. Devctl does
not analyze Go syntax or maintain a runtime plugin registry.

Named registrations use unexported, namespaced Canonical DI Keys such as
`db-connection:analytics`, `kafka-consumer:audit`,
`kafka-producer:audit`, `http-client:billing`, and
`grpc-client:billing`. The constants remain internal to the composition
package. Each Kafka consumer has a create-once provider binding that selects
its message type, decoder, handler, and retry policy; `provideApplication`
calls that binding.

Managed scenario code defines `Scenario`, which owns a dependency graph and
selected runner and provides `Run(ctx)` and `Shutdown(ctx)`. `NewAPI(ctx)`
resolves an unnamed API Runtime whose lifecycle tasks contain HTTP, gRPC,
health, and pprof work. `NewConsumer(ctx, name)` validates the Manifest-derived
consumer name and enabled toggle before resolution, calls `provideApplication`,
and resolves only the selected runner by its Canonical DI Key. Providers are
registered eagerly but constructed lazily. Selecting a disabled consumer is a
typed scaffolded-application error and process exit 1, not a Devctl CLI public
error category.

Application routes and services are registered through typed `HTTPRegistrar`
and `GRPCRegistrar` interfaces. Managed runtime code resolves these registrars
and applies them to the Echo and gRPC servers. Outbound clients expose raw
transport handles and base addresses: HTTP exposes `*http.Client` and its base
URL, while gRPC exposes `*grpc.ClientConn`. Application code constructs the
generated OpenAPI clients and Proto stubs it needs.

Database Variants share one named logical provider that switches on effective
config and opens only the selected backend. A logical Connection cannot mix a
ClickHouse Variant with transactional Variants. Kafka producers are ordinary
`*kafka.Producer[[]byte]`; topic config is injected into application types
that need it rather than hidden behind a generated wrapper.

Kafka consumer infrastructure uses a managed generic provider that resolves a
named `kafka.ConsumerConfig`, `kafka.Decoder[T]`,
`kafka.BatchHandler[T]`, and `retry.Policy`, then registers the named runner.
Per-consumer managed config maps generated Runtime Config and registers the
default exponential policy. Managed helpers cover raw bytes, JSON, and Proto;
Proto support and its dependency are conditional. Raw decoding is zero-copy
because the handler contract forbids retaining a batch. Schema-backed
consumers still default to `[]byte`; the user opts into actual generated JSON
or Proto types in the Provider Binding. Buf output receives no Devctl facade or
type-alias file.

The initial raw handler Seed has no infrastructure dependencies and returns a
permanent `ErrNotImplemented`, making an unimplemented consumer fail closed
instead of acknowledging messages silently.

## Consequences

Composition remains readable Go and user intent survives scaffold refresh.
Adding a Component requires adding its explicit Provider Binding call, and the
generated documentation must include that checklist. Scenario resolution
constructs only the selected runnable branch while still allowing shared
infrastructure providers.

We reject AST mutation of user code, implicit runtime registries, exported DI
key constants, generated wrappers around generated clients, and no-op default
handlers.
