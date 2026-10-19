# Expose structured command outcomes for automation

Status: Accepted
Date: 2026-09-03

Automation needs stable outcomes without parsing human messages, so JSON mode
emits result and error events with defined streams, failure categories, and exit
codes. Validation and lint findings remain normal result data even when they
produce exit code `1`. Completed progress after a later failure belongs in an
error's partial result, preserving recovery information without presenting the
whole invocation as successful.

The rationale is recorded in
[ADR 0006](../../adr/0006-machine-readable-cli-contract.md). Current behavior is
described in [CLI automation](../cli-automation.md).
