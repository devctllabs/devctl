package scaffold_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	scaffolddomain "github.com/devctllabs/devctl/internal/domain/scaffold"
	manifestrepo "github.com/devctllabs/devctl/internal/repository/manifest"
	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/scaffold"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFilesystemRepoPreflightsAndPublishesRenderedArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repository := workspacerepo.NewFilesystemRepo()
	service := scaffold.New(zap.NewNop(), fixedProjectRepository{project: projectdomain.Project{Root: root, Manifest: projectdomain.Manifest{
		Project: projectdomain.Identity{Name: "sample", Language: "go"}, Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/sample"}},
	}}}, repository)
	changes, err := service.Scaffold(context.Background(), scaffolddomain.Command{})
	require.NoError(t, err)
	require.NotEmpty(t, changes.Files)
	requireGoldenFile(t, filepath.Join(root, "cmd/sample/main.go"), "testdata/minimal/cmd/sample/main.go")
	requireGoldenFile(t, filepath.Join(root, "go.mod"), "testdata/minimal/go.mod")
	changes, err = service.Scaffold(context.Background(), scaffolddomain.Command{})
	require.NoError(t, err)
	for _, change := range changes.Files {
		require.Equal(t, scaffolddomain.FileUnchanged, change.Action)
	}
}

func requireGoldenFile(t *testing.T, actualPath, goldenPath string) {
	t.Helper()
	actual, err := os.ReadFile(actualPath)
	require.NoError(t, err)
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, golden, actual)
}

func TestFilesystemRepoCreatesCLIFoundation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("version: 1\nproject: {name: sample-cli, language: go}\nenv: {}\npaths: {external_contracts: api/external}\nsources: {}\nexports: {}\ncomponents:\n  logging: {}\nlanguages:\n  go:\n    module: github.com/acme/sample-cli\n    generators:\n      config: {out: gen/config}\n"), 0o644))

	result, err := scaffoldProject(context.Background(), manifestPath)

	require.NoError(t, err)
	require.NotEmpty(t, result.Files)
	for _, path := range []string{"go.mod", ".mise.toml", ".golangci.yml", "cmd/sample-cli/main.go", "internal/deps/container.gen.go"} {
		_, err := os.Stat(filepath.Join(root, path))
		require.NoError(t, err, path)
	}
}

func TestFilesystemRepoRefreshPreservesApplicationAndAddsConsumerSeeds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	initialManifest := []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`)
	require.NoError(t, os.WriteFile(manifestPath, initialManifest, 0o644))
	_, err := scaffoldProject(context.Background(), manifestPath)
	require.NoError(t, err)
	applicationPath := filepath.Join(root, "internal/deps/application.go")
	require.NoError(t, os.WriteFile(applicationPath, []byte("package deps\n\n// user composition\n"), 0o644))

	withConsumer := []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources: {}
exports: {}
components:
  kafka:
    consumers:
      - name: audit
        topic: sample.audit.events.v1
        contract: {format: raw}
languages:
  go: {module: example.test/sample}
`)
	require.NoError(t, os.WriteFile(manifestPath, withConsumer, 0o644))
	result, err := scaffoldProject(context.Background(), manifestPath)
	require.NoError(t, err)

	application, err := os.ReadFile(applicationPath)
	require.NoError(t, err)
	require.Equal(t, "package deps\n\n// user composition\n", string(application))
	requireFileChange(t, result, "internal/deps/application.go", scaffolddomain.FileUnchanged)
	requireFileChange(t, result, "internal/deps/consumer_audit.go", scaffolddomain.FileCreated)
	_, err = os.Stat(filepath.Join(root, "internal/transport/consumerkafka/audit/handler.go"))
	require.NoError(t, err)
}

func requireFileChange(t *testing.T, result scaffolddomain.Result, path string, action scaffolddomain.FileAction) {
	t.Helper()
	for _, change := range result.Files {
		if change.Path == path {
			require.Equal(t, action, change.Action)
			return
		}
	}
	require.Fail(t, "scaffold change missing", path)
}

func TestFilesystemRepoCreatesGoLibsDBProviders(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample-api, language: go}
env: {prefix: SAMPLE_}
paths: {external_contracts: api/external}
sources: {}
exports: {}
components:
  health: {server: {start: {env: HEALTH_ENABLED, default: true}}}
  telemetry: {start: {env: TELEMETRY_ENABLED, default: false}}
  db:
    connections:
      - name: primary
        default: sqlite
        variants:
          - {name: sqlite, kind: sqlite, dsn_default: 'file:./data/app.db?_foreign_keys=on'}
          - {name: postgres, kind: postgres, secret: true}
languages:
  go: {module: github.com/acme/sample-api}
`), 0o644))

	result, err := scaffoldProject(context.Background(), manifestPath)
	require.NoError(t, err)
	require.NotEmpty(t, result.Files)
	apiSource, err := os.ReadFile(filepath.Join(root, "cmd/sample-api/internal/api.go"))
	require.NoError(t, err)
	for _, declaration := range []string{
		"func NewCmdAPI() *cli.Command", "deps.NewAPI(ctx)", "scenario.Run(ctx)",
	} {
		require.Contains(t, string(apiSource), declaration)
	}

	storage, err := os.ReadFile(filepath.Join(root, "internal/deps/storage_primary.gen.go"))
	require.NoError(t, err)
	selectors := selectorNames(t, storage)
	for _, expected := range []string{
		"sqlitedb.Open", "postgresdb.Open", "di.ProvideNamedResource", "di.ProvideNamed[txmanager.Managers]",
	} {
		if expected == "di.ProvideNamed[txmanager.Managers]" {
			expected = "di.ProvideNamed"
		}
		require.Contains(t, selectors, expected)
	}
	_, err = os.Stat(filepath.Join(root, "data/.gitkeep"))
	require.NoError(t, err)
}

func selectorNames(t *testing.T, source []byte) map[string]struct{} {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors)
	require.NoError(t, err)
	selectors := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok {
			selectors[identifier.Name+"."+selector.Sel.Name] = struct{}{}
		}
		return true
	})
	return selectors
}

func scaffoldProject(ctx context.Context, manifestPath string) (scaffolddomain.Result, error) {
	adapter := workspacerepo.NewFilesystemRepo()
	projects := projectservice.New(zap.NewNop(), projectservice.Dependencies{
		Manifests: manifestrepo.NewFilesystemRepo(), Locator: adapter,
	})
	service := scaffold.New(zap.NewNop(), projects, adapter)
	result, err := service.Scaffold(ctx, scaffolddomain.Command{ManifestPath: manifestPath})
	if err != nil {
		return result, fmt.Errorf("service.Scaffold: %w", err)
	}
	return result, nil
}

type fixedProjectRepository struct{ project projectdomain.Project }

func (r fixedProjectRepository) LoadProject(context.Context, string) (projectdomain.Project, error) {
	return r.project, nil
}
