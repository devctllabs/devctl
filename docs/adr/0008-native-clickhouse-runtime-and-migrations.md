# ADR 0008: Use the native ClickHouse runtime and separate migrations

- Status: accepted
- Date: 2026-09-03

## Context

ClickHouse's native Go API provides capabilities and operational semantics that
do not fit the transactional `database/sql` abstraction used by SQLite and
PostgreSQL. Migration tooling, however, already integrates through a separate
driver and migration URL.

## Decision

ClickHouse runtime connections use
`github.com/ClickHouse/clickhouse-go/v2` v2.48.0 and expose the native
`driver.Conn`. Construction parses the DSN, opens the connection, performs an
eager ping, and registers owned close behavior. The connection is named with
the same `db-connection:<name>` convention as other database Resources, and a
narrow health checker delegates to `Ping`. ClickHouse does not receive the
transaction manager, read/write endpoint split, or generated repository
getter used for transactional databases; application repositories resolve the
named native connection and define their own narrow interfaces.

ClickHouse migrations remain supported through golang-migrate's ClickHouse
database driver and a migration-only secret URL. They retain the same Project
surface as other databases: a default migration directory, an explicit path
override, and `--no-migrations`. Runtime native-driver config and migration
driver config are independent.

ClickHouse migrations are non-transactional. Documentation recommends a
migration DSN with `x-multi-statement=true` when a file contains multiple
statements, but Devctl neither appends nor enforces that parameter. The known
semicolon-splitting limitations and the possibility of partial application and
manual recovery must be documented.

## Consequences

Runtime code retains ClickHouse-native behavior instead of presenting a false
transactional abstraction. Applications that support several database kinds
must keep ClickHouse separate from transactional Variants within one logical
Connection. Migration operators explicitly own multi-statement and recovery
trade-offs.

We reject `database/sql` as the ClickHouse runtime contract, generated
transaction helpers for ClickHouse, and silent DSN mutation.

## References

- [clickhouse-go README](https://github.com/ClickHouse/clickhouse-go)
- [Native driver.Conn API](https://pkg.go.dev/github.com/ClickHouse/clickhouse-go/v2@v2.48.0/lib/driver#Conn)
- [golang-migrate ClickHouse driver](https://github.com/golang-migrate/migrate/blob/master/database/clickhouse/README.md)
