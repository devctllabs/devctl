# Sources and Exports

## `sources`

Sources are named map entries:

```yaml
sources:
  contracts:
    type: local
    path: api/contracts
```

Every Source has `type`. Other fields are type-specific:

| Type | Required | Optional | Forbidden or ignored |
|---|---|---|---|
| `local` | `path` | `proto.buf_config` | URL and repository fields |
| `url` | `url` | `filename`, `allow_insecure_http` | `path`, repository fields |
| `git` | `repo`, `ref` | `path`, `proto.buf_config` | URL fields |
| `devctl` | `repo`, `ref` | none | `path`, URL fields; consumers use `export` |

### Source fields

| Field | Description |
|---|---|
| `type` | `local`, `url`, `git`, or `devctl` |
| `path` | Relative containment root for local or Git content |
| `url` | Initial fetch URL for a URL closure |
| `filename` | Virtual committed filename for a single URL document |
| `allow_insecure_http` | Allows `http`; HTTPS is required by default |
| `repo` | Git clone location or upstream Devctl checkout |
| `ref` | Reviewable repository revision |
| `proto.buf_config` | Source-relative supplier Buf config |

Credential-bearing URLs are rejected. URL references may fetch only relative
same-origin documents and are limited to 64 documents, 64 MiB total, and 32
MiB per response. Query participates in fetch identity but is redacted from
diagnostics; fragments do not participate. Absolute references are ignored.

For Proto Snapshots, the effective supplier Buf config and adjacent `buf.lock`
travel with the Contract. They are separate from consumer-owned `*.gen.yaml`.

## `exports`

Exports publish exact Project surfaces to a downstream Devctl Source:

```yaml
exports:
  public-api:
    kind: openapi
    path: api/openapi/swagger.yaml
  billing-grpc:
    kind: grpc
    path: api/proto/grpc
  order-events:
    kind: kafka
    producer: orders
```

| Kind | Required | Meaning |
|---|---|---|
| `openapi` | `path` | Must equal the effective HTTP server Entrypoint |
| `grpc` | `path` | Must equal the effective gRPC server Module Root |
| `kafka` | `producer` | Names an existing producer and inherits topic/format |

`producer` is forbidden for OpenAPI/gRPC. `path` is forbidden for Kafka.
Local Project validation checks every Export. When a downstream Project syncs
one Export from a Devctl Source, only that selected upstream surface must be
materializable; unrelated upstream Readiness is not required.
