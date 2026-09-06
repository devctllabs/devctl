# sample-api

This project foundation is scaffolded by Devctl.

## Bootstrap

```sh
mise install
go mod download all
go mod tidy
devctl lint
devctl gen
go mod tidy
mise run check
```

Inspect the application commands with `go run ./cmd/sample-api --help`.

## Updating the foundation

- Run `devctl sync` after changing remote sources.
- Run `devctl init scaffold` after changing components in `devctl.yaml`.
- Run `devctl gen` after changing API or schema contracts.

Devctl replaces files ending in `*.gen.go`. Ordinary `.go` files and this
README are created once, so application code and local notes are preserved.

When a component adds a provider seed, review it and call the provider from
`internal/deps/application.go`. That file is the user-owned composition root;
Devctl does not rewrite its provider list.
