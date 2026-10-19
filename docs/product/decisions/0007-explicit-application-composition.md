# Keep application composition explicit and user-owned

Status: Accepted
Date: 2026-09-03

Automatic registry wiring would hide application intent and require runtime
discovery or mutation of user code. Generated infrastructure therefore exposes
providers and typed protocol boundaries, while a user-owned composition list
selects application behavior and lazy Scenarios construct the selected runtime
branch. Unimplemented Kafka handlers fail closed rather than acknowledge
messages without application work.

## Consequences

Adding a Component can create new binding Seeds but requires an explicit provider
call in existing composition. Refresh preserves those choices instead of
silently activating newly available behavior.

The rationale is recorded in
[ADR 0007](../../adr/0007-explicit-application-composition-and-runtime-scenarios.md).
Current behavior is described in [Generated application](../generated-application.md).
