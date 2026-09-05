# ADR 0006: Use structured events as the machine-readable CLI contract

- Status: accepted
- Date: 2026-09-03

## Context

Automation needs deterministic result and failure records while humans need
safe diagnostics. Success already uses structured log events, but late
failures currently place partial results in the same `data` field as success,
and adapters can collapse specific source failures into broader categories.

## Decision

`--json` emits compact JSONL. A successful command emits exactly one structured
event on stdout with `level`, `ts`, `msg`, and its command-specific payload in
`data`. It does not emit a bare result object. Diagnostic events may precede
the final event only under `--verbose`.

Usage, execution, and cancellation failures emit one final event on stderr
with stable `code` and `exit_code` fields. Safe scalar context belongs in
`details`. A late workflow failure places the completed operation result in
`details.partial_result`; the reporter safely merges it with existing details,
top-level error `data` is removed, and stdout remains empty. This pre-v1 wire
change has no compatibility shim. Raw causes and external tool output appear
only under `--verbose`.

Invalid `validate` and `lint` findings are normal results: the structured event
is written to stdout, stderr is empty, and the process exits `1`. Execution
errors also exit `1`, usage/help exits `2`, cancellation exits `130`, and
success exits `0`.

Public failure codes remain `usage`, `invalid_input`, `not_found`, `conflict`,
`unavailable`, `unsupported`, `cancelled`, and `internal`. Adapters add safe
operation, Target, Source, field, and path context without replacing a more
specific underlying category. Partial results are attached only after actual
progress, never merely after planning or validation.

## Consequences

Callers can distinguish successful data from recovery information and branch
on stable failure categories without parsing messages. CLI DTO, reporter, and
end-to-end tests must change together.

We reject bare success objects, mixed stdout error payloads, top-level error
`data`, and reducing every source acquisition failure to `unavailable`.
