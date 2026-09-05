# Devctl

`devctl` is a non-interactive CLI for defining, validating, and materializing
reproducible Go projects. A checked-in `devctl.yaml` Manifest describes the
Project, its runtime Capabilities and Resources, its API and event Contracts,
and the project-owned tools used to generate code.

Devctl keeps each workflow explicit: changing the Manifest does not install
tools or rewrite application code, and `sync`, `lint`, and `gen` never invoke
one another implicitly.

## Requirements

- Go 1.26
- Git for `git` and `devctl` sources
- [Mise](https://mise.jdx.dev/) for scaffolded toolchains and quality tasks
- Docker for the optional local PostgreSQL walkthrough or the published Devctl image

## Install

Install the latest published Go module:

```sh
go install github.com/devctllabs/devctl/cmd/devctl@latest
devctl --version
```

## Start a Project

```sh
mkdir orders-api
cd orders-api

devctl init manifest \
  --lang go \
  --preset http-service \
  --name orders-api \
  --module example.com/orders-api

devctl add db primary --kind postgres
devctl init scaffold
mise install
go mod tidy
devctl validate
```

The published container image is available at
`ghcr.io/devctllabs/devctl`. Stable releases provide both a SemVer tag and
`latest`; release candidates provide only their version tag:

```sh
docker run --rm ghcr.io/devctllabs/devctl:latest --version
```

The image includes Devctl, Go, Git, Node, and quicktype so a mounted Project
can use Devctl's `sync`, `lint`, and `gen` workflows. Project-declared Go tools
remain owned by that Project and are resolved from its `go.mod`.

Continue with the [HTTP and PostgreSQL getting started
guide](docs/user-guide/getting-started.md) to define an Orders API, apply a
migration, implement the generated server interface, and make a real database
round trip.

The normal Project lifecycle is:

```text
init or mutate Manifest -> scaffold -> validate -> sync -> lint -> gen
```

Only run the steps required by a change. For example, a local server Contract
does not need `sync`, while a changed external Contract does.

## Documentation

- [User guide](docs/user-guide/README.md)
- [Command reference](docs/user-guide/reference/commands.md)
- [Manifest reference](docs/user-guide/reference/manifest/README.md)
- [Output and error contract](docs/user-guide/reference/output-and-errors.md)
- [Development guide](docs/development.md)

Release artifacts are published as `devctl_<version>_<os>_<arch>.tar.gz` for
Linux amd64 and macOS amd64/arm64. Releases use stable and RC lines and are
created as draft GitHub Releases for manual publication.

The documentation on `main` describes the current source tree. For a released
binary, read the documentation at the matching Git tag.

## License

Apache-2.0. See [LICENSE](LICENSE).
