# Migrations

Devctl creates and executes user-owned SQL migrations for SQLite, PostgreSQL,
and ClickHouse database Variants. Execution is an explicit command; scaffold,
generation, and application startup do not apply migrations.

## Operations

| Operation | Behavior |
| --- | --- |
| `migrate create <name>` | Creates an empty timestamped `.up.sql` / `.down.sql` pair in the selected migration directory. Does not resolve a database URL or connect to a database. |
| `migrate up` | Applies all pending migrations by default, or a positive number selected by `--steps N`. Repeating an unbounded successful application succeeds with `no_change: true`. An empty SQL source fails. |
| `migrate down` | Rolls back one migration by default, or a positive number selected by `--steps N`. No confirmation is requested. A database with no applied version fails. |
| `migrate status` | Reports the current version and dirty state. No applied version is represented by `version: null`. Dirty state is a successful status result. |

Each operation loads a semantically valid Manifest. Unrelated generator tools,
generated code, and application module readiness are not migration prerequisites.
Common command options include `--file`, `--target`, `--json`, and `--verbose`;
`--env-file` is available on `up`, `down`, and `status`. See the
[command reference](../user-guide/reference/commands.md) for the complete CLI.

## Selecting a Migration Target

A Migration Target belongs to one database Connection and Variant and is
addressed as `connection:variant`. Only Variants with a `migrations` block are
eligible. With no `--target`, Devctl selects automatically only when exactly
one eligible Variant exists in the whole Manifest. A runtime default
Connection does not influence migration selection.

Multiple eligible Variants require an explicit target and produce an
`invalid_input` error listing candidates. No eligible target or an unknown
selector produces `not_found`; explicitly selecting an existing Variant with
migrations disabled produces `unsupported`. Each invocation operates on one
target. CI should always provide `--target`.

The reason for this selection policy is recorded in
[PDR 0008](decisions/0008-explicit-migration-target-selection.md).

## Database configuration and paths

The selected Variant declares `migrations.path`, `migrations.database_env`, and
an optional `migrations.database_default`. The environment key includes the
Project's effective environment prefix. The URL is resolved in this order:

1. A present process environment key.
2. That key in the explicitly supplied dotenv file.
3. The Manifest default.

Presence controls precedence: an empty process or file value fails instead of
falling back. An explicitly supplied file must be readable and valid even when
the process environment already supplies the URL. Reading it does not change
the process environment. Devctl does not load `.env` implicitly or use an
application runtime DSN as a fallback.

URLs must match the selected backend: `sqlite://`, `postgres://` or
`postgresql://`, or `clickhouse://`. Devctl passes the URL through without
rewriting its path or driver parameters. The same Manifest target can therefore
be used across environments by supplying a different migration URL.

SQL paths are relative to the selected Manifest's directory and must stay
within that Project. Relative `--env-file` paths and relative SQLite database
URLs retain the invoking working directory's meaning, including when `--file`
selects a Manifest elsewhere. There is no command-line SQL directory override.

## SQL ownership and creation

SQL files remain user-owned, including after scaffold runs. `create` makes a
missing migration directory and names its pair
`YYYYMMDDhhmmss_<name>.{up,down}.sql` using UTC with second precision. Names
start with an ASCII letter or digit and contain only ASCII letters, digits,
underscores, and hyphens.

An existing version, concurrent creation in the same second, or an existing
destination produces a conflict without overwriting SQL. Ordinary creation
failure attempts to remove files created by that invocation; a process crash
can leave an incomplete pair or a `.devctl-create-<timestamp>` reservation for
operator inspection.

Scaffold manages the Mise wrappers `migrate:<connection>:<variant>:` followed
by `create`, `up`, `down`, or `status`. They invoke Devctl with an explicit
target; no separate migration executable is required.

## State, failures, and recovery

Opening a target for `status` may initialize its migration metadata table and,
for SQLite, create the database file. Status is therefore a successful state
inspection with possible initialization effects. Both `up` and `down` reject
dirty state as a conflict. Repair and force operations remain outside Devctl;
see [PDR 0009](decisions/0009-migration-status-and-recovery.md).

A failed command can leave changes in the database. In particular, bounded
`up` or `down` requesting more steps than exist can apply available steps
before reporting the shortage. Devctl does not roll back completed migrations
after a later failure. When changes are observed, the error includes the
available partial result; SQL and credentials are omitted from diagnostics even
with `--verbose`.
Success reports the selected target and either created file paths or the
observed version and dirty state, following the
[output and error contract](../user-guide/reference/output-and-errors.md).

Cancellation is checked before migration work; an in-flight SQL operation and
resource closing may wait for the driver. Operators configure timeouts through
the database driver URL. ClickHouse migrations are non-transactional and may
need manual recovery after partial application. Multi-statement files require
appropriate driver configuration, commonly `x-multi-statement=true`; Devctl
does not add it, and the driver's semicolon splitting has limitations.
