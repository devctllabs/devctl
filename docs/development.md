# Developing Devctl

This guide is for contributors to the Devctl repository. Project authors using
the CLI should start with the [user guide](user-guide/README.md).

## Local checks

Install the repository-pinned tools once, or again after `.mise.toml` changes:

```sh
mise install
```

Run the complete local CI contract through Mise:

```sh
mise run check
```

`check` runs formatting, golangci-lint (including `govet`), dependency-graph
hygiene, race-enabled Go tests, build, generated documentation, the Orders API
example, and e2e workflows. The ordinary `mise run test` task remains
available for faster non-race iteration.

The pull request CI runs this complete check on Linux and cross-builds the
Devctl binary for macOS amd64 and arm64 with CGO disabled. Native macOS jobs
are intentionally not part of the regular CI because the repository has no
platform-specific implementation. The release workflow still builds and
publishes native macOS release archives for both architectures.

Regenerate the CLI reference after changing visible command metadata:

```sh
mise run docs:generate
```

## Working with sibling modules

For local work across sibling `devctl` and `go-libs` checkouts, create a Go
workspace outside either repository, for example in their parent directory:

```sh
go work init ./devctl ./go-libs/...
```

Use that workspace only for interactive development. Do not commit `go.work`
or add `replace` directives to the released module. Before submitting, rerun
the checks above with `GOWORK=off` so they exercise the dependency graph used
by CI and release builds.

## Ownership boundaries

Devctl uses `github.com/devctllabs/go-libs/filesystem` for rooted file
operations, `go-libs/log` for structured logging, and `go-libs/di` for command
scenario graphs. Atomic file publication, Git, process execution, and external
tool boundaries remain owned by Devctl. Completed Target or scaffold changes
are not rolled back after a later failure.
