package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestPlanMatchesGoldenTrees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manifest projectdomain.Manifest
		seeds    []string
	}{
		{name: "minimal", manifest: minimalScaffoldManifest(), seeds: []string{
			"README.md", "cmd/sample/main.go", "internal/deps/application.go",
		}},
		{name: "full", manifest: fullCharacterizationManifest(), seeds: []string{
			"README.md",
			"api/openapi/swagger.yaml",
			"cmd/sample-api/internal/api.go",
			"cmd/sample-api/internal/consumer.go",
			"cmd/sample-api/main.go",
			"data/.gitkeep",
			"internal/deps/application.go",
			"internal/deps/consumer_audit.go",
			"internal/deps/consumer_invoice.go",
			"internal/transport/consumerkafka/audit/handler.go",
			"internal/transport/consumerkafka/invoice/handler.go",
			"migrations/analytics/clickhouse/.gitkeep",
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			artifacts, err := plan(test.manifest)
			require.NoError(t, err)
			goldenRoot := filepath.Join("testdata", test.name)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				writeGoldenTree(t, goldenRoot, artifacts)
			}
			expected := readGoldenTree(t, goldenRoot)

			actual := make(map[string]string, len(artifacts))
			actualPaths := make([]string, 0, len(artifacts))
			actualSeeds := make([]string, 0, len(test.seeds))
			for _, artifact := range artifacts {
				require.NotContains(t, actual, artifact.Path, "duplicate artifact path")
				require.Equal(t, fs.FileMode(0o644), artifact.Mode, artifact.Path)
				actual[artifact.Path] = string(artifact.Content)
				actualPaths = append(actualPaths, artifact.Path)
				if artifact.CreateOnly {
					actualSeeds = append(actualSeeds, artifact.Path)
				}
			}

			expectedPaths := sortedPaths(expected)
			require.Equal(t, expectedPaths, actualPaths, "artifact paths and deterministic position")
			require.Equal(t, test.seeds, actualSeeds, "artifact ownership")
			for _, path := range expectedPaths {
				require.Equal(t, expected[path], actual[path], path)
			}
		})
	}
}

func writeGoldenTree(t *testing.T, root string, artifacts []Artifact) {
	t.Helper()
	require.NoError(t, os.RemoveAll(root))
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, artifact.Content, artifact.Mode))
	}
}

func readGoldenTree(t *testing.T, root string) map[string]string {
	t.Helper()

	contents := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel: %w", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("os.ReadFile: %w", err)
		}
		contents[filepath.ToSlash(relativePath)] = string(content)
		return nil
	})
	require.NoError(t, err)
	return contents
}

func sortedPaths(contents map[string]string) []string {
	paths := make([]string, 0, len(contents))
	for path := range contents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func fullCharacterizationManifest() projectdomain.Manifest {
	return projectdomain.Manifest{
		Version: 1,
		Project: projectdomain.Identity{Name: "sample-api", Language: "go"},
		Env:     projectdomain.Env{Prefix: "SAMPLE_"},
		Sources: map[string]projectdomain.Source{
			"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
		},
		Components: projectdomain.Components{
			Logging:   &projectdomain.Logging{},
			Health:    &projectdomain.Health{},
			Telemetry: &projectdomain.Telemetry{},
			HTTP: &projectdomain.HTTP{
				Server:  &projectdomain.HTTPServer{OpenAPI: "api/openapi/swagger.yaml"},
				Clients: []projectdomain.HTTPClient{{Name: "catalog", Source: "contracts", Path: "catalog.yaml", BaseURLEnv: "CATALOG_BASE_URL"}},
			},
			GRPC: &projectdomain.GRPC{
				Server:  &projectdomain.GRPCServer{ProtoRoot: "api/proto", BufConfig: "buf.yaml"},
				Clients: []projectdomain.GRPCClient{{Name: "billing", Source: "contracts", Path: "billing.proto", AddrEnv: "BILLING_GRPC_ADDR"}},
			},
			Kafka: &projectdomain.Kafka{
				Consumers: []projectdomain.KafkaConsumer{
					{Name: "audit", Topic: "sample.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"}},
					{Name: "invoice", Topic: "sample.invoice.events.v1", Contract: projectdomain.KafkaContract{Format: "proto", Source: "contracts", Path: "invoice.proto"}},
				},
				Producers: []projectdomain.KafkaProducer{{Name: "events", Topic: "sample.events.v1", Contract: projectdomain.KafkaContract{Format: "json", Source: "contracts", Path: "events.json"}}},
			},
			Redis: &projectdomain.Redis{Connections: []projectdomain.RedisConnection{{Name: "cache", AddrDefault: "localhost:6379"}}},
			S3: &projectdomain.S3{
				Connections: []projectdomain.S3Connection{{Name: "default", Region: "us-east-1"}},
				Buckets:     []projectdomain.S3Bucket{{Name: "media", Connection: "default", Bucket: "media-local"}},
			},
			DB: &projectdomain.DB{Connections: []projectdomain.DBConnection{{
				Name: "primary", Default: "sqlite",
				Variants: []projectdomain.DBVariant{
					{Name: "sqlite", Kind: "sqlite", DSNDefault: "file:./data/app.db?_foreign_keys=on"},
					{Name: "postgres", Kind: "postgres", Secret: true},
				},
			}, {
				Name: "analytics", Default: "clickhouse",
				Variants: []projectdomain.DBVariant{{Name: "clickhouse", Kind: "clickhouse", DSNDefault: "clickhouse://localhost:9000/default", Secret: true, Migrations: &projectdomain.DBMigrations{
					Path: "migrations/analytics/clickhouse", DatabaseEnv: "DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL",
				}}},
			}}},
		},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{
			Module: "example.test/sample-api",
			Generators: projectdomain.GoGenerators{
				GRPC:  &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"},
				Kafka: &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"},
			},
			Components: projectdomain.GoComponents{Pprof: &projectdomain.Pprof{}},
		}},
	}
}
