# ADR 0009: Execute migrations through an explicit Devctl command

- Status: accepted
- Date: 2026-10-19

Devctl adds `migrate create/up/down/status` as its ninth top-level command.
This amends the fixed command list in
[ADR 0001](0001-canonical-project-model-and-command-boundaries.md) and the
project-owned external migration executable in
[ADR 0004](0004-project-owned-generator-toolchain.md). Generated Mise tasks
retain their names but call Devctl, so local execution and CI share selection,
configuration, output, and failure rules. Scaffold and application startup never
apply migrations.

A shared Project-domain projection derives ordered Migration Targets from
database Connections and Variants, including SQL paths and prefixed environment
keys. Inspect, scaffold, and execution consume it independently of the Target
Catalog and application Runtime Config, preserving the boundary in
[ADR 0005](0005-canonical-runtime-config.md). Selection and migration-only URL
resolution follow [PDR 0008](../product/decisions/0008-explicit-migration-target-selection.md);
the service uses semantic Project loading without unrelated generator readiness
checks.

`go-libs/dbmigrate` owns SQL execution through caller-registered upstream drivers
instead of invoking a Project-installed migration executable. Devctl owns
configuration resolution, contained timestamped file creation, and CLI result
mapping. Status uses the same backend constructor as execution, accepting its
metadata and SQLite file initialization effects as recorded in
[PDR 0009](../product/decisions/0009-migration-status-and-recovery.md).

The source filesystem stays open until the migrator closes; operation and close
errors are joined. State observed before and after an operation supports partial
results without attempting to undo completed SQL. Creation uses rooted exclusive
file operations and a version reservation because SQL is user-owned and must
never be replaced; ordinary failure cleans up owned files, but a pair is not
atomic across crashes.

SQL cancellation and closing follow the library's waiting semantics; driver
timeouts remain operator-owned. PostgreSQL, SQLite, and the ClickHouse SQL
migration driver support builds with CGO disabled. ClickHouse remains separate
from the native application connection in
[ADR 0008](0008-native-clickhouse-runtime-and-migrations.md), non-transactional,
and receives unchanged SQL and URLs. Migration errors preserve causes
internally but do not render SQL or credentials, including under verbose logging.

The current operation, path, no-change, dirty-state, and recovery rules are
maintained in the [migration product document](../product/migrations.md).
