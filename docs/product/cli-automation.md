# CLI automation

Devctl commands are non-interactive and expose structured outcomes for local
tools and CI. Users explicitly choose each workflow and inspect its result
before retrying a failed operation. The automation decision is recorded in
[PDR 0006](decisions/0006-structured-cli-contract.md).

## Invocation and preview

Executable leaves accept `--file`, `--json`, and `--verbose`. Flags and positional
arguments are validated before constructing the command's service graph. Help
does not require a Project or initialized integration resources.

Text is the default output encoding. `--json` selects compact JSON Lines;
`--verbose` independently enables diagnostic detail. Common Manifest discovery
and target selection are described in [Project lifecycle](project-lifecycle.md).

`sync` and `gen` offer `--dry-run`. Preview plans from local state without Source
acquisition, generator execution, publication, or pruning. It reports planned
actions, not a promise that later execution will succeed against changed remote
or workspace state.

## Results, findings, and errors

| Outcome | Output and exit behavior |
| --- | --- |
| Success | One final stdout event; command payload under `data`; exit `0` |
| Invalid validate/lint findings | Normal stdout result containing findings; exit `1` |
| Execution failure | Final stderr error with stable `code` and `exit_code`; stdout empty; exit `1` |
| Invalid CLI usage | Final stderr error; exit `2` |
| Cancellation | Final stderr error; exit `130` |

JSON success is an event with `level`, `ts`, and `msg`, not a bare result object.
Errors expose safe context under `details`, and recovery information under
`details.partial_result`; they have no top-level `data`. Diagnostic events can
appear under verbose mode. Callers branch on stable failure categories rather
than human-readable messages. Exact fields and codes remain in the
[output reference](../user-guide/reference/output-and-errors.md).

For validation and lint, exit `1` can mean completed analysis with findings.
Automation must inspect that command's stdout result instead of treating every
nonzero exit as an execution-error envelope.

## Partial work and diagnostics

Targets execute sequentially. Completed publication or SQL is not automatically
undone after a later operation or shutdown failure. A partial result describes
available completed progress, not a successful whole invocation. Planned work
alone does not justify a partial result. Review observed state before retrying.

Ordinary diagnostics suppress raw causes and external tool output unless
verbose mode is selected. Migration SQL and credentials remain suppressed even
under verbose mode. Typed internal causes are retained for classification and
cancellation handling.

SIGINT and SIGTERM cancel the Devctl command context. Cancellation does not
guarantee rollback or prompt interruption of every backend operation; migration
SQL and closing follow their documented
[driver waiting semantics](migrations.md#state-failures-and-recovery).
