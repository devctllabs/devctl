package manifest_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/repository/manifest"
	"github.com/stretchr/testify/require"
)

func TestFilesystemRepoLoadMinimalManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	err := os.WriteFile(manifestPath, []byte("version: 1\nproject:\n  name: example\n  language: go\nenv: {}\npaths:\n  external_contracts: api/external\nsources: {}\nexports: {}\ncomponents: {}\nlanguages:\n  go:\n    module: github.com/acme/example\n"), 0o644)
	require.NoError(t, err)

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), manifestPath)

	require.NoError(t, err)
	require.Empty(t, loaded.Issues)
	require.Equal(t, "example", loaded.Project.Manifest.Project.Name)
	require.Empty(t, loaded.Project.Manifest.Env.Prefix)
	require.Equal(t, root, loaded.Project.Root)
}

func TestFilesystemRepoSavesCanonicalManifestIdempotently(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	project := projectdomain.Project{
		Root: root, ManifestPath: manifestPath,
		Manifest: projectdomain.Manifest{
			Version:   1,
			Project:   projectdomain.Identity{Name: "example", Language: "go"},
			Sources:   map[string]projectdomain.Source{},
			Exports:   map[string]projectdomain.Export{},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	repository := manifest.NewFilesystemRepo()

	changed, err := repository.Save(context.Background(), project)
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = repository.Save(context.Background(), project)
	require.NoError(t, err)
	require.False(t, changed)

	loaded, err := repository.Load(context.Background(), manifestPath)
	require.NoError(t, err)
	require.Equal(t, project.Manifest.Project, loaded.Project.Manifest.Project)
	require.Equal(t, project.Manifest.Languages.Go.Module, loaded.Project.Manifest.Languages.Go.Module)
}

func TestFilesystemRepoClassifiesMissingManifest(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.yaml")

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), missing)

	require.ErrorIs(t, err, os.ErrNotExist)
	require.Empty(t, loaded.Issues)
}

func TestFilesystemRepoReturnsMalformedYAMLAsDocumentIssue(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: [\n"), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), path)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.DecodeIssue{{Kind: projectdomain.DecodeYAMLInvalid}}, loaded.Issues)
}

func TestFilesystemRepoReturnsTypeMismatchPosition(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: nope\n"), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), path)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.DecodeIssue{{
		Kind: projectdomain.DecodeSchemaInvalid, Field: "version", Line: 1, Column: 10,
	}}, loaded.Issues)
}

func TestFilesystemRepoReturnsDuplicateKeyPosition(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte("project:\n  name: first\n  name: second\n"), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), path)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.DecodeIssue{{
		Kind: projectdomain.DecodeDuplicateKey, Field: "project.name", Line: 3, Column: 3,
	}}, loaded.Issues)
}

func TestFilesystemRepoAggregatesUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte("unknown_root: true\nproject:\n  unknown_project: true\n"), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), path)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.DecodeIssue{
		{Kind: projectdomain.DecodeUnknownField, Field: "unknown_root", Line: 1, Column: 1},
		{Kind: projectdomain.DecodeUnknownField, Field: "project.unknown_project", Line: 3, Column: 3},
	}, loaded.Issues)
}

func TestLoadManifestMapsYAMLDocumentToProjectSpec(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: example, language: go}
env: {prefix: EXAMPLE_}
paths: {external_contracts: contracts/external}
sources:
  remote:
    type: url
    url: http://example.test/openapi.yaml
    filename: remote.yaml
    allow_insecure_http: true
exports: {}
components:
  http:
    clients:
      - name: remote
        source: remote
        path: openapi.yaml
        base_url_env: REMOTE_BASE_URL
        oapi_config: tools/oapi/remote.yaml
  db:
    connections:
      - name: primary
        default: sqlite
        kind_env: DB_PRIMARY_KIND
        variants:
          - name: sqlite
            kind: sqlite
            dsn_env: DB_PRIMARY_SQLITE_DSN
            dsn_default: file:./data/primary.db
            migrations:
              path: migrations/primary/sqlite
              database_env: DB_PRIMARY_SQLITE_MIGRATIONS_URL
              database_default: sqlite://./data/primary.db
  redis:
    connections:
      - name: cache
        addr_env: REDIS_CACHE_ADDR
        addr_default: redis://localhost:6379/1
languages:
  go:
    module: github.com/acme/example
    generators:
      http:
        oapi_config: tools/oapi/server.yaml
        server_out: gen/server
        client_out: gen/client
`), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), manifestPath)

	require.NoError(t, err)
	require.Empty(t, loaded.Issues)
	require.Equal(t, root, loaded.Project.Root)
	require.Equal(t, manifestPath, loaded.Project.ManifestPath)
	spec := loaded.Project.Manifest
	require.Equal(t, "contracts/external", spec.Paths.ExternalContracts)
	require.True(t, spec.Sources["remote"].AllowInsecureHTTP)
	require.Equal(t, "REMOTE_BASE_URL", spec.Components.HTTP.Clients[0].BaseURLEnv)
	require.Equal(t, "tools/oapi/remote.yaml", spec.Components.HTTP.Clients[0].OAPIConfig)
	require.Equal(t, "DB_PRIMARY_KIND", spec.Components.DB.Connections[0].KindEnv)
	require.Equal(t, "DB_PRIMARY_SQLITE_DSN", spec.Components.DB.Connections[0].Variants[0].DSNEnv)
	require.Equal(t, &projectdomain.DBMigrations{
		Path: "migrations/primary/sqlite", DatabaseEnv: "DB_PRIMARY_SQLITE_MIGRATIONS_URL",
		DatabaseDefault: "sqlite://./data/primary.db",
	}, spec.Components.DB.Connections[0].Variants[0].Migrations)
	require.Equal(t, projectdomain.RedisConnection{
		Name: "cache", AddrEnv: "REDIS_CACHE_ADDR", AddrDefault: "redis://localhost:6379/1",
	}, spec.Components.Redis.Connections[0])
	require.Equal(t, "gen/server", spec.Languages.Go.Generators.HTTP.ServerOut)
	require.Equal(t, "gen/client", spec.Languages.Go.Generators.HTTP.ClientOut)
}

func TestFilesystemRepoRejectsRemovedRedisFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`components:
  redis:
    instances: []
    connections:
      - name: cache
        addr_env: REDIS_CACHE_ADDR
        default: true
`), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), path)

	require.NoError(t, err)
	fields := make([]string, len(loaded.Issues))
	for index, issue := range loaded.Issues {
		require.Equal(t, projectdomain.DecodeUnknownField, issue.Kind)
		fields[index] = issue.Field
	}
	require.Equal(t, []string{
		"components.redis.instances",
		"components.redis.connections[0].default",
	}, fields)
}

func TestMutationRejectsUnknownExtensionFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	original := `version: 1
project: {name: example, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  users:
    # transport note
    type: git
    repo: old
    ref: main
    x-owner: contracts
exports: {}
components:
  x-component: {keep: true}
languages:
  go: {module: github.com/acme/example}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(original), 0o644))

	loaded, err := manifest.NewFilesystemRepo().Load(context.Background(), manifestPath)
	require.NoError(t, err)
	require.Equal(t, []projectdomain.DecodeIssue{
		{Kind: projectdomain.DecodeUnknownField, Field: "sources.users.x-owner", Line: 11, Column: 5},
		{Kind: projectdomain.DecodeUnknownField, Field: "components.x-component", Line: 14, Column: 3},
	}, loaded.Issues)
	updated, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Contains(t, string(updated), "# transport note")
	require.Contains(t, string(updated), "x-owner: contracts")
	require.Contains(t, string(updated), "x-component: {keep: true}")
	require.Contains(t, string(updated), "repo: old")
}
