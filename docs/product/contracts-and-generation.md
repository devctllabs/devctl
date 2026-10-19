# Contracts and generation

Devctl manages OpenAPI 3.1, Proto, JSON Schema, and raw Kafka Contract surfaces.
A Contract Reference selects content from a bounded Source. External updates
become reviewable repository changes through explicit synchronization.

## Sources and Exports

| Source | Selection and acquisition |
| --- | --- |
| Local | File closures or Proto trees inside a Project-local containment root; sync succeeds without publication |
| URL | A URL-based OpenAPI or JSON Schema closure with a virtual committed Entrypoint |
| Git | A repository and revision, optionally narrowed to a contained path; supports file closures and Proto trees |
| Devctl | A repository and revision containing an upstream Project; selects a named Export, not a Source path |

Ordinary Sources use a relative Contract path. Devctl Sources use an Export
whose surface must match the upstream effective HTTP server, gRPC server Module
Root, or Kafka producer. Only the selected upstream surface needs to be
materializable; unrelated upstream Readiness is not required.

Raw Kafka Targets have no Source, Contract path, or Export reference and do not
materialize a Snapshot. Cross-project schema-backed Kafka consumption selects
JSON or Proto explicitly.

URL acquisition requires HTTPS unless insecure HTTP was explicitly allowed.
Credentials are forbidden. Requests and redirects retain the original scheme,
host, and effective port; only relative references are followed, while absolute
references are ignored. Limits are 64 documents, 32 MiB per response, and
64 MiB per Snapshot. Query values distinguish fetch identities but are redacted
from diagnostics; fragments do not distinguish fetch identities. Distinct URLs
that collide at one committed path fail rather than overwrite one another.

## Committed Contract state

`sync` materializes external Contract Snapshots and publishes them under the
Project's managed external-contract namespace. Users review and commit that
state. A full or family sync may remove stale Target children; a targeted sync
never prunes siblings. `--dry-run` previews planned publication and removal
without acquiring Sources, invoking tools, or changing files.

Snapshots retain their Entrypoint or Module Root and relevant supplier Buf
configuration and lock file. Devctl-sourced gRPC and schema-backed Kafka
snapshots carry `.devctl-contract.json` metadata describing their selected
surface:

| Snapshot | Metadata fields |
| --- | --- |
| gRPC Proto | `kind: grpc`, `format: proto`, `module_root`, `buf_config` |
| Kafka JSON | `kind: kafka`, `format: json`, `topic`, `entrypoint` |
| Kafka Proto | `kind: kafka`, `format: proto`, `topic`, `entrypoint`, `module_root`, `buf_config` |

Metadata rejects unknown fields and trailing JSON values. Paths are normalized,
relative, contained, and must resolve to the declared Snapshot content; a Proto
Kafka Entrypoint stays inside its Module Root. `buf.lock` is a Snapshot file,
not a metadata field. Local and HTTP/OpenAPI inputs have no sidecar.

Required metadata that is missing, malformed, or inconsistent fails downstream
execution with `snapshot_metadata_invalid` and guidance to run `devctl sync`;
Devctl does not rediscover a schema heuristically.

`lint`, `gen`, and inspection consume local or committed inputs without
reacquiring the configured supplier. This separates Contract updates from their
consumption, as recorded in
[PDR 0002](decisions/0002-committed-contract-snapshots.md). External tools may
still need network access for their own modules or plugins; operation without
network access also requires prepared tool dependencies and caches.

## Lint and generation

Lint checks Contract structure and conventions: OpenAPI operations and successful
responses, Proto naming and Buf lint rules, Kafka topic shape, and JSON Schema
validity and root `title`. Local Kafka schema filenames must be `<topic>.json`
or `<topic>.proto`; directories may vary, while the basename must match. This
filename rule does not apply to external Sources. Findings are normal output
with exit code `1` when invalid. Lint never synchronizes or generates.

| Target input | Generator |
| --- | --- |
| Runtime Config | Shared Devctl config renderer |
| HTTP server/client OpenAPI | Project-declared oapi-codegen |
| gRPC and Kafka Proto | Project-declared Buf and its generator configuration |
| Kafka JSON Schema | Project-installed quicktype; root `title` owns the Go type name |
| Raw Kafka | No generated Contract code |

The consuming Project owns tool versions and native generator configuration;
see [PDR 0004](decisions/0004-project-owned-toolchain.md). Canonical native
configs are Managed Outputs. Alternate configs are user-owned, must exist for
validation/generation, and are not created or replaced by scaffold. Multiple
Targets may consume one user-owned config. Supplier Buf configuration and the
consumer's `*.gen.yaml` have separate ownership.

Generation executes config, HTTP server/client, gRPC server/client, then Kafka
consumer/producer Targets. Tools write into temporary storage before managed
publication. Each selected output tree replaces its previous tree and removes
stale files inside that boundary; unrelated Target trees remain untouched.
Config generation also publishes `.env.example` as a separate managed file.

Publication is atomic at each file or tree boundary, not across the whole
workflow. A later failure preserves completed changes and returns their partial
result. Dry-run does not execute generators or publish outputs. See
[scaffold and ownership](scaffold-and-ownership.md), the
[workflow guide](../user-guide/contracts-and-generation.md), and the
[Source reference](../user-guide/reference/manifest/sources-and-exports.md).
