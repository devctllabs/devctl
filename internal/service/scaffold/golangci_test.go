package scaffold

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPlanConfiguresGoQualityChecks(t *testing.T) {
	t.Parallel()

	artifacts := plannedArtifacts(t, minimalScaffoldManifest())
	var config struct {
		Run struct {
			Tests bool `yaml:"tests"`
		} `yaml:"run"`
		Linters struct {
			Default    string                    `yaml:"default"`
			Enable     []string                  `yaml:"enable"`
			Settings   map[string]map[string]any `yaml:"settings"`
			Exclusions struct {
				Generated string           `yaml:"generated"`
				Paths     []string         `yaml:"paths"`
				Rules     []map[string]any `yaml:"rules"`
			} `yaml:"exclusions"`
		} `yaml:"linters"`
		Formatters struct {
			Enable []string `yaml:"enable"`
		} `yaml:"formatters"`
	}
	require.NoError(t, yaml.Unmarshal(artifacts[".golangci.yml"].Content, &config))
	require.True(t, config.Run.Tests)
	require.Equal(t, "none", config.Linters.Default)
	require.Subset(t, config.Linters.Enable, []string{"cyclop", "gocognit", "godoclint", "inamedparam", "revive", "bodyclose", "contextcheck", "staticcheck"})
	require.Equal(t, 10, config.Linters.Settings["cyclop"]["max-complexity"])
	require.Equal(t, 15, config.Linters.Settings["gocognit"]["min-complexity"])
	require.Equal(t, map[string]any{
		"default": "none", "enable": []any{"require-doc"},
		"options": map[string]any{"require-doc": map[string]any{"ignore-unexported": false}},
	}, config.Linters.Settings["godoclint"])
	require.Equal(t, false, config.Linters.Settings["inamedparam"]["skip-single-param"])
	require.Equal(t, []any{
		map[string]any{"name": "argument-limit", "arguments": []any{4}},
		map[string]any{"name": "function-result-limit", "arguments": []any{3}},
		map[string]any{"name": "exported", "arguments": []any{
			"check-private-receivers", "check-public-interface", "disable-stuttering-check",
			"disable-checks-on-constants", "disable-checks-on-variables",
		}},
	}, config.Linters.Settings["revive"]["rules"])
	require.Equal(t, "strict", config.Linters.Exclusions.Generated)
	require.Equal(t, []string{"^gen/"}, config.Linters.Exclusions.Paths)
	require.Len(t, config.Linters.Exclusions.Rules, 2)
	for _, rule := range config.Linters.Exclusions.Rules {
		require.Equal(t, []any{"godoclint"}, rule["linters"], "exclusions must preserve complexity and revive checks in tests")
	}
	entrypoints := config.Linters.Exclusions.Rules[1]
	for _, source := range []string{
		"func TestQuality(t *testing.T)      { t.Helper() }",
		"func TestMain(m *testing.M)         { m.Run() }",
		"func BenchmarkQuality(b *testing.B) { b.Helper() }",
		"func FuzzQuality(f *testing.F)      { f.Add(1) }",
		"func Example()                      {}",
	} {
		require.Regexp(t, entrypoints["source"], source)
	}
	require.NotRegexp(t, entrypoints["source"], "func (helper) TestQuality(t *testing.T) { t.Helper() }")
	require.Equal(t, []string{"gofmt"}, config.Formatters.Enable)
	require.False(t, artifacts[".golangci.yml"].CreateOnly)
}

func TestPlanDocumentsApplicationAndKafkaSeeds(t *testing.T) {
	t.Parallel()

	artifacts := plannedArtifacts(t, fullCharacterizationManifest())
	for _, path := range []string{
		"internal/deps/application.go",
		"internal/transport/consumerkafka/audit/handler.go",
	} {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, artifacts[path].Content, parser.ParseComments)
		require.NoError(t, err)
		documentation, err := doc.NewFromFiles(fileSet, []*ast.File{file}, "", doc.AllDecls)
		require.NoError(t, err)
		require.NotEmpty(t, documentation.Types, path)
		for _, declaration := range documentation.Types {
			require.NotEmpty(t, declaration.Doc, "%s: %s needs Go doc", path, declaration.Name)
			for _, method := range declaration.Methods {
				require.NotEmpty(t, method.Doc, "%s: %s needs Go doc", path, method.Name)
			}
			for _, constructor := range declaration.Funcs {
				require.NotEmpty(t, constructor.Doc, "%s: %s needs Go doc", path, constructor.Name)
			}
		}
	}
}
