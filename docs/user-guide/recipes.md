# Recipes

These recipes show the Manifest mutation step. Follow each with `init
scaffold`, `validate`, `sync`, `lint`, or `gen` only when the changed surface
requires it.

## Databases

```sh
devctl add db primary --kind sqlite
devctl add db primary --kind postgres --default
devctl add db analytics --kind clickhouse
```

Migration targets default to `migrations/<connection>/<variant>`. Override the
path with `--migrations-path`, or opt out with `--no-migrations`.

A Connection with several Variants must name a default. ClickHouse cannot be
mixed with SQLite or PostgreSQL Variants in the same logical Connection because
its native runtime is not transactional.

## Redis

```sh
devctl add redis cache
devctl add redis sessions \
  --addr-env SESSIONS_REDIS_ADDR \
  --addr-default localhost:6380
```

The default address is `localhost:6379`. Addresses may be `host:port` or a
`redis`/`rediss` URL without embedded credentials.

## S3

```sh
devctl add s3-connection assets --credentials ambient
devctl add s3 uploads --connection assets
```

Credential modes are `ambient` and `static`. When `add s3` omits
`--connection`, Devctl creates the canonical local static Connection if
needed.

## Contract Sources

```sh
# Project-local containment root
devctl add source contracts --type local --path api/contracts

# HTTPS closure rooted at a URL
devctl add source public-api \
  --type url \
  --url https://contracts.example.com/ \
  --filename openapi.yaml

# Repository checkout
devctl add source shared \
  --type git \
  --repo https://github.com/acme/contracts.git \
  --ref v1.4.0 \
  --path services

# Named Exports from another Devctl Project
devctl add source platform \
  --type devctl \
  --repo https://github.com/acme/platform-contracts.git \
  --ref v1.4.0
```

URL Sources require HTTPS unless `--allow-insecure-http` is explicitly set.
Git Sources may select a containment path. Devctl Sources forbid `path` and
are consumed through named Exports.

## HTTP and gRPC clients

```sh
devctl add http-client billing \
  --source contracts \
  --path billing/openapi.yaml

devctl add grpc-client ledger \
  --source platform \
  --export ledger-grpc
```

Use `--path` for local, URL, and Git Sources. Use `--export` for a Devctl
Source. A custom `--buf-gen-config` is user-owned and must exist before
validation.

## Kafka endpoints

```sh
devctl add kafka-consumer audit \
  --topic orders.audit.v1 \
  --format raw

devctl add kafka-producer created \
  --topic orders.created.v1 \
  --format json \
  --source contracts \
  --path orders-created.schema.json

devctl add kafka-consumer billing \
  --topic billing.events.v1 \
  --format proto \
  --source platform \
  --export billing-events \
  --message acme.billing.v1.Event \
  --encoding binary
```

`raw` has no Contract files and supports inspect/lint but not sync or code
generation. JSON Schema roots need a non-empty `title`. Proto encoding is
`binary` or `json`.

## Targeted previews

```sh
devctl inspect
devctl sync http --target http-client:billing --dry-run
devctl gen grpc --target grpc-client:ledger --dry-run
```

Copy Target IDs from `inspect`; do not derive them from output paths.
