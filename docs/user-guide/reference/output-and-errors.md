# Output and errors

Every executable leaf accepts `--json`; text output is the default.
`--verbose` independently enables debug diagnostics and raw causes.

## JSONL success

JSON mode emits compact JSON Lines, not a bare result object. A successful
command emits exactly one final info event on stdout:

```json
{"level":"info","ts":0,"msg":"project validation completed","command":"validate","data":{"valid":true,"issues":[]}}
```

Diagnostic events may precede the final event only with `--verbose`.

## Findings

Invalid `validate` and `lint` findings are normal results. The complete event
is written to stdout, stderr is empty, and the process exits `1`. Do not treat
every exit status `1` as an execution-error envelope; inspect stdout first for
these two commands.

## Errors

Usage, execution, and cancellation failures emit one final event on stderr.
stdout is empty for execution failures.

```json
{"level":"error","ts":0,"msg":"internal error","code":"internal","exit_code":1,"details":{"partial_result":{"targets":["config"],"changes":[],"dry_run":false}}}
```

Safe scalar context appears in `details`. Raw causes and external tool output
appear only with `--verbose`. Error events never carry a top-level `data`
field.

## Exit codes

| Exit code | Meaning |
|---:|---|
| `0` | Success |
| `1` | Validation/lint findings or execution failure |
| `2` | Invalid CLI usage or help failure |
| `130` | Cancellation |

## Stable error codes

| Code | Meaning |
|---|---|
| `usage` | Invalid command, argument, or flag usage |
| `invalid_input` | Input is understood but invalid for the operation |
| `not_found` | Requested Project object, Target, Source, or file is absent |
| `conflict` | Existing state prevents the requested change |
| `unavailable` | External source or dependency cannot be reached |
| `unsupported` | The selected object does not support the operation |
| `cancelled` | Context or process cancellation |
| `internal` | Unexpected failure without a safer public category |

## Command result payloads

The command-specific payload is under `data` on success and may appear under
`details.partial_result` after real partial progress.

| Commands | Payload shape |
|---|---|
| `init manifest`, `enable`, `add` | `{manifest, change}` |
| `init scaffold` | `{files: [{path, action}]}` |
| `validate` | `{valid, issues}` |
| `inspect` | `{project: {...}}` |
| `lint` | `{valid, contracts, issues}` |
| `sync`, `gen` | `{targets, changes, dry_run}` |

Publication actions are `created`, `updated`, `unchanged`, and `removed`.
Static previews use `planned_publish` and `planned_remove`.

## Partial results

A partial result appears only when work completed before a later execution or
shutdown failure. It is recovery information, not success output. Review it
before retrying because already-published Targets are not rolled back.
