# Project, environment, paths, and Go tooling

## `project`

```yaml
project:
  name: orders-api
  language: go
```

| Field | Required | Values |
|---|:---:|---|
| `project.name` | yes | Kebab-case Project name |
| `project.language` | yes | `go` |

The name owns the default Runtime Config prefix. `orders-api` becomes
`ORDERS_API_`.

## `env`

```yaml
env:
  prefix: ORDERS_
  custom:
    - group: Payments
      vars:
        - key: PAYMENTS_TIMEOUT
          type: duration
          default: 5s
        - key: PAYMENTS_TOKEN
          type: string
          secret: true
```

| Field | Required | Description |
|---|:---:|---|
| `env.prefix` | no | Overrides the Project-derived prefix |
| `env.custom[].group` | yes | Semantic group used in generated Go fields |
| `env.custom[].vars` | yes | Environment variables in the group |
| `vars[].key` | yes | Environment key before effective prefixing |
| `vars[].type` | no | `string` (default), `bool`, `int`, or `duration` |
| `vars[].default` | no | Typed runtime default |
| `vars[].secret` | no | Marks a secret and suppresses rendered defaults |

Component `env.system` and `env.custom` entries use the same variable shape,
without the outer `group` field. Conflicting effective keys or generated Go
field paths produce `runtime_config_conflict`.

Keys beginning with `OTEL_` retain that ecosystem prefix instead of receiving
the Project prefix. Secret values never carry defaults in generated config,
`.env.example`, or `inspect`.

## `paths`

```yaml
paths:
  external_contracts: api/external
```

`paths.external_contracts` is optional and defaults to `api/external`. The
complete subtree belongs to `sync` and contains one child tree per external
Target.

## `languages.go`

```yaml
languages:
  go:
    module: example.com/orders-api
    generators:
      config:
        out: gen/config/config.gen.go
      http:
        oapi_config: tools/oapi/server.yaml
        server_out: gen/serverhttp
        client_out: gen/clienthttp
      grpc:
        out: gen/grpc
        buf_gen_config: tools/buf/grpc.gen.yaml
      kafka:
        out: gen/kafka
        buf_gen_config: tools/buf/kafka.gen.yaml
    components:
      pprof:
        server:
          start:
            env: PPROF_ENABLED
            default: false
        env:
          system:
            - key: PPROF_ADDR
              type: string
              default: 127.0.0.1:6060
```

| Field | Required | Effective default |
|---|:---:|---|
| `languages.go.module` | yes | none |
| `generators.config.out` | no | `gen/config/config.gen.go` |
| `generators.http.oapi_config` | no | `tools/oapi/server.yaml` |
| `generators.http.server_out` | no | `gen/serverhttp` |
| `generators.http.client_out` | no | `gen/clienthttp` |
| `generators.grpc.out` | no | `gen/grpc` |
| `generators.grpc.buf_gen_config` | no | `tools/buf/grpc.gen.yaml` |
| `generators.kafka.out` | no | `gen/kafka` |
| `generators.kafka.buf_gen_config` | no | `tools/buf/kafka.gen.yaml` |

Every Go Project has an implicit config Target even if `generators.config` is
omitted. Canonical native generator configs are Managed Outputs. An explicitly
selected alternate config is user-owned, must already exist, and is never
created or replaced by scaffold.

`languages.go.components.pprof` uses the same `start` and component `env`
shapes described in [Components](components.md). The HTTP service preset uses
`127.0.0.1:6060` and a disabled-by-default `PPROF_ENABLED` toggle.
