# ADR 0001: Use one canonical Project model and explicit command boundaries

- Status: accepted
- Date: 2026-09-03

## Context

Project bootstrap, manifest mutation, contract acquisition, linting, code
generation, and scaffolding have different side effects and readiness needs.
Allowing each workflow to interpret the Manifest independently would duplicate
defaults and paths, while implicit chaining would make a command's filesystem
and network effects difficult to predict.

## Decision

The v1 CLI has exactly eight top-level commands: `init`, `validate`, `inspect`,
`enable`, `add`, `sync`, `lint`, and `gen`. Initialization is explicit through
`init manifest` and `init scaffold`. Manifest mutations do not install tools,
run generators, or change handwritten Go. `sync`, `lint`, and `gen` never
invoke one another implicitly.

`devctl.yaml` is the canonical desired-state Manifest. The Project service is
the only gateway for manifest discovery, decoding, semantic validation,
inspection, and mutation. Structural decoding issues, semantic validity, and
Project Readiness are distinct. `validate` checks all three; other workflows
load the semantically valid Project and check only their own prerequisites.

The project domain projects the Manifest into one immutable, total, and
deterministically ID-sorted Target Catalog. The catalog owns effective Target
IDs, families, roles, references, source locations, Logical Inputs, outputs,
defaults, and supported operations. A valid committed snapshot may add an
optional Resolved Input for inspection without changing the Logical Input.
Missing committed metadata does not make `inspect` fail.

Workflows select and execute catalog Targets instead of deriving these facts
again. Unknown families are `invalid_input`; a known family with no applicable
Targets succeeds with an empty result; an unknown explicit Target ID is
`not_found`; and an existing Target that lacks the requested operation is
`unsupported`. Local sync is a supported no-op. Raw Kafka Targets do not claim
sync support.

The catalog remains sorted by Target ID for stable inspection and selection.
Generation separately executes `config`, HTTP server/client, gRPC
server/client, then Kafka consumer/producer Targets so dependency-sensitive
work remains explicit.

The dependency direction is `cmd -> service -> domain`. Delivery code invokes
one service root per executable command. Consumer-owned ports isolate
repositories, external tools, and protocol analyzers, while `internal/deps` is
the production composition root.

## Consequences

Adding a Component or Target requires one domain projection change followed by
the relevant inspect, sync, lint, generation, and scaffold adapters. Invalid
references remain visible to validation instead of disappearing from a partial
catalog. Readiness failures remain attributable to the workflow that actually
needs the missing file or tool.

We reject command aliases with hidden orchestration, workflow-local copies of
Target defaults, and selectors whose meaning changes by workflow.
