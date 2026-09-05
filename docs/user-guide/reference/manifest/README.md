# Manifest v1 reference

`devctl.yaml` is the canonical desired-state document for a Project. The root
must be a YAML mapping with these fields:

```yaml
version: 1
project: {}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {}
```

| Field | Required | Description |
|---|:---:|---|
| `version` | yes | Manifest format; only `1` is supported |
| `project` | yes | Project identity |
| `env` | yes | Global Runtime Config declarations |
| `paths` | yes | Project-owned path overrides |
| `sources` | yes | Named Contract Sources |
| `exports` | yes | Named surfaces supplied to other Projects |
| `components` | yes | Runtime Components, Capabilities, and Resources |
| `languages.go` | yes | Go module and generator policy |

Unknown and duplicate fields are rejected. YAML scalar, sequence, and mapping
types are checked before semantic validation. Required values, references,
safe relative paths, conflicts, and Project Readiness are checked separately by
`devctl validate`.

Continue with:

- [Project, environment, paths, and Go tooling](project-and-language.md)
- [Sources and Exports](sources-and-exports.md)
- [Components and Resources](components.md)

## Path rules

Manifest paths are project-relative and must remain contained by the Project.
Absolute paths, traversal through `..`, symlinks at managed boundaries, and
non-regular files where files are required are rejected. Managed paths must not
overlap in ways that let one workflow replace another workflow's output.

## Defaults

Omitted optional fields decode to zero values. Effective defaults are applied
by the Project model and may therefore appear in `inspect` even when absent
from YAML. This reference lists effective defaults next to the owning field.
