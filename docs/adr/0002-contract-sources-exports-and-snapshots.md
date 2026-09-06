# ADR 0002: Materialize stable Contract Snapshots from bounded Sources

- Status: accepted
- Date: 2026-09-03

## Context

Contracts can originate locally, at a URL, in Git, or as a named Export from
another Devctl Project. Generation and linting must remain reproducible and
must not silently depend on changing external suppliers. Cross-project Kafka
and gRPC contracts also need enough committed information to be interpreted
without filesystem discovery.

## Decision

A Source is a containment root. A Contract Reference selects a relative
Entrypoint or a named Export, and materialization produces a Snapshot with
`ModuleRoot`, `Entrypoint`, `Files`, and `Metadata`. OpenAPI and JSON Schema
Snapshots have an Entrypoint. A gRPC Export is module-only: it has a Module
Root and no Entrypoint. A Proto-backed Kafka Snapshot may have both, with the
Entrypoint contained by the Module Root.

Local Exports are exact projections of effective Project surfaces. An OpenAPI
Export uses the effective HTTP server OpenAPI path. A gRPC Export uses the
effective gRPC server Proto root. A Kafka producer Export names an existing
producer and inherits that producer's topic and format. A Devctl Source always
addresses the upstream checkout root, forbids `path`, and selects a named
Export. Upstream Manifest loading is structural-only; the selected Export is
validated at materialization time.

For a URL Source, `source.url` is the fetch base while the selected Target path
is the virtual committed Entrypoint. Only relative `$ref` values are followed;
absolute references are ignored even when same-origin. A relative reference is
resolved for fetching from the current document URL and for storage from the
current virtual path. Query participates in fetch identity, fragments do not,
and two distinct query identities that map to the same virtual path are an
invalid collision. Query values are redacted from diagnostics.

Every URL request and redirect must retain the initial scheme, host, and
effective port. Credentials and scheme changes are forbidden. One Snapshot is
limited to 64 documents, 64 MiB aggregate, and 32 MiB per response; Devctl
keeps no persistent URL cache. Policy, reference, limit, and collision failures
map to `invalid_input`; HTTP 404 and 410 map to `not_found`; other non-success
HTTP responses and network, DNS, TLS, or timeout failures map to `unavailable`.

Proto Snapshots carry the effective upstream Buf config declared by
`components.grpc.server.buf_config`, defaulting to `buf.yaml`, plus an adjacent
`buf.lock` when present. No discovery heuristic selects alternate Buf files.

Every Devctl-sourced gRPC or Kafka Managed External Contract has a root
`.devctl-contract.json` sidecar whose relative paths are interpreted from that
root. gRPC metadata records `kind: grpc`, `format: proto`, `module_root`, and
`buf_config`. Kafka metadata records `kind`, `topic`, and `format`; JSON adds
`entrypoint`, Proto adds `entrypoint`, `module_root`, and `buf_config`, and raw
Kafka adds no file fields. `buf.lock` is a Snapshot file, not a metadata field.
Local Sources have no sidecar and local sync is a no-op.

Downstream lint, generation, inspection resolution, and re-export consume the
committed metadata and never rediscover a schema or contact the original
supplier. Missing or invalid required metadata is a stale Snapshot, not an
invalid Contract. It returns `invalid_input` with typed reason
`snapshot_metadata_invalid`, the offending field and reason, and a hint to run
`devctl sync`. Legacy Devctl-sourced gRPC and non-raw Kafka snapshots without
complete metadata are handled the same way. `inspect` remains tolerant and
simply omits Resolved Input until committed metadata is valid.

## Consequences

Contract updates become visible repository changes and builds remain stable
when suppliers are unavailable. Same-origin URL closures support multi-file
specifications without becoming a general web crawler. Existing cross-project
snapshots must be refreshed before downstream operations rely on the new
metadata.

We reject single-document URL semantics, absolute-reference crawling,
implicit network access from lint or generation, arbitrary gRPC first-file
entrypoints, and schema discovery by file count.
