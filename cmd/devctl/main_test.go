package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/devctllabs/devctl/internal/testutil/testexec"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

var testCLIBinary string

func TestMain(m *testing.M) {
	testRoot, err := os.MkdirTemp("", "devctl-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test directory: %v\n", err)
		os.Exit(1)
	}

	binary := filepath.Join(testRoot, "devctl")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binary, ".")
	output, err := build.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build test CLI: %v\n%s", err, output)
		_ = os.RemoveAll(testRoot)
		os.Exit(1)
	}
	testCLIBinary = binary

	exitCode := m.Run()
	if err := os.RemoveAll(testRoot); err != nil {
		fmt.Fprintf(os.Stderr, "remove test directory: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func TestRootHelp(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	stdout := runCLI(t, binary, "--help")
	for _, commandName := range []string{"init", "validate", "inspect", "enable", "add", "sync", "gen", "lint"} {
		require.Contains(t, stdout, commandName)
	}
	require.NotContains(t, stdout, "--file")
	require.NotContains(t, stdout, "--json")
	require.NotContains(t, stdout, "--verbose")
	require.NotContains(t, stdout, "--format")
}

func TestCanonicalCommandFamiliesExposePlannedLeaves(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)

	tests := []struct {
		args   []string
		leaves []string
	}{
		{args: []string{"enable", "--help"}, leaves: []string{"grpc"}},
		{args: []string{"add", "--help"}, leaves: []string{"grpc-client", "kafka-consumer", "kafka-producer", "redis", "s3-connection", "s3"}},
		{args: []string{"sync", "--help"}, leaves: []string{"grpc", "kafka"}},
		{args: []string{"gen", "--help"}, leaves: []string{"grpc", "kafka"}},
		{args: []string{"lint", "--help"}, leaves: []string{"grpc", "kafka"}},
	}

	for _, test := range tests {
		output := runCLI(t, binary, test.args...)
		for _, leaf := range test.leaves {
			require.Contains(t, output, leaf, "%v", test.args)
		}
	}
}

func TestInitScaffoldCreatesPinnedBufTooling(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  grpc:
    server: {proto_root: api/proto, buf_config: buf.yaml}
languages:
  go:
    module: example.test/sample
    generators:
      grpc: {out: gen/grpc, buf_gen_config: tools/buf/grpc.gen.yaml}
`), 0o644))

	runCLI(t, binary, "init", "scaffold", "--file", manifestPath, "--json")

	goModBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	goMod, err := modfile.Parse("go.mod", goModBytes, nil)
	require.NoError(t, err)
	tools := make([]string, 0, len(goMod.Tool))
	for _, tool := range goMod.Tool {
		tools = append(tools, tool.Path)
	}
	require.ElementsMatch(t, []string{
		"github.com/bufbuild/buf/cmd/buf",
		"google.golang.org/protobuf/cmd/protoc-gen-go",
		"google.golang.org/grpc/cmd/protoc-gen-go-grpc",
	}, tools)
	requiredVersions := map[string]string{}
	for _, dependency := range goMod.Require {
		requiredVersions[dependency.Mod.Path] = dependency.Mod.Version
	}
	require.Equal(t, "v1.72.0", requiredVersions["github.com/bufbuild/buf"])
	require.Equal(t, "v1.36.12", requiredVersions["google.golang.org/protobuf"])
	require.Equal(t, "v1.6.2", requiredVersions["google.golang.org/grpc/cmd/protoc-gen-go-grpc"])

	bufModuleBytes, err := os.ReadFile(filepath.Join(root, "buf.yaml"))
	require.NoError(t, err)
	var bufModule struct {
		Version string `yaml:"version"`
		Modules []struct {
			Path string `yaml:"path"`
		} `yaml:"modules"`
		Lint struct {
			Use    []string `yaml:"use"`
			Except []string `yaml:"except"`
		} `yaml:"lint"`
	}
	require.NoError(t, yaml.Unmarshal(bufModuleBytes, &bufModule))
	require.Equal(t, "v2", bufModule.Version)
	require.Equal(t, "api/proto", bufModule.Modules[0].Path)
	require.Equal(t, []string{"STANDARD"}, bufModule.Lint.Use)
	require.Equal(t, []string{"FILE_LOWER_SNAKE_CASE"}, bufModule.Lint.Except)

	bufGenerateBytes, err := os.ReadFile(filepath.Join(root, "tools/buf/grpc.gen.yaml"))
	require.NoError(t, err)
	var bufGenerate struct {
		Version string `yaml:"version"`
		Plugins []struct {
			Local []string `yaml:"local"`
			Out   string   `yaml:"out"`
		} `yaml:"plugins"`
	}
	require.NoError(t, yaml.Unmarshal(bufGenerateBytes, &bufGenerate))
	require.Equal(t, "v2", bufGenerate.Version)
	require.Equal(t, [][]string{
		{"go", "tool", "protoc-gen-go"},
		{"go", "tool", "protoc-gen-go-grpc"},
	}, [][]string{bufGenerate.Plugins[0].Local, bufGenerate.Plugins[1].Local})
	require.Equal(t, ".", bufGenerate.Plugins[0].Out)
	require.Equal(t, ".", bufGenerate.Plugins[1].Out)
}

func TestEnableGRPCWritesCanonicalManifest(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "enable", "grpc", "--file", manifestPath, "--json")

	manifest := readTestManifest(t, manifestPath)
	require.NotNil(t, manifest.Components.GRPC)
	require.Equal(t, "api/proto/grpc", manifest.Components.GRPC.Server.ProtoRoot)
	require.Equal(t, "buf.yaml", manifest.Components.GRPC.Server.BufConfig)
	require.Equal(t, "GRPC_SERVER_ENABLED", manifest.Components.GRPC.Server.Start.Env)
	require.True(t, *manifest.Components.GRPC.Server.Start.Default)
	require.Equal(t, testGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"}, *manifest.Languages.Go.Generators.GRPC)
}

func TestEnableGRPCAlwaysOmitsStartPolicy(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "enable", "grpc", "--file", manifestPath, "--always")

	manifest := readTestManifest(t, manifestPath)
	require.NotNil(t, manifest.Components.GRPC)
	require.Nil(t, manifest.Components.GRPC.Server.Start)
}

func TestAddGRPCClientWritesCanonicalManifest(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  contracts: {type: local, path: api/contracts}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "grpc-client", "billing", "--file", manifestPath,
		"--source", "contracts", "--path", "billing", "--proto-root", "proto",
		"--buf-gen-config", "tools/buf/billing.gen.yaml", "--addr-env", "BILLING_GRPC_ADDR")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, []testGRPCClient{{
		Name: "billing", Source: "contracts", Path: "billing", ProtoRoot: "proto",
		BufGenConfig: "tools/buf/billing.gen.yaml", AddrEnv: "BILLING_GRPC_ADDR",
	}}, manifest.Components.GRPC.Clients)
	require.Equal(t, testGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"}, *manifest.Languages.Go.Generators.GRPC)
}

func TestAddKafkaEndpointsWritesConsumerAndProducerPolicies(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "kafka-consumer", "billing", "--file", manifestPath,
		"--topic", "billing.events", "--format", "raw", "--group-env", "BILLING_GROUP")
	runCLI(t, binary, "add", "kafka-producer", "audit", "--file", manifestPath,
		"--topic", "audit.events", "--format", "raw", "--topic-env", "AUDIT_TOPIC")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, []testKafkaConsumer{{
		Name: "billing", Topic: "billing.events", GroupEnv: "BILLING_GROUP",
		Start:    &testStart{Env: "KAFKA_BILLING_CONSUMER_ENABLED", Default: boolPointer(false)},
		Contract: testKafkaContract{Format: "raw"},
	}}, manifest.Components.Kafka.Consumers)
	require.Equal(t, []testKafkaProducer{{
		Name: "audit", Topic: "audit.events", TopicEnv: "AUDIT_TOPIC",
		Contract: testKafkaContract{Format: "raw"},
	}}, manifest.Components.Kafka.Producers)
}

func TestAddSourcePersistsNestedBufConfig(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "source", "events", "--file", manifestPath,
		"--type", "local", "--path", "api/events", "--buf-config", "buf.yaml")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, "buf.yaml", manifest.Sources["events"].Proto.BufConfig)
}

func TestValidateAcceptsLocalSourceDirectory(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api", "events"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  events: {type: local, path: api/events}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "validate", "--file", manifestPath)
}

func TestAddKafkaProtoEndpointConfiguresBufGeneration(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  events:
    type: local
    path: api/events
    proto: {buf_config: buf.yaml}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "kafka-producer", "invoice", "--file", manifestPath,
		"--topic", "invoice.events", "--format", "proto", "--source", "events",
		"--path", "proto/invoice.proto", "--proto-root", "proto", "--message", "acme.invoice.v1.Invoice",
		"--encoding", "binary")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, testKafkaContract{
		Source: "events", Path: "proto/invoice.proto", Format: "proto", ProtoRoot: "proto",
		Message: "acme.invoice.v1.Invoice", Encoding: "binary",
	}, manifest.Components.Kafka.Producers[0].Contract)
	require.Equal(t, testGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"}, *manifest.Languages.Go.Generators.Kafka)
}

func TestAddStorageResourcesWritesSafeDefaults(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "redis", "cache", "--file", manifestPath)
	runCLI(t, binary, "add", "s3", "media", "--file", manifestPath)
	runCLI(t, binary, "add", "db", "analytics", "--file", manifestPath, "--kind", "clickhouse")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, []testRedisConnection{{Name: "cache", AddrEnv: "REDIS_CACHE_ADDR", AddrDefault: "localhost:6379"}}, manifest.Components.Redis.Connections)
	require.Equal(t, []testS3Connection{{Name: "default", Credentials: "static", Endpoint: "http://localhost:9000", Region: "us-east-1", PathStyle: true}}, manifest.Components.S3.Connections)
	require.Equal(t, []testS3Bucket{{Name: "media", Connection: "default", Bucket: "media-local"}}, manifest.Components.S3.Buckets)
	require.Equal(t, "clickhouse", manifest.Components.DB.Connections[0].Variants[0].Kind)
	require.Equal(t, "clickhouse://localhost:9000/default", manifest.Components.DB.Connections[0].Variants[0].DSNDefault)
}

func TestAddSQLiteDatabaseConfiguresMigrations(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "db", "primary", "--file", manifestPath, "--kind", "sqlite")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, []testDBVariant{{
		Kind:       "sqlite",
		DSNDefault: "file:./data/primary.db?_foreign_keys=on",
		Migrations: &testDBMigrations{
			Path:            "migrations/primary/sqlite",
			DatabaseEnv:     "DB_PRIMARY_SQLITE_MIGRATIONS_URL",
			DatabaseDefault: "sqlite://./data/primary.db?_pragma=foreign_keys%281%29",
		},
	}}, manifest.Components.DB.Connections[0].Variants)
}

func TestAddDatabaseSupportsMigrationOverrideAndOptOut(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "db", "archive", "--file", manifestPath, "--kind", "postgres", "--migrations-path", "db/archive")
	runCLI(t, binary, "add", "db", "scratch", "--file", manifestPath, "--kind", "sqlite", "--no-migrations")

	manifest := readTestManifest(t, manifestPath)
	require.Equal(t, "archive", manifest.Components.DB.Connections[0].Name)
	require.Equal(t, &testDBMigrations{Path: "db/archive", DatabaseEnv: "DB_ARCHIVE_POSTGRES_MIGRATIONS_URL"}, manifest.Components.DB.Connections[0].Variants[0].Migrations)
	require.Equal(t, "scratch", manifest.Components.DB.Connections[1].Name)
	require.Nil(t, manifest.Components.DB.Connections[1].Variants[0].Migrations)
}

func TestAddRejectsUnsafeMigrationAndRedisOptions(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	tests := []struct {
		args     []string
		exitCode int
	}{
		{[]string{"add", "db", "primary", "--file", manifestPath, "--kind", "sqlite", "--no-migrations", "--migrations-path", "migrations/primary/sqlite", "--json"}, 2},
		{[]string{"add", "db", "primary", "--file", manifestPath, "--kind", "sqlite", "--migrations-path", "../outside", "--json"}, 1},
		{[]string{"add", "db", "analytics", "--file", manifestPath, "--kind", "clickhouse", "--migrations-path", "../outside", "--json"}, 1},
		{[]string{"add", "redis", "cache", "--file", manifestPath, "--addr-default", "redis://user:secret@localhost:6379/0", "--json"}, 1},
		{[]string{"add", "redis", "cache", "--file", manifestPath, "--json", "--default"}, 2},
	}
	for _, test := range tests {
		_, exitCode := runCLIError(t, binary, test.args...)
		require.Equal(t, test.exitCode, exitCode, test.args)
	}
}

func TestInitScaffoldCreatesMigrationToolingWithoutOwningSQL(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  db:
    connections:
      - name: primary
        default: sqlite
        variants:
          - name: sqlite
            kind: sqlite
            dsn_env: DB_PRIMARY_SQLITE_DSN
            dsn_default: file:./data/primary.db?_foreign_keys=on
            migrations:
              path: migrations/primary/sqlite
              database_env: DB_PRIMARY_SQLITE_MIGRATIONS_URL
              database_default: sqlite://./data/primary.db?_pragma=foreign_keys%281%29
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "init", "scaffold", "--file", manifestPath)

	require.FileExists(t, filepath.Join(root, "migrations/primary/sqlite/.gitkeep"))
	var mise struct {
		Tasks map[string]struct {
			Run string `toml:"run"`
		} `toml:"tasks"`
	}
	_, err := toml.DecodeFile(filepath.Join(root, ".mise.toml"), &mise)
	require.NoError(t, err)
	require.Contains(t, mise.Tasks, "migrate:primary:sqlite:create")
	require.Contains(t, mise.Tasks, "migrate:primary:sqlite:up")
	require.Contains(t, mise.Tasks, "migrate:primary:sqlite:down")
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	require.NotContains(t, string(goMod), "golang-migrate")

	migrationPath := filepath.Join(root, "migrations/primary/sqlite/20260830010000_create_users.up.sql")
	require.NoError(t, os.WriteFile(migrationPath, []byte("CREATE TABLE users (id INTEGER PRIMARY KEY);\n"), 0o644))
	runCLI(t, binary, "init", "scaffold", "--file", manifestPath)
	content, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	require.Equal(t, "CREATE TABLE users (id INTEGER PRIMARY KEY);\n", string(content))
	runCLI(t, binary, "validate", "--file", manifestPath)
}

func TestAddedClickHouseConnectionPassesValidation(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	runCLI(t, binary, "add", "db", "analytics", "--kind", "clickhouse", "--file", manifestPath)
	runCLI(t, binary, "init", "scaffold", "--file", manifestPath)
	output := runCLI(t, binary, "validate", "--file", manifestPath, "--json")
	var event struct {
		Data struct {
			Valid  bool  `json:"valid"`
			Issues []any `json:"issues"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.True(t, event.Data.Valid)
	require.Empty(t, event.Data.Issues)
}

func TestValidateRejectsGRPCClientWithUnknownSource(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  grpc:
    clients:
      - {name: billing, source: missing, path: proto/billing, proto_root: proto}
languages:
  go: {module: example.test/sample}
`), 0o644))

	entry, exitCode := runCLIError(t, binary, "validate", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "source_not_found", issue["code"])
	require.Equal(t, "components.grpc.clients.billing.source", issue["field"])
}

func TestValidateRejectsGRPCClientWithInvalidContractSelection(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "buf"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "buf", "grpc.gen.yaml"), []byte("version: v2\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  contracts: {type: git, repo: example/contracts, ref: v1}
exports: {}
components:
  grpc:
    clients:
      - {name: billing, source: contracts, export: billing}
languages:
  go: {module: example.test/sample}
`), 0o644))

	entry, exitCode := runCLIError(t, binary, "validate", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "grpc_client_invalid", issue["code"])
	require.Equal(t, "components.grpc.clients.billing", issue["field"])
}

func TestValidateRejectsS3BucketWithUnknownConnection(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  s3:
    connections:
      - {name: default, credentials: static}
    buckets:
      - {name: media, connection: archive, bucket: media-local}
languages:
  go: {module: example.test/sample}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n"), 0o644))

	entry, exitCode := runCLIError(t, binary, "validate", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["valid"])
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "s3_connection_not_found", issue["code"])
	require.Equal(t, "components.s3.buckets.media.connection", issue["field"])
}

func TestValidateRejectsKafkaContractWithUnknownSource(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  kafka:
    consumers:
      - name: billing
        topic: billing_service.invoice.events.v1
        contract:
          source: missing
          path: invoice.proto
          format: proto
          message: acme.invoice.v1.Invoice
          encoding: binary
languages:
  go: {module: example.test/sample}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n"), 0o644))

	entry, exitCode := runCLIError(t, binary, "validate", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "source_not_found", issue["code"])
	require.Equal(t, "components.kafka.consumers.billing.contract.source", issue["field"])
}

func TestValidateReportsMissingGRPCGeneratorConfig(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  grpc:
    server: {proto_root: api/proto, buf_config: buf.yaml}
languages:
  go:
    module: example.test/sample
    generators:
      grpc: {out: gen/grpc, buf_gen_config: tools/buf/grpc.gen.yaml}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "buf.yaml"), []byte("version: v2\nmodules:\n  - path: api/proto\n"), 0o644))

	entry, exitCode := runCLIError(t, binary, "validate", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_config_missing", issue["code"])
	require.Equal(t, "tools/buf/grpc.gen.yaml", issue["field"])
}

func TestInspectReportsGRPCAndKafkaTargets(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env:
  custom:
    - group: app
      vars:
        - {key: API_TOKEN, type: string, default: do-not-leak, secret: true}
paths: {}
sources:
  contracts: {type: local, path: api/contracts}
exports: {}
components:
  grpc:
    server: {proto_root: api/proto, buf_config: buf.yaml}
    clients:
      - {name: billing, source: contracts, path: proto/billing, proto_root: proto}
  kafka:
    producers:
      - name: audit
        topic: audit_service.audit.events.v1
        contract: {format: raw}
languages:
  go: {module: example.test/sample}
`), 0o644))

	output := runCLI(t, binary, "inspect", "--file", manifestPath, "--json")
	var event struct {
		Data struct {
			Project struct {
				Targets []struct {
					ID     string `json:"id"`
					Family string `json:"family"`
					Format string `json:"format"`
					Input  string `json:"input"`
					Config string `json:"config"`
					Output string `json:"output"`
				} `json:"targets"`
				Env []struct {
					Key     string `json:"key"`
					Secret  bool   `json:"secret"`
					Default *any   `json:"default"`
				} `json:"env"`
			} `json:"project"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, "config", event.Data.Project.Targets[0].ID)
	require.Equal(t, "gen/config", event.Data.Project.Targets[0].Output)
	require.Equal(t, "grpc-client:billing", event.Data.Project.Targets[1].ID)
	require.Equal(t, "grpc", event.Data.Project.Targets[1].Family)
	require.Equal(t, "proto", event.Data.Project.Targets[1].Format)
	require.Equal(t, "api/contracts/proto", event.Data.Project.Targets[1].Input)
	require.Equal(t, "tools/buf/grpc.gen.yaml", event.Data.Project.Targets[1].Config)
	require.Equal(t, "gen/grpc/client/billing", event.Data.Project.Targets[1].Output)
	require.Equal(t, "grpc-server", event.Data.Project.Targets[2].ID)
	require.Equal(t, "kafka-producer:audit", event.Data.Project.Targets[3].ID)
	require.Equal(t, "raw", event.Data.Project.Targets[3].Format)
	require.Equal(t, "SAMPLE_API_TOKEN", event.Data.Project.Env[0].Key)
	require.True(t, event.Data.Project.Env[0].Secret)
	require.Nil(t, event.Data.Project.Env[0].Default)
}

func TestInspectAddsResolvedInputFromCommittedSnapshotMetadata(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  upstream: {type: devctl, repo: example/contracts, ref: v1}
exports: {}
components:
  kafka:
    consumers:
      - name: audit
        topic: audit_service.audit.events.v1
        contract: {format: json, source: upstream, export: audit}
languages:
  go: {module: example.test/sample}
`), 0o644))
	targetRoot := filepath.Join(root, "api/external/kafka/consumer/audit")
	require.NoError(t, os.MkdirAll(filepath.Join(targetRoot, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(targetRoot, ".devctl-contract.json"),
		[]byte(`{"kind":"kafka","topic":"audit_service.audit.events.v1","format":"json","entrypoint":"schemas/event.json"}`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(targetRoot, "schemas/event.json"), []byte(`{"title":"AuditEvent","type":"object"}`), 0o644,
	))

	output := runCLI(t, binary, "inspect", "--file", manifestPath, "--json")
	var event struct {
		Data struct {
			Project struct {
				Targets []struct {
					ID            string `json:"id"`
					Input         string `json:"input"`
					ResolvedInput string `json:"resolved_input"`
				} `json:"targets"`
			} `json:"project"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, "kafka-consumer:audit", event.Data.Project.Targets[1].ID)
	require.Equal(t, "api/external/kafka/consumer/audit", event.Data.Project.Targets[1].Input)
	require.Equal(t, "api/external/kafka/consumer/audit/schemas/event.json", event.Data.Project.Targets[1].ResolvedInput)
}

func TestSyncGRPCDryRunReportsPlannedClientPublication(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  billing: {type: url, url: https://example.test/billing.proto}
exports: {}
components:
  grpc:
    clients:
      - {name: billing, source: billing, path: billing.proto, proto_root: .}
languages:
  go: {module: example.test/sample}
`), 0o644))

	output := runCLI(t, binary, "sync", "grpc", "--file", manifestPath, "--dry-run", "--json")
	var event struct {
		Data struct {
			Targets []string `json:"targets"`
			Changes []struct {
				Target string `json:"target"`
				Path   string `json:"path"`
				Action string `json:"action"`
			} `json:"changes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, []string{"grpc-client:billing"}, event.Data.Targets)
	require.Equal(t, "grpc-client:billing", event.Data.Changes[0].Target)
	require.Equal(t, "api/external/grpc/client/billing", event.Data.Changes[0].Path)
	require.Equal(t, "planned_publish", event.Data.Changes[0].Action)
}

func TestSyncKafkaDryRunReportsPlannedProducerPublication(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  events: {type: url, url: https://example.test/events.proto}
exports: {}
components:
  kafka:
    producers:
      - name: invoice
        topic: invoice.events
        contract: {source: events, path: events.proto, format: proto, message: acme.Invoice, encoding: binary}
languages:
  go: {module: example.test/sample}
`), 0o644))

	output := runCLI(t, binary, "sync", "kafka", "--file", manifestPath, "--dry-run", "--json")
	var event struct {
		Data struct {
			Targets []string `json:"targets"`
			Changes []struct {
				Target string `json:"target"`
				Path   string `json:"path"`
			} `json:"changes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, []string{"kafka-producer:invoice"}, event.Data.Targets)
	require.Equal(t, "api/external/kafka/producer/invoice", event.Data.Changes[0].Path)
}

func TestGenGRPCDryRunReportsServerAndClientOutputs(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  billing: {type: local, path: api/contracts}
exports: {}
components:
  grpc:
    server: {proto_root: api/proto, buf_config: buf.yaml}
    clients:
      - {name: billing, source: billing, path: billing, proto_root: proto}
languages:
  go:
    module: example.test/sample
    generators:
      grpc: {out: gen/grpc, buf_gen_config: tools/buf/grpc.gen.yaml}
`), 0o644))

	output := runCLI(t, binary, "gen", "grpc", "--file", manifestPath, "--dry-run", "--json")
	var event struct {
		Data struct {
			Targets []string `json:"targets"`
			Changes []struct {
				Target string `json:"target"`
				Path   string `json:"path"`
				Action string `json:"action"`
			} `json:"changes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, []string{"grpc-server", "grpc-client:billing"}, event.Data.Targets)
	require.Equal(t, []struct {
		Target string `json:"target"`
		Path   string `json:"path"`
		Action string `json:"action"`
	}{
		{Target: "grpc-server", Path: "gen/grpc/server", Action: "planned_publish"},
		{Target: "grpc-client:billing", Path: "gen/grpc/client/billing", Action: "planned_publish"},
	}, event.Data.Changes)
}

func TestGenKafkaDryRunReportsConsumerAndProducerOutputs(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {external_contracts: api/external}
sources:
  events: {type: local, path: api/events}
exports: {}
components:
  kafka:
    consumers:
      - name: invoices
        topic: invoice_service.invoice.events.v1
        contract: {source: events, path: invoice.proto, format: proto, proto_root: ., message: acme.Invoice, encoding: binary}
    producers:
      - name: audit
        topic: audit_service.audit.events.v1
        contract: {source: events, path: audit.proto, format: proto, proto_root: ., message: acme.Audit, encoding: binary}
languages:
  go:
    module: example.test/sample
    generators:
      kafka: {out: gen/kafka, buf_gen_config: tools/buf/kafka.gen.yaml}
`), 0o644))

	output := runCLI(t, binary, "gen", "kafka", "--file", manifestPath, "--dry-run", "--json")
	var event struct {
		Data struct {
			Targets []string `json:"targets"`
			Changes []struct {
				Target string `json:"target"`
				Path   string `json:"path"`
				Action string `json:"action"`
			} `json:"changes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, []string{"kafka-consumer:invoices", "kafka-producer:audit"}, event.Data.Targets)
	require.Equal(t, []struct {
		Target string `json:"target"`
		Path   string `json:"path"`
		Action string `json:"action"`
	}{
		{Target: "kafka-consumer:invoices", Path: "gen/kafka/consumer/invoices", Action: "planned_publish"},
		{Target: "kafka-producer:audit", Path: "gen/kafka/producer/audit", Action: "planned_publish"},
	}, event.Data.Changes)
}

func TestGenGRPCPublishesBufOutput(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  grpc:
    server: {proto_root: api/proto, buf_config: buf.yaml}
languages:
  go:
    module: example.test/sample
    generators:
      grpc: {out: gen/grpc, buf_gen_config: tools/buf/grpc.gen.yaml}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/proto/acme/v1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/proto/acme/v1/service.proto"), []byte("syntax = \"proto3\";\npackage acme.v1;\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools/buf"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools/buf/grpc.gen.yaml"), []byte("version: v2\nplugins: []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "buf.yaml"), []byte("version: v2\nmodules:\n  - path: api/proto\n"), 0o644))
	t.Setenv("PATH", testexec.StubPathCommand(t, "go", `#!/bin/sh
set -eu
test "$1" = tool
test "$2" = buf
test "$3" = generate
test "$4" = api/proto
test "$5" = --template
test "$6" = tools/buf/grpc.gen.yaml
test "$7" = --output
mkdir -p "$8/acme/v1"
printf '// Code generated by protoc-gen-go. DO NOT EDIT.\npackage acmev1\n' > "$8/acme/v1/service.pb.go"
`))

	output := runCLI(t, binary, "gen", "grpc", "--target", "grpc-server", "--file", manifestPath, "--json")
	var event struct {
		Data struct {
			Targets []string `json:"targets"`
			Changes []struct {
				Path   string `json:"path"`
				Action string `json:"action"`
			} `json:"changes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &event))
	require.Equal(t, []string{"grpc-server"}, event.Data.Targets)
	require.Equal(t, "gen/grpc/server/acme/v1/service.pb.go", event.Data.Changes[0].Path)
	require.Equal(t, "created", event.Data.Changes[0].Action)
	require.FileExists(t, filepath.Join(root, "gen/grpc/server/acme/v1/service.pb.go"))
}

func TestGenGRPCClientUsesProtoRootAndSelectedPath(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  contracts: {type: local, path: api/contracts}
exports: {}
components:
  grpc:
    clients:
      - name: billing
        source: contracts
        path: proto/acme/billing/v1
        proto_root: proto
        buf_gen_config: tools/buf/billing.gen.yaml
languages:
  go:
    module: example.test/sample
    generators:
      grpc: {out: gen/grpc, buf_gen_config: tools/buf/grpc.gen.yaml}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/contracts/proto/acme/billing/v1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/contracts/proto/acme/billing/v1/billing.proto"), []byte("syntax = \"proto3\";\npackage acme.billing.v1;\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools/buf"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools/buf/billing.gen.yaml"), []byte("version: v2\nplugins: []\n"), 0o644))
	t.Setenv("PATH", testexec.StubPathCommand(t, "go", `#!/bin/sh
set -eu
test "$1" = tool
test "$2" = buf
test "$3" = generate
test "$4" = api/contracts/proto
test "$5" = --template
test "$6" = tools/buf/billing.gen.yaml
test "$7" = --path
test "$8" = acme/billing/v1
test "$9" = --output
mkdir -p "${10}/acme/billing/v1"
printf '// Code generated by protoc-gen-go. DO NOT EDIT.\npackage billingv1\n' > "${10}/acme/billing/v1/billing.pb.go"
`))

	runCLI(t, binary, "gen", "grpc", "--target", "grpc-client:billing", "--file", manifestPath, "--json")

	require.FileExists(t, filepath.Join(root, "gen/grpc/client/billing/acme/billing/v1/billing.pb.go"))
}

func TestGenKafkaProducerUsesProtoContractSelection(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  events: {type: local, path: api/events}
exports: {}
components:
  kafka:
    producers:
      - name: invoice
        topic: invoice_service.invoice.events.v1
        contract:
          source: events
          path: proto/invoice_service.invoice.events.v1.proto
          format: proto
          proto_root: proto
          message: acme.invoice.v1.Invoice
          encoding: binary
languages:
  go:
    module: example.test/sample
    generators:
      kafka: {out: gen/kafka, buf_gen_config: tools/buf/kafka.gen.yaml}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/events/proto"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/events/proto/invoice_service.invoice.events.v1.proto"), []byte("syntax = \"proto3\";\npackage acme.invoice.v1;\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools/buf"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools/buf/kafka.gen.yaml"), []byte("version: v2\nplugins: []\n"), 0o644))
	t.Setenv("PATH", testexec.StubPathCommand(t, "go", `#!/bin/sh
set -eu
test "$1" = tool
test "$2" = buf
test "$3" = generate
test "$4" = api/events/proto
test "$5" = --template
test "$6" = tools/buf/kafka.gen.yaml
test "$7" = --path
test "$8" = invoice_service.invoice.events.v1.proto
test "$9" = --output
mkdir -p "${10}/acme/invoice/v1"
printf '// Code generated by protoc-gen-go. DO NOT EDIT.\npackage invoicev1\n' > "${10}/acme/invoice/v1/invoice.pb.go"
`))

	runCLI(t, binary, "gen", "kafka", "--target", "kafka-producer:invoice", "--file", manifestPath, "--json")

	require.FileExists(t, filepath.Join(root, "gen/kafka/producer/invoice/acme/invoice/v1/invoice.pb.go"))
}

func TestLintGRPCPreservesCompletedFindingWhenBufIsUnavailable(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  grpc:
    server: {proto_root: api/proto, buf_config: buf.yaml}
languages:
  go: {module: example.test/sample}
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/proto/acme/v1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/proto/acme/v1/service.proto"), []byte("syntax = \"proto3\";\npackage acme.v1;\nservice BillingService {}\n"), 0o644))

	entry, exitCode := runCLIError(t, binary, "lint", "grpc", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	require.Equal(t, "unavailable", entry["code"])
	require.NotContains(t, entry, "data")
	details, ok := entry["details"].(map[string]any)
	require.True(t, ok)
	partial, ok := details["partial_result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, partial["valid"])
	require.Equal(t, []any{"grpc-server"}, partial["contracts"])
	issues, ok := partial["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "proto_filename", issue["code"])
	require.Equal(t, "grpc-server", issue["target"])
	require.Equal(t, "api/proto/acme/v1/service.proto", issue["path"])
}

func TestLintKafkaReportsInvalidTopicName(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  kafka:
    producers:
      - name: audit
        topic: audit.events
        contract: {format: raw}
languages:
  go: {module: example.test/sample}
`), 0o644))

	entry, exitCode := runCLIError(t, binary, "lint", "kafka", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["valid"])
	require.Equal(t, []any{"kafka-producer:audit"}, data["contracts"])
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "kafka_topic", issue["code"])
	require.Equal(t, "kafka-producer:audit", issue["target"])
}

func TestLintKafkaReportsMismatchedLocalSchemaFilename(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  events: {type: local, path: api/events}
exports: {}
components:
  kafka:
    producers:
      - name: audit
        topic: audit_service.audit.created.v1
        contract:
          source: events
          path: wrong_name.json
          format: json
languages:
  go: {module: example.test/sample}
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/events"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/events/wrong_name.json"), []byte(`{"title":"AuditEvent","type":"object"}`), 0o644))

	entry, exitCode := runCLIError(t, binary, "lint", "kafka", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["valid"])
	require.Equal(t, []any{"kafka-producer:audit"}, data["contracts"])
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "kafka_schema_filename", issue["code"])
	require.Equal(t, "kafka-producer:audit", issue["target"])
	require.Equal(t, "wrong_name.json", issue["path"])
}

func TestLintKafkaReportsMissingJSONSchemaTitle(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources:
  events: {type: local, path: api/events}
exports: {}
components:
  kafka:
    producers:
      - name: audit
        topic: audit_service.audit.created.v1
        contract:
          source: events
          path: audit_service.audit.created.v1.json
          format: json
languages:
  go: {module: example.test/sample}
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/events"), 0o755))
	schemaPath := filepath.Join(root, "api/events/audit_service.audit.created.v1.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{"type":"object"}`), 0o644))

	entry, exitCode := runCLIError(t, binary, "lint", "kafka", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "json_schema_title", issue["code"])
	require.Equal(t, "kafka-producer:audit", issue["target"])
	require.Equal(t, schemaPath, issue["path"])
	require.Equal(t, "title", issue["field"])
}

func TestLintWithoutFamilyIncludesKafkaContracts(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  kafka:
    consumers:
      - name: audit
        topic: audit.events
        contract: {format: raw}
languages:
  go: {module: example.test/sample}
`), 0o644))

	entry, exitCode := runCLIError(t, binary, "lint", "--file", manifestPath, "--json")

	require.Equal(t, 1, exitCode)
	data, ok := entry["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"kafka-consumer:audit"}, data["contracts"])
	issues, ok := data["issues"].([]any)
	require.True(t, ok)
	require.Len(t, issues, 1)
	issue, ok := issues[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "kafka_topic", issue["code"])
}

type testManifest struct {
	Sources    map[string]testSource `yaml:"sources"`
	Components struct {
		GRPC  *testGRPC  `yaml:"grpc"`
		Kafka *testKafka `yaml:"kafka"`
		Redis *testRedis `yaml:"redis"`
		S3    *testS3    `yaml:"s3"`
		DB    *testDB    `yaml:"db"`
	} `yaml:"components"`
	Languages struct {
		Go struct {
			Generators struct {
				GRPC  *testGenerator `yaml:"grpc"`
				Kafka *testGenerator `yaml:"kafka"`
			} `yaml:"generators"`
		} `yaml:"go"`
	} `yaml:"languages"`
}

type testSource struct {
	Proto struct {
		BufConfig string `yaml:"buf_config"`
	} `yaml:"proto"`
}
type testGenerator struct {
	Out          string `yaml:"out"`
	BufGenConfig string `yaml:"buf_gen_config"`
}
type testStart struct {
	Env     string `yaml:"env"`
	Default *bool  `yaml:"default"`
}
type testGRPC struct {
	Server  *testGRPCServer  `yaml:"server"`
	Clients []testGRPCClient `yaml:"clients"`
}
type testGRPCServer struct {
	ProtoRoot string     `yaml:"proto_root"`
	BufConfig string     `yaml:"buf_config"`
	Start     *testStart `yaml:"start"`
}
type testGRPCClient struct {
	Name         string `yaml:"name"`
	Source       string `yaml:"source"`
	Path         string `yaml:"path"`
	ProtoRoot    string `yaml:"proto_root"`
	BufGenConfig string `yaml:"buf_gen_config"`
	AddrEnv      string `yaml:"addr_env"`
}
type testKafka struct {
	Consumers []testKafkaConsumer `yaml:"consumers"`
	Producers []testKafkaProducer `yaml:"producers"`
}
type testKafkaContract struct {
	Source    string `yaml:"source"`
	Path      string `yaml:"path"`
	Format    string `yaml:"format"`
	ProtoRoot string `yaml:"proto_root"`
	Message   string `yaml:"message"`
	Encoding  string `yaml:"encoding"`
}
type testKafkaConsumer struct {
	Name     string            `yaml:"name"`
	Topic    string            `yaml:"topic"`
	GroupEnv string            `yaml:"group_env"`
	Start    *testStart        `yaml:"start"`
	Contract testKafkaContract `yaml:"contract"`
}
type testKafkaProducer struct {
	Name     string            `yaml:"name"`
	Topic    string            `yaml:"topic"`
	TopicEnv string            `yaml:"topic_env"`
	Contract testKafkaContract `yaml:"contract"`
}
type testRedis struct {
	Connections []testRedisConnection `yaml:"connections"`
}
type testRedisConnection struct {
	Name        string `yaml:"name"`
	AddrEnv     string `yaml:"addr_env"`
	AddrDefault string `yaml:"addr_default"`
}
type testS3 struct {
	Connections []testS3Connection `yaml:"connections"`
	Buckets     []testS3Bucket     `yaml:"buckets"`
}
type testS3Connection struct {
	Name        string `yaml:"name"`
	Credentials string `yaml:"credentials"`
	Endpoint    string `yaml:"endpoint"`
	Region      string `yaml:"region"`
	PathStyle   bool   `yaml:"path_style"`
}
type testS3Bucket struct {
	Name       string `yaml:"name"`
	Connection string `yaml:"connection"`
	Bucket     string `yaml:"bucket"`
}
type testDB struct {
	Connections []testDBConnection `yaml:"connections"`
}
type testDBConnection struct {
	Name     string          `yaml:"name"`
	Variants []testDBVariant `yaml:"variants"`
}
type testDBVariant struct {
	Kind       string            `yaml:"kind"`
	DSNDefault string            `yaml:"dsn_default"`
	Migrations *testDBMigrations `yaml:"migrations"`
}
type testDBMigrations struct {
	Path            string `yaml:"path"`
	DatabaseEnv     string `yaml:"database_env"`
	DatabaseDefault string `yaml:"database_default"`
}

func readTestManifest(t *testing.T, path string) testManifest {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var manifest testManifest
	require.NoError(t, yaml.Unmarshal(data, &manifest))
	return manifest
}

func boolPointer(value bool) *bool { return &value }

func TestLeafHelpOwnsCommonFlags(t *testing.T) {
	t.Parallel()
	stdout := runCLI(t, buildCLI(t), "validate", "--help")

	require.Contains(t, stdout, "--file")
	require.Contains(t, stdout, "--json")
	require.Contains(t, stdout, "--verbose")
	require.NotContains(t, stdout, "--format")
}

func TestValidateEmitsJSONResultEvent(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(manifestPath), "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestPath, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))

	command := exec.CommandContext(context.Background(), binary, "validate", "--file", manifestPath, "--json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	require.NoError(t, command.Run(), "stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	require.Empty(t, stderr.String())
	var event struct {
		Level   string `json:"level"`
		Message string `json:"msg"`
		Command string `json:"command"`
		Data    struct {
			Valid  bool  `json:"valid"`
			Issues []any `json:"issues"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &event))
	require.Equal(t, "info", event.Level)
	require.Equal(t, "project validation completed", event.Message)
	require.Equal(t, "validate", event.Command)
	require.True(t, event.Data.Valid)
	require.Empty(t, event.Data.Issues)
}

func TestUsageErrorsExitTwo(t *testing.T) {
	t.Parallel()

	binary := buildCLI(t)
	for _, args := range [][]string{{"--bad"}, {"--format", "json", "validate"}, {"--json", "validate"}, {"unknown"}, {"inspect", "extra"}} {
		command := exec.CommandContext(context.Background(), binary, args...)
		err := command.Run()
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError, "args: %v", args)
		require.Equal(t, 2, exitError.ExitCode(), "args: %v", args)
	}
}

func TestErrorsUseSelectedLogEncoding(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)

	t.Run("json usage", func(t *testing.T) {
		t.Parallel()
		entry, exitCode := runCLIError(t, binary, "validate", "--json", "extra")

		require.Equal(t, 2, exitCode)
		require.Equal(t, "error", entry["level"])
		require.Equal(t, "usage", entry["code"])
		require.EqualValues(t, 2, entry["exit_code"])
		require.NotEmpty(t, entry["msg"])
		require.NotContains(t, entry, "logger")
		require.NotContains(t, entry, "error")
	})

	t.Run("console usage", func(t *testing.T) {
		t.Parallel()
		command := exec.CommandContext(context.Background(), binary, "unknown")
		output, err := command.CombinedOutput()
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError)
		require.Equal(t, 2, exitError.ExitCode())
		require.Contains(t, string(output), "\terror\t")
		require.Contains(t, string(output), `"code": "usage"`)
	})
}

func TestExecutionErrorHidesRawCauseUnlessVerbose(t *testing.T) {
	t.Parallel()
	binary := buildCLI(t)
	manifestPath := filepath.Join(t.TempDir(), "missing.yaml")

	safeEntry, exitCode := runCLIError(t, binary, "validate", "--json", "--file", manifestPath)
	require.Equal(t, 1, exitCode)
	require.Equal(t, "not_found", safeEntry["code"])
	require.EqualValues(t, 1, safeEntry["exit_code"])
	require.Equal(t, "requested resource was not found", safeEntry["msg"])
	require.NotContains(t, safeEntry, "error")
	require.NotContains(t, safeEntry, "data")
	require.NotContains(t, safeEntry, "details")
	require.NotContains(t, stringJSON(t, safeEntry), manifestPath)

	verboseEntry, exitCode := runCLIError(t, binary, "validate", "--json", "--verbose", "--file", manifestPath)
	require.Equal(t, 1, exitCode)
	require.Contains(t, verboseEntry["error"], "readManifestFile")
	require.Contains(t, verboseEntry["error"], filepath.Base(manifestPath))
}

func buildCLI(t *testing.T) string {
	t.Helper()
	if testCLIBinary == "" {
		t.Fatal("test CLI binary was not initialized")
	}
	return testCLIBinary
}

func runCLI(t *testing.T, binary string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(context.Background(), binary, args...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "output:\n%s", output)
	return string(output)
}

func runCLIError(t *testing.T, binary string, args ...string) (map[string]any, int) {
	t.Helper()
	command := exec.CommandContext(context.Background(), binary, args...)
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.NotEmpty(t, lines)
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &entry), "output:\n%s", output)
	return entry, exitError.ExitCode()
}

func stringJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
