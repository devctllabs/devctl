# Components and Resources

## Shared environment and start policy

Components may declare system and custom Runtime Config entries:

```yaml
env:
  system:
    - key: HTTP_ADDR
      type: string
      default: :8080
  custom:
    - key: HEADER_LIMIT
      type: int
      default: 50
```

Both lists use `key`, optional `type`, optional `default`, and optional
`secret`. See [environment reference](project-and-language.md#env).

Runnable Capabilities use this shape:

```yaml
start:
  env: HTTP_SERVER_ENABLED
  default: true
```

`start.env` is required when `start` exists. An absent `start` means always on.
When `start` exists and `default` is omitted, the effective default is `false`.

## HTTP

```yaml
components:
  http:
    server:
      openapi: api/openapi/swagger.yaml
      start: {env: HTTP_SERVER_ENABLED, default: true}
    clients:
      - name: billing
        source: contracts
        path: billing/openapi.yaml
        base_url_env: BILLING_BASE_URL
        oapi_config: tools/oapi/clients.billing.yaml
    env:
      system:
        - {key: HTTP_ADDR, type: string, default: ":8080"}
```

| Field | Required | Description/default |
|---|:---:|---|
| `server.openapi` | no | Server Entrypoint; `api/openapi/swagger.yaml` |
| `server.start` | no | HTTP Runtime Start Policy |
| `clients[].name` | yes | Unique client name |
| `clients[].source` | yes | Existing Source |
| `clients[].path` | conditional | Entrypoint for non-Devctl Sources |
| `clients[].export` | conditional | Named Export for a Devctl Source |
| `clients[].base_url_env` | no | Runtime base URL key |
| `clients[].oapi_config` | no | Per-client generator config |
| `env` | no | Component Runtime Config |

`path` and `export` are alternatives selected by Source type.

## gRPC

```yaml
components:
  grpc:
    server:
      proto_root: api/proto/grpc
      buf_config: buf.yaml
      start: {env: GRPC_SERVER_ENABLED, default: true}
    clients:
      - name: ledger
        source: shared
        export: ledger-grpc
        proto_root: api/proto/ledger
        buf_gen_config: tools/buf/ledger.gen.yaml
        addr_env: LEDGER_GRPC_ADDR
    env: {}
```

| Field | Required | Description/default |
|---|:---:|---|
| `server.proto_root` | no | Proto Module Root; `api/proto/grpc` |
| `server.buf_config` | no | Supplier Buf config; `buf.yaml` |
| `server.start` | no | gRPC Runtime Start Policy |
| `clients[].name` | yes | Unique client name |
| `clients[].source` | yes | Existing Source |
| `clients[].path` | conditional | Contract path for non-Devctl Sources |
| `clients[].export` | conditional | Named Export for a Devctl Source |
| `clients[].proto_root` | no | Module Root containing the selected path |
| `clients[].buf_gen_config` | no | Consumer generator config |
| `clients[].addr_env` | no | Runtime server address key |
| `env` | no | Component Runtime Config |

Supplier `buf_config`/`buf.lock` and consumer `buf_gen_config` have different
owners. An explicit consumer config is a user-owned file.

## Kafka

```yaml
components:
  kafka:
    consumers:
      - name: audit
        topic: orders.audit.v1
        group_env: KAFKA_AUDIT_GROUP
        start: {env: KAFKA_AUDIT_CONSUMER_ENABLED, default: false}
        contract: {format: raw}
    producers:
      - name: created
        topic: orders.created.v1
        topic_env: KAFKA_CREATED_TOPIC
        contract:
          source: contracts
          path: orders-created.schema.json
          format: json
    env: {}
```

Consumers require `name` and `topic`; `group_env` and `start` are optional.
Producers require `name` and `topic`; `topic_env` is optional. Both use the
same `contract` shape:

| Field | Required | Description |
|---|:---:|---|
| `format` | yes | `raw`, `json`, or `proto` |
| `source` | schema-backed | Existing Source |
| `path` | conditional | Entrypoint for non-Devctl Sources |
| `export` | conditional | Kafka Export for a Devctl Source |
| `proto_root` | Proto only | Module Root containing `path` |
| `message` | Proto only | Fully-qualified message name |
| `encoding` | Proto only | `binary` or `json` |

Raw endpoints have no Source, path, Export, or Proto fields. JSON Schema roots
need a non-empty `title`, which owns the generated Go type name. Kafka Runtime
Config also derives brokers, batching, retry, rebalance, drain, and shutdown
settings from each effective endpoint.

## Databases

```yaml
components:
  db:
    connections:
      - name: primary
        default: postgres
        kind_env: DB_PRIMARY_KIND
        variants:
          - name: postgres
            kind: postgres
            dsn_env: DB_PRIMARY_POSTGRES_DSN
            secret: true
            migrations:
              path: migrations/primary/postgres
              database_env: DB_PRIMARY_POSTGRES_MIGRATIONS_URL
    env: {}
```

| Field | Required | Description |
|---|:---:|---|
| `connections[].name` | yes | Unique logical Connection name |
| `connections[].default` | multiple Variants | Name of the default Variant |
| `connections[].kind_env` | no | Runtime Variant selector key |
| `connections[].variants` | yes | One or more Variants |
| `variants[].name` | yes | Unique name within the Connection |
| `variants[].kind` | yes | `sqlite`, `postgres`, or `clickhouse` |
| `variants[].dsn_env` | no | Runtime DSN key |
| `variants[].dsn_default` | no | Runtime DSN default |
| `variants[].secret` | no | Redacts/suppresses the DSN default |
| `variants[].migrations` | no | Migration target |
| `migrations.path` | yes | Project-relative migration directory |
| `migrations.database_env` | yes | Migration-only database URL key |
| `migrations.database_default` | no | Migration URL default |
| `env` | no | Component Runtime Config |

A single-Variant Connection may omit `default`; multiple Variants require an
existing default. Migration URL schemes must match the database kind.
ClickHouse cannot share a Connection with transactional Variants.

## Redis

```yaml
components:
  redis:
    connections:
      - name: cache
        addr_env: REDIS_CACHE_ADDR
        addr_default: localhost:6379
    env: {}
```

Connections require `name`; `addr_env` and `addr_default` are optional. The
`add redis` defaults are `REDIS_<NAME>_ADDR` and `localhost:6379`. Credential-
bearing Redis URLs are rejected.

## S3

```yaml
components:
  s3:
    connections:
      - name: assets
        credentials: static
        endpoint: http://localhost:9000
        region: us-east-1
        path_style: true
        access_key_env: S3_ASSETS_ACCESS_KEY
        secret_key_env: S3_ASSETS_SECRET_KEY
    buckets:
      - name: uploads
        connection: assets
        bucket: uploads
    env: {}
```

| Field | Required | Description |
|---|:---:|---|
| `connections[].name` | yes | Unique Connection name |
| `connections[].credentials` | no | `ambient` or `static` |
| `connections[].endpoint` | no | Custom service endpoint |
| `connections[].region` | no | AWS region |
| `connections[].path_style` | no | Use path-style bucket addressing |
| `connections[].access_key_env` | static | Access key environment name |
| `connections[].secret_key_env` | static | Secret key environment name |
| `buckets[].name` | yes | Unique logical bucket name |
| `buckets[].connection` | yes | Existing S3 Connection |
| `buckets[].bucket` | no | Physical bucket name |
| `env` | no | Component Runtime Config |

## Logging, health, and telemetry

```yaml
components:
  logging:
    env: {}
  health:
    server:
      start: {env: HEALTH_SERVER_ENABLED, default: true}
    env: {}
  telemetry:
    start: {env: TELEMETRY_ENABLED, default: false}
    env: {}
```

`logging` contains only `env`. `health.server.start` and `telemetry.start` use
the shared Runtime Start Policy. The HTTP service preset also supplies
`HEALTH_ADDR=:8081`; telemetry uses the ecosystem-standard `OTEL_*` keys in
the effective Runtime Config catalog.
