# ADR 0005: Use one canonical Runtime Config catalog

- Status: accepted
- Date: 2026-09-03

## Context

Config generation, scaffold templates, and `inspect` previously derived
environment keys, defaults, types, and field names independently. Their
policies diverged, so a Managed Output refresh could generate Go code that did
not match the runtime consumers created by scaffold.

## Decision

The project domain owns an immutable, key-sorted Runtime Config catalog derived
from one Manifest. It is the sole policy owner for effective prefixes, keys,
types, defaults, secret markers, semantic Go field paths, and the runtime,
example, and inspect projections. Conflicting declarations and colliding Go
field paths are typed domain errors reported by `validate` as
`runtime_config_conflict`.

Every Go Project has an implicit `config` Target. Its default Managed Output is
`gen/config/config.gen.go`; `languages.go.generators.config.out` only overrides
that location. Newly initialized Manifests omit the redundant default block.
`devctl gen config` and `devctl init scaffold` use the same renderer for one
grouped generated `Config` type and the root `.env.example`. Scaffold emits an
adapter that imports the generated type rather than maintaining a second
schema. `inspect` projects its environment list from the same catalog.

No `start` block means a Capability is always on and has no environment toggle.
A present `start` block creates a toggle; if it omits `default`, its effective
default is `false`. Construction and startup are separate: disabled gateable
Capabilities may be constructed but do not run. A selected disabled Kafka
consumer is rejected before dependency resolution, so its client is not
constructed. Ecosystem-standard OpenTelemetry keys retain the `OTEL_*` prefix.

Each Kafka consumer receives runtime fields for enabled state, group, topic,
batch maximum size, batch flush interval, retry maximum attempts, retry maximum
elapsed time, retry initial and maximum delays, rebalance and drain timeouts,
and shutdown timeout. The Manifest topic is the runtime default and may be
overridden through environment. `CommitRetry` inherits the main retry policy;
`OnReject` is fixed to `RejectStop`; rarer library knobs remain available only
through user-owned custom environment and explicit composition until they
become Manifest features.

Migration database environment belongs only to example and inspect
projections. It is absent from application Runtime Config, and the migration
URL is distinct from the runtime database connection. Secret fields never
carry defaults: generated tags omit them, `.env.example` leaves them blank,
and inspect reports only the secret marker.

## Consequences

Generated field paths are semantic and grouped, such as `cfg.HTTP.Address`,
`cfg.Telemetry.ServiceVersion`, and `cfg.DBPrimary.PostgresDSN`. Projects using
the former flat scaffold config regenerate scaffold and config together.
Adding environment-backed behavior requires one catalog policy and projection
tests.

We reject separate scaffold and generation config builders, hidden default
topic changes, and compatibility flat fields with no independent lifetime.
