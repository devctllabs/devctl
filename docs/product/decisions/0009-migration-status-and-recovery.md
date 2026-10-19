# Report dirty migration state and keep recovery explicit

Status: Accepted
Date: 2026-10-19

Operators need to inspect migration state after a failure, so `status` succeeds
when a database is dirty while `up` and `down` reject that state. Status uses
the normal migration backend and accepts metadata initialization, including
creation of a SQLite database file, to keep state interpretation consistent
with execution. Recovery remains an explicit operator action outside Devctl
because partial SQL effects cannot be safely inferred or repaired automatically.

## Consequences

Status exit success does not imply a clean database or an absence of writes;
automation must inspect `dirty`. Devctl provides no force or automatic repair
operation, and an operation error does not guarantee that no SQL took effect.

Current behavior is described in [Migrations](../migrations.md).
