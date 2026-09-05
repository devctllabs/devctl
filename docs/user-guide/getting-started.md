# Build an Orders API with PostgreSQL

This walkthrough starts with an empty directory and ends with a running HTTP
API that writes to PostgreSQL. It exercises the complete local Devctl workflow:
Manifest creation, scaffolding, Contract linting, generation, a migration, and
user-owned application code.

The finished result is checked in at
[`examples/orders-api`](../../examples/orders-api/README.md).

## Prerequisites

Install Go 1.26, Git, Mise, and Docker. Then install Devctl:

```sh
go install github.com/devctllabs/devctl/cmd/devctl@latest
devctl --version
```

## 1. Create the Project

```sh
mkdir orders-api
cd orders-api

devctl init manifest \
  --lang go \
  --preset http-service \
  --name orders-api \
  --module example.com/orders-api

devctl add db primary --kind postgres
devctl init scaffold
mise install
go mod tidy
```

At this point `devctl.yaml` is the desired state, files ending in `*.gen.go`
belong to Devctl, and ordinary `.go` files belong to you. The database command
also added PostgreSQL configuration and migration tasks to the Project.

## 2. Define the HTTP Contract

Replace `api/openapi/swagger.yaml` with this OpenAPI 3.1 Contract:

```yaml
openapi: 3.1.0
info:
  title: Orders API
  version: 1.0.0
paths:
  /orders:
    post:
      operationId: createOrder
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateOrder"
      responses:
        "201":
          description: Order created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Order"
  /orders/{id}:
    get:
      operationId: getOrder
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
            format: int64
            minimum: 1
      responses:
        "200":
          description: Order found
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Order"
        "404":
          description: Order not found
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Problem"
components:
  schemas:
    CreateOrder:
      type: object
      additionalProperties: false
      required: [customer_name, total_cents]
      properties:
        customer_name:
          type: string
          minLength: 1
        total_cents:
          type: integer
          format: int64
          minimum: 0
    Order:
      type: object
      additionalProperties: false
      required: [id, customer_name, total_cents, created_at]
      properties:
        id:
          type: integer
          format: int64
        customer_name:
          type: string
        total_cents:
          type: integer
          format: int64
        created_at:
          type: string
          format: date-time
    Problem:
      type: object
      additionalProperties: false
      required: [message]
      properties:
        message:
          type: string
```

Lint the Contract and generate the strict Echo server interface:

```sh
devctl lint
devctl gen
go mod tidy
```

`devctl lint` validates the Contract but does not generate code. `devctl gen`
publishes the generated server to `gen/serverhttp/server.gen.go`.

## 3. Create the database migration

Create a timestamped migration pair:

```sh
mise run migrate:primary:postgres:create create_orders
```

Put this in the new `.up.sql` file:

```sql
CREATE TABLE orders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer_name text NOT NULL,
    total_cents bigint NOT NULL CHECK (total_cents >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
```

Put this in the matching `.down.sql` file:

```sql
DROP TABLE orders;
```

## 4. Implement the user-owned code

Create an `internal/orders` package that contains:

- an `Order` domain value and a small `Store` interface;
- a PostgreSQL store using `*postgresdb.Endpoint`;
- a handler implementing `serverhttp.StrictServerInterface`;
- a mapping from the domain value to the generated API type.

The complete implementation is
[`internal/orders/orders.go`](../../examples/orders-api/internal/orders/orders.go).
Its important boundary is deliberately small:

```go
type Store interface {
    Create(ctx context.Context, customerName string, totalCents int64) (Order, error)
    Get(ctx context.Context, id int64) (Order, error)
}

type Handler struct {
    store Store
}
```

Then edit the user-owned `internal/deps/application.go`. Resolve the reader and
writer endpoints published by the generated database provider, construct the
store and handler, and register the strict server:

```go
reader, err := di.ResolveNamed[*postgresdb.Endpoint](
    resolver,
    storagePrimaryConnectionName+".reader",
)
// handle err
writer, err := di.ResolveNamed[*postgresdb.Endpoint](
    resolver,
    storagePrimaryConnectionName+".writer",
)
// handle err

store := orders.NewPostgresStore(reader, writer)
app := &application{orders: orders.NewHandler(store)}
```

```go
func (a *application) RegisterHTTP(server *echo.Echo) {
    strict := serverhttp.NewStrictHandler(a.orders, nil)
    serverhttp.RegisterHandlers(server, strict)
}
```

See the complete
[`application.go`](../../examples/orders-api/internal/deps/application.go) and
the handler tests in
[`handler_test.go`](../../examples/orders-api/internal/orders/handler_test.go).

Verify that the Project compiles before starting infrastructure:

```sh
go test ./...
```

## 5. Start PostgreSQL and apply the migration

Start an isolated PostgreSQL container:

```sh
docker run --rm --detach \
  --name devctl-orders-postgres \
  --publish 5432:5432 \
  --env POSTGRES_USER=orders \
  --env POSTGRES_PASSWORD=orders \
  --env POSTGRES_DB=orders \
  postgres:18.6-alpine3.23
```

Wait until it is ready:

```sh
docker exec devctl-orders-postgres pg_isready -U orders -d orders
```

Configure both the runtime pool and the migration tool, then migrate:

```sh
export ORDERS_API_DB_PRIMARY_KIND=postgres
export ORDERS_API_DB_PRIMARY_POSTGRES_DSN='postgres://orders:orders@127.0.0.1:5432/orders?sslmode=disable'
export ORDERS_API_DB_PRIMARY_POSTGRES_MIGRATIONS_URL="$ORDERS_API_DB_PRIMARY_POSTGRES_DSN"

mise run migrate:primary:postgres:up
```

The generated `.env.example` lists every runtime key. Secrets have empty
values and must be supplied through the environment or your secret manager.

## 6. Run and call the API

Start the API in one terminal:

```sh
go run ./cmd/orders-api api
```

Create an order from another terminal:

```sh
curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"customer_name":"Ada","total_cents":1250}' \
  http://127.0.0.1:8080/orders
```

The response has status `201` and contains the assigned ID:

```json
{"created_at":"2026-09-04T19:30:00Z","customer_name":"Ada","id":1,"total_cents":1250}
```

Use that ID to read the row back:

```sh
curl --fail-with-body http://127.0.0.1:8080/orders/1
```

Stop the API with Ctrl-C, then stop PostgreSQL:

```sh
docker stop devctl-orders-postgres
```

## What to do next

- Read [concepts and ownership](concepts-and-ownership.md) before editing
  generated files.
- Follow the [Project workflow](project-workflow.md) when adding Components.
- Use [contracts and generation](contracts-and-generation.md) for local and
  external Contract changes.
- Use the [recipes](recipes.md) to add more Resources and clients.
