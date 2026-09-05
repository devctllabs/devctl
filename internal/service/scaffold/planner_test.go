package scaffold

import (
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

func TestPlanAddsPinnedBufToolingForGRPC(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.GRPC = &projectdomain.GRPC{Server: &projectdomain.GRPCServer{
		ProtoRoot: "api/proto", BufConfig: "buf.yaml",
	}}
	manifest.Languages.Go.Generators.GRPC = &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"}
	artifacts := plannedArtifacts(t, manifest)

	goMod, err := modfile.Parse("go.mod", artifacts["go.mod"].Content, nil)
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

	var moduleConfig struct {
		Modules []struct {
			Path string `yaml:"path"`
		} `yaml:"modules"`
		Lint struct {
			Use    []string `yaml:"use"`
			Except []string `yaml:"except"`
		} `yaml:"lint"`
	}
	require.NoError(t, yaml.Unmarshal(artifacts["buf.yaml"].Content, &moduleConfig))
	require.Equal(t, "api/proto", moduleConfig.Modules[0].Path)
	require.Equal(t, []string{"STANDARD"}, moduleConfig.Lint.Use)
	require.Equal(t, []string{"FILE_LOWER_SNAKE_CASE"}, moduleConfig.Lint.Except)

	var generationConfig struct {
		Plugins []struct {
			Local []string `yaml:"local"`
		} `yaml:"plugins"`
	}
	require.NoError(t, yaml.Unmarshal(artifacts["tools/buf/grpc.gen.yaml"].Content, &generationConfig))
	require.Equal(t, [][]string{
		{"go", "tool", "protoc-gen-go"},
		{"go", "tool", "protoc-gen-go-grpc"},
	}, [][]string{generationConfig.Plugins[0].Local, generationConfig.Plugins[1].Local})
}

func TestPlanManagesOnlyCanonicalBufGenerationConfigs(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Sources = map[string]projectdomain.Source{
		"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"},
	}
	manifest.Components.GRPC = &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{
		{Name: "billing", Source: "contracts", Path: "proto/billing.proto", BufGenConfig: "tools/buf/billing.gen.yaml"},
		{Name: "catalog", Source: "contracts", Path: "proto/catalog.proto", BufGenConfig: "tools/buf/grpc.gen.yaml"},
		{Name: "orders", Source: "contracts", Path: "proto/orders.proto", BufGenConfig: "tools/buf/orders.gen.yaml"},
	}}

	artifacts := plannedArtifacts(t, manifest)

	require.Contains(t, artifacts, "tools/buf/grpc.gen.yaml")
	require.NotContains(t, artifacts, "tools/buf/billing.gen.yaml")
	require.NotContains(t, artifacts, "tools/buf/orders.gen.yaml")
}

func TestPlanDoesNotCreateUnusedCanonicalBufConfigForCustomOnlyGRPCTargets(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Sources = map[string]projectdomain.Source{
		"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"},
	}
	manifest.Components.GRPC = &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{
		{Name: "billing", Source: "contracts", Path: "proto/billing.proto", BufGenConfig: "tools/buf/billing.gen.yaml"},
		{Name: "orders", Source: "contracts", Path: "proto/orders.proto", BufGenConfig: "tools/buf/orders.gen.yaml"},
	}}

	artifacts := plannedArtifacts(t, manifest)

	require.NotContains(t, artifacts, "tools/buf/grpc.gen.yaml")
	require.NotContains(t, artifacts, "tools/buf/billing.gen.yaml")
	require.NotContains(t, artifacts, "tools/buf/orders.gen.yaml")
}

func TestPlanAddsPinnedBufToolingForKafkaProto(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.Kafka = &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
		Name: "events", Topic: "sample.event.events.v1",
		Contract: projectdomain.KafkaContract{Format: "proto", Source: "contracts", Path: "proto/sample.event.events.v1.proto", ProtoRoot: "proto"},
	}}}
	manifest.Languages.Go.Generators.Kafka = &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"}
	artifacts := plannedArtifacts(t, manifest)

	require.Contains(t, artifacts, "tools/buf/kafka.gen.yaml")
	goMod, err := modfile.Parse("go.mod", artifacts["go.mod"].Content, nil)
	require.NoError(t, err)
	tools := make([]string, 0, len(goMod.Tool))
	for _, tool := range goMod.Tool {
		tools = append(tools, tool.Path)
	}
	require.Contains(t, tools, "github.com/bufbuild/buf/cmd/buf")
	require.NotContains(t, string(artifacts["go.mod"].Content), "github.com/devctllabs/go-libs/kafkaproto")
}

func TestPlanAddsKafkaProtoDecoderOnlyForProtoConsumer(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.Kafka = &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
		Name: "events", Topic: "sample.event.events.v1",
		Contract: projectdomain.KafkaContract{Format: "proto", Source: "contracts", Path: "proto/sample.event.events.v1.proto"},
	}}}
	artifacts := plannedArtifacts(t, manifest)

	requireArtifactContains(t, artifacts, "go.mod", "github.com/devctllabs/go-libs/kafkaproto v0.1.0")
	requireArtifactContains(t, artifacts, "internal/deps/kafka_consumers.gen.go", "kafkaproto.NewDecoder", "protoKafkaDecoder")
}

func TestPlanAddsPinnedQuicktypeToolingForKafkaJSON(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.Kafka = &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
		Name: "events", Topic: "sample.event.events.v1",
		Contract: projectdomain.KafkaContract{Format: "json", Source: "contracts", Path: "json/sample.event.events.v1.json"},
	}}}
	artifacts := plannedArtifacts(t, manifest)

	require.NotContains(t, string(artifacts["go.mod"].Content), "go-jsonschema")
	var mise struct {
		Tools map[string]any `toml:"tools"`
	}
	_, err := toml.Decode(string(artifacts[".mise.toml"].Content), &mise)
	require.NoError(t, err)
	require.Equal(t, "24", mise.Tools["node"])
	require.Equal(t, "26.0.0", mise.Tools["npm:quicktype"])
}

func TestPlanPinsPublishedGoLibVersionsByModule(t *testing.T) {
	t.Parallel()

	artifacts := plannedArtifacts(t, fullCharacterizationManifest())
	goMod := string(artifacts["go.mod"].Content)

	for _, dependency := range []string{
		"github.com/devctllabs/go-libs/lifecycle v0.2.0",
		"github.com/devctllabs/go-libs/log v0.2.0",
		"github.com/devctllabs/go-libs/oapivalidator v0.2.0",
		"github.com/devctllabs/go-libs/postgresdb v0.2.0",
		"github.com/devctllabs/go-libs/config v0.1.0",
		"github.com/devctllabs/go-libs/di v0.1.0",
		"github.com/devctllabs/go-libs/kafka v0.1.0",
		"github.com/devctllabs/go-libs/kafkaproto v0.1.0",
		"github.com/devctllabs/go-libs/retry v0.1.0",
		"github.com/devctllabs/go-libs/sqlitedb v0.1.0",
		"github.com/devctllabs/go-libs/txmanager v0.1.0",
		"github.com/devctllabs/go-libs/telemetry v0.1.0",
		"github.com/devctllabs/go-libs/health v0.1.0",
		"github.com/devctllabs/go-libs/healthserver v0.1.0",
		"github.com/devctllabs/go-libs/debugserver v0.1.0",
	} {
		require.Contains(t, goMod, dependency)
	}
	require.NotContains(t, goMod, "replace ")
}

func TestPlanIncludesEveryV1ComponentFoundation(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.GRPC = &projectdomain.GRPC{Server: &projectdomain.GRPCServer{ProtoRoot: "api/proto/grpc", BufConfig: "buf.yaml"}}
	manifest.Components.Kafka = &projectdomain.Kafka{
		Consumers: []projectdomain.KafkaConsumer{{Name: "audit", Topic: "sample.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"}}},
		Producers: []projectdomain.KafkaProducer{{Name: "events", Topic: "sample.event.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"}}},
	}
	manifest.Components.Redis = &projectdomain.Redis{Connections: []projectdomain.RedisConnection{{Name: "cache"}}}
	manifest.Components.S3 = &projectdomain.S3{Connections: []projectdomain.S3Connection{{Name: "default"}}, Buckets: []projectdomain.S3Bucket{{Name: "uploads", Connection: "default"}}}
	artifacts := plannedArtifacts(t, manifest)

	for _, filename := range []string{
		"internal/deps/grpc.gen.go",
		"internal/deps/kafka_broker.gen.go",
		"internal/deps/kafka_consumers.gen.go",
		"internal/deps/kafka_producers.gen.go",
		"internal/deps/redis.gen.go",
		"internal/deps/s3.gen.go",
		"cmd/sample/internal/consumer.go",
		"internal/transport/consumerkafka/audit/handler.go",
	} {
		artifact, exists := artifacts[filename]
		require.True(t, exists, filename)
		_, err := parser.ParseFile(token.NewFileSet(), filename, artifact.Content, parser.AllErrors)
		require.NoError(t, err, filename)
	}
	require.True(t, artifacts["cmd/sample/internal/consumer.go"].CreateOnly)
	require.True(t, artifacts["internal/transport/consumerkafka/audit/handler.go"].CreateOnly)
}

func TestPlanAppliesS3ConnectionRuntimeConfig(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.S3 = &projectdomain.S3{
		Connections: []projectdomain.S3Connection{{
			Name: "archive", Credentials: "static", Endpoint: "http://localhost:9000",
			Region: "us-east-1", PathStyle: true,
		}},
		Buckets: []projectdomain.S3Bucket{{Name: "media", Connection: "archive", Bucket: "media-local"}},
	}
	artifacts := plannedArtifacts(t, manifest)

	requireArtifactContains(t, artifacts, "internal/deps/s3.gen.go",
		`s3ArchiveKey = "s3-connection:archive"`,
		"credentials.NewStaticCredentialsProvider",
		"cfg.S3.ArchiveAccessKeyID", "cfg.S3.ArchiveSecretAccessKey",
		"cfg.S3.ArchiveEndpoint", "options.UsePathStyle = cfg.S3.ArchiveForcePathStyle",
		`s3MediaBucketKey = "s3-bucket:media"`, `s3ArchiveKey`, "cfg.S3.MediaBucket",
	)
}

func TestPlanMakesGeneratedGoOwnershipExplicit(t *testing.T) {
	t.Parallel()

	manifest := fullCharacterizationManifest()
	manifest.Components.GRPC = &projectdomain.GRPC{Server: &projectdomain.GRPCServer{}}
	manifest.Components.Kafka = &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
		Name: "audit", Topic: "sample.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"},
	}}}
	manifest.Components.Redis = &projectdomain.Redis{Connections: []projectdomain.RedisConnection{{Name: "cache"}}}
	manifest.Components.S3 = &projectdomain.S3{Connections: []projectdomain.S3Connection{{Name: "default"}}}

	for path, artifact := range plannedArtifacts(t, manifest) {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if artifact.CreateOnly {
			require.NotContains(t, path, ".gen.go", path)
			require.NotContains(t, string(artifact.Content), "Code generated by devctl", path)
			continue
		}
		require.True(t, strings.HasSuffix(path, ".gen.go"), path)
		require.True(t, strings.HasPrefix(string(artifact.Content), "// Code generated by devctl. DO NOT EDIT.\n"), path)
	}
}

func TestPlanCreatesProjectReadmeWithRefreshChecklist(t *testing.T) {
	t.Parallel()

	artifact := plannedArtifacts(t, fullCharacterizationManifest())["README.md"]

	require.True(t, artifact.CreateOnly)
	require.Contains(t, string(artifact.Content), "go run ./cmd/sample-api --help")
	require.Contains(t, string(artifact.Content), "devctl sync")
	require.Contains(t, string(artifact.Content), "devctl init scaffold")
	require.Contains(t, string(artifact.Content), "devctl gen")
	require.Contains(t, string(artifact.Content), "*.gen.go")
	require.Contains(t, string(artifact.Content), "internal/deps/application.go")
}

func TestPlanSelectsRuntimeRootComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configure        func(*projectdomain.Manifest)
		dependency       string
		configField      string
		runtimeField     string
		unexpectedFields []string
		artifactCount    int
	}{
		{
			name: "HTTP server",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Components.HTTP = &projectdomain.HTTP{Server: &projectdomain.HTTPServer{}}
			},
			dependency: "github.com/labstack/echo/v5 v5.3.1", configField: "HTTPConfig", runtimeField: "http *http.Server",
			unexpectedFields: []string{"httpEnabled bool", "healthEnabled bool", "pprofEnabled bool"}, artifactCount: 14,
		},
		{
			name: "health server",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Components.Health = &projectdomain.Health{}
			},
			dependency: "github.com/devctllabs/go-libs/healthserver v0.1.0", configField: "HealthConfig", runtimeField: "health *healthserverlib.Server",
			unexpectedFields: []string{"httpEnabled bool", "healthEnabled bool", "pprofEnabled bool"}, artifactCount: 12,
		},
		{
			name: "pprof server",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Languages.Go.Components.Pprof = &projectdomain.Pprof{}
			},
			dependency: "github.com/devctllabs/go-libs/debugserver v0.1.0", configField: "PprofConfig", runtimeField: "pprof *debugserverlib.Server",
			unexpectedFields: []string{"httpEnabled bool", "healthEnabled bool", "pprofEnabled bool"}, artifactCount: 12,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manifest := minimalScaffoldManifest()
			test.configure(&manifest)
			artifacts := plannedArtifacts(t, manifest)

			require.Len(t, artifacts, test.artifactCount)
			require.Contains(t, artifacts, "cmd/sample/internal/api.go")
			requireArtifactContains(t, artifacts, "go.mod", "github.com/devctllabs/go-libs/lifecycle v0.2.0", test.dependency)
			requireArtifactContains(t, artifacts, "gen/config/config.gen.go", test.configField)
			requireArtifactContains(t, artifacts, "internal/deps/runtime.gen.go", test.runtimeField)
			for _, field := range test.unexpectedFields {
				require.NotContains(t, string(artifacts["internal/deps/runtime.gen.go"].Content), field)
			}
		})
	}
}

func TestPlanRendersLazyRuntimeScenariosAndFailClosedConsumers(t *testing.T) {
	t.Parallel()

	disabled := false
	enabled := true
	manifest := minimalScaffoldManifest()
	manifest.Components.HTTP = &projectdomain.HTTP{Server: &projectdomain.HTTPServer{}}
	manifest.Components.GRPC = &projectdomain.GRPC{Server: &projectdomain.GRPCServer{Start: &projectdomain.Start{}}}
	manifest.Components.Health = &projectdomain.Health{Server: &projectdomain.HealthServer{Start: &projectdomain.Start{Default: &enabled}}}
	manifest.Components.Kafka = &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{
		{Name: "audit", Topic: "sample.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"}},
		{Name: "invoice", Topic: "sample.invoice.events.v1", Start: &projectdomain.Start{Default: &disabled}, Contract: projectdomain.KafkaContract{Format: "raw"}},
	}}
	artifacts := plannedArtifacts(t, manifest)

	container := string(artifacts["internal/deps/container.gen.go"].Content)
	require.Contains(t, container, "func NewAPI(ctx context.Context) (*Scenario, error)")
	require.Contains(t, container, "func NewConsumer(ctx context.Context, name string) (*Scenario, error)")
	require.Contains(t, container, `case "audit":`)
	require.Contains(t, container, `case "invoice":`)
	require.Contains(t, container, `if !(cfg.Kafka.InvoiceEnabled)`)
	require.Contains(t, container, `di.ResolveNamed[scenarioRunner](graph, kafkaConsumerKey(name))`)
	consumerSource := container[strings.Index(container, "func NewConsumer"):]
	require.Less(t, strings.Index(consumerSource, "switch name"), strings.Index(consumerSource, "graph := di.New()"))
	require.Contains(t, container, "graph.Shutdown(context.WithoutCancel(ctx))")

	runtime := string(artifacts["internal/deps/runtime.gen.go"].Content)
	require.Contains(t, runtime, `lifecycle.Task{Name: "http"`)
	require.Contains(t, runtime, "if r.grpcEnabled")
	require.Contains(t, runtime, "if r.healthEnabled")
	require.Contains(t, runtime, "r.grpc.Stop()")
	require.Contains(t, runtime, "<-ctx.Done()")

	kafka := string(artifacts["internal/deps/kafka_consumers.gen.go"].Content)
	for _, expected := range []string{
		"config.CommitRetry = &config.Retry", "kafka.RejectStop",
		"RebalanceTimeout:", "RebalanceDrainTimeout:", "ShutdownTimeout:",
	} {
		require.Contains(t, kafka, expected)
	}
	requireArtifactContains(t, artifacts, "gen/config/config.gen.go",
		`env:"SAMPLE_KAFKA_AUDIT_TOPIC" default:"sample.audit.events.v1"`,
		`env:"SAMPLE_KAFKA_AUDIT_RETRY_MAX_ATTEMPTS" default:"3"`,
		`env:"SAMPLE_KAFKA_INVOICE_CONSUMER_ENABLED" default:"false"`,
	)
	requireArtifactContains(t, artifacts, "internal/transport/consumerkafka/audit/handler.go",
		"retry.Permanent(ErrNotImplemented)", "must not retain batch data",
	)
}

func TestPlanKeepsTelemetryOutOfServerRoot(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.Telemetry = &projectdomain.Telemetry{}
	artifacts := plannedArtifacts(t, manifest)

	require.Len(t, artifacts, 10)
	require.NotContains(t, artifacts, "internal/deps/runtime.gen.go")
	require.NotContains(t, artifacts, "cmd/sample/internal/api.go")
	requireArtifactContains(t, artifacts, "go.mod", "github.com/devctllabs/go-libs/telemetry v0.1.0")
	requireArtifactContains(t, artifacts, "gen/config/config.gen.go", "TelemetryConfig")
	requireArtifactContains(t, artifacts, "internal/deps/container.gen.go", "telemetrylib.Open", "cfg.Telemetry.ServiceVersion")
	require.NotContains(t, string(artifacts["go.mod"].Content), "github.com/devctllabs/go-libs/lifecycle")
}

func TestPlanAppliesHTTPArtifactPolicies(t *testing.T) {
	t.Parallel()

	t.Run("server and clients", func(t *testing.T) {
		t.Parallel()

		manifest := minimalScaffoldManifest()
		manifest.Sources = map[string]projectdomain.Source{
			"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
		}
		manifest.Components.HTTP = &projectdomain.HTTP{
			Server: &projectdomain.HTTPServer{OpenAPI: "contracts/service.yaml"},
			Clients: []projectdomain.HTTPClient{
				{Name: "orders", Source: "contracts", Path: "orders.yaml", OAPIConfig: "tools/oapi/orders.yaml"},
				{Name: "catalog", Source: "contracts", Path: "catalog.yaml"},
			},
		}
		manifest.Languages.Go.Generators.HTTP = &projectdomain.HTTPGenerator{OAPIConfig: "tools/oapi/custom-server.yaml"}
		artifacts := plannedArtifacts(t, manifest)

		require.True(t, artifacts["contracts/service.yaml"].CreateOnly)
		require.False(t, artifacts["tools/oapi/custom-server.yaml"].CreateOnly)
		require.False(t, artifacts["tools/oapi/clients.catalog.yaml"].CreateOnly)
		require.False(t, artifacts["tools/oapi/orders.yaml"].CreateOnly)
		require.NotContains(t, artifacts, "tools/oapi/server.yaml")
		require.NotContains(t, artifacts, "api/openapi/swagger.yaml")
		requireArtifactContains(t, artifacts, "go.mod", "tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen")
	})

	t.Run("client only", func(t *testing.T) {
		t.Parallel()

		manifest := minimalScaffoldManifest()
		manifest.Sources = map[string]projectdomain.Source{
			"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
		}
		manifest.Components.HTTP = &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{{
			Name: "catalog", Source: "contracts", Path: "catalog.yaml",
		}}}
		artifacts := plannedArtifacts(t, manifest)

		require.Contains(t, artifacts, "tools/oapi/clients.catalog.yaml")
		require.NotContains(t, artifacts, "tools/oapi/server.yaml")
		require.NotContains(t, artifacts, "api/openapi/swagger.yaml")
		require.NotContains(t, artifacts, "internal/deps/runtime.gen.go")
		require.NotContains(t, artifacts, "cmd/sample/internal/api.go")
		requireArtifactContains(t, artifacts, "go.mod", "tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen")
	})
}

func TestPlanExposesRegistrarsAndRawOutboundClients(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Sources = map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"}}
	manifest.Components.HTTP = &projectdomain.HTTP{
		Server:  &projectdomain.HTTPServer{},
		Clients: []projectdomain.HTTPClient{{Name: "billing", Source: "contracts", Path: "billing.yaml"}},
	}
	manifest.Components.GRPC = &projectdomain.GRPC{
		Server:  &projectdomain.GRPCServer{},
		Clients: []projectdomain.GRPCClient{{Name: "billing", Source: "contracts", Path: "billing.proto"}},
	}
	artifacts := plannedArtifacts(t, manifest)

	requireArtifactContains(t, artifacts, "internal/deps/runtime.gen.go",
		"type HTTPRegistrar interface", "RegisterHTTP(*echo.Echo)",
		"type GRPCRegistrar interface", "RegisterGRPC(*grpc.Server)",
		"httpRegistrar.RegisterHTTP", "grpcRegistrar.RegisterGRPC",
	)
	requireArtifactContains(t, artifacts, "internal/deps/http_clients.gen.go",
		`httpClientBillingKey = "http-client:billing"`,
		"func BillingHTTPTransport(resolver di.Resolver) (*http.Client, error)",
		"func BillingHTTPBaseURL(resolver di.Resolver) (string, error)",
	)
	requireArtifactContains(t, artifacts, "internal/deps/grpc_clients.gen.go",
		`grpcClientBillingKey = "grpc-client:billing"`,
		"func BillingGRPCConn(resolver di.Resolver) (*grpc.ClientConn, error)",
	)
	require.NotContains(t, string(artifacts["internal/deps/http_clients.gen.go"].Content), "ClientWithResponses")
	require.NotContains(t, string(artifacts["internal/deps/grpc_clients.gen.go"].Content), "NewBillingClient")
}

func TestPlanRendersDatabaseDecisions(t *testing.T) {
	t.Parallel()

	t.Run("multiple kinds with telemetry", func(t *testing.T) {
		t.Parallel()

		manifest := minimalScaffoldManifest()
		manifest.Env.Prefix = "APP_"
		manifest.Components.Telemetry = &projectdomain.Telemetry{}
		manifest.Components.DB = &projectdomain.DB{Connections: []projectdomain.DBConnection{
			{
				Name: "read-model", KindEnv: "DATABASE_DRIVER",
				Variants: []projectdomain.DBVariant{{Name: "postgres", Kind: "postgres", DSNEnv: "READ_DATABASE_URL", Secret: true}},
			},
			{
				Name: "primary", Default: "local",
				Variants: []projectdomain.DBVariant{
					{Name: "local", Kind: "sqlite", DSNDefault: "file:./data/app.db"},
					{Name: "remote", Kind: "postgres", Secret: true},
				},
			},
		}}
		artifacts := plannedArtifacts(t, manifest)

		require.Contains(t, artifacts, "internal/deps/storage_primary.gen.go")
		require.Contains(t, artifacts, "internal/deps/storage_read_model.gen.go")
		require.True(t, artifacts["data/.gitkeep"].CreateOnly)
		goMod := string(artifacts["go.mod"].Content)
		require.Equal(t, 1, strings.Count(goMod, "github.com/devctllabs/go-libs/sqlitedb v0.1.0"))
		require.Equal(t, 1, strings.Count(goMod, "github.com/devctllabs/go-libs/postgresdb v0.2.0"))
		require.Equal(t, 1, strings.Count(goMod, "github.com/devctllabs/go-libs/txmanager v0.1.0"))
		requireArtifactContains(t, artifacts, "gen/config/config.gen.go",
			`env:"APP_DATABASE_DRIVER" default:"postgres"`,
			`env:"APP_READ_DATABASE_URL"`,
			"DBPrimary   DBPrimaryConfig",
			`env:"APP_DB_PRIMARY_KIND" default:"local"`,
		)
		requireArtifactContains(t, artifacts, "internal/deps/storage_primary.gen.go", `case "local":`, `case "remote":`, "cfg.DBPrimary.Kind", "telemetryRuntime.TracerProvider()")
		requireArtifactContains(t, artifacts, "internal/deps/application.go", "provideStorageReadModel", "provideStoragePrimary")
	})

	t.Run("postgres does not create sqlite data directory", func(t *testing.T) {
		t.Parallel()

		manifest := minimalScaffoldManifest()
		manifest.Components.DB = &projectdomain.DB{Connections: []projectdomain.DBConnection{{
			Name: "primary", Variants: []projectdomain.DBVariant{{Name: "postgres", Kind: "postgres", Secret: true}},
		}}}
		artifacts := plannedArtifacts(t, manifest)

		require.NotContains(t, artifacts, "data/.gitkeep")
		requireArtifactContains(t, artifacts, "gen/config/config.gen.go", `default:"postgres"`, `env:"SAMPLE_DB_PRIMARY_POSTGRES_DSN"`)
		require.NotContains(t, string(artifacts["internal/deps/storage_primary.gen.go"].Content), "sqlitedb")
	})

	t.Run("native clickhouse keeps telemetry out and uses namespaced health key", func(t *testing.T) {
		t.Parallel()

		manifest := minimalScaffoldManifest()
		manifest.Components.Health = &projectdomain.Health{}
		manifest.Components.Telemetry = &projectdomain.Telemetry{}
		manifest.Components.DB = &projectdomain.DB{Connections: []projectdomain.DBConnection{{
			Name: "analytics", Default: "clickhouse",
			Variants: []projectdomain.DBVariant{{Name: "clickhouse", Kind: "clickhouse", DSNDefault: "clickhouse://localhost:9000/default"}},
		}}}
		artifacts := plannedArtifacts(t, manifest)

		storage := string(artifacts["internal/deps/storage_analytics.gen.go"].Content)
		require.Contains(t, storage, `storageAnalyticsConnectionName = "db-connection:analytics"`)
		for _, operation := range []string{"clickhouse.ParseDSN", "clickhouse.Open", "connection.Ping", "connection.Close"} {
			require.Contains(t, storage, operation)
		}
		require.NotContains(t, storage, "telemetrylib")
		require.NotContains(t, storage, "txmanager")
		requireArtifactContains(t, artifacts, "internal/deps/runtime.gen.go",
			`di.ResolveNamed[dbChecker](resolver, "db-connection:analytics")`,
		)
	})
}

func TestPlanScaffoldsMigrationTargetsAndMiseTasks(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Env.Prefix = "APP_"
	manifest.Components.DB = &projectdomain.DB{Connections: []projectdomain.DBConnection{
		{Name: "archive", Variants: []projectdomain.DBVariant{{
			Name: "postgres", Kind: "postgres", Migrations: &projectdomain.DBMigrations{
				Path: "db/archive", DatabaseEnv: "ARCHIVE_MIGRATIONS_URL",
			},
		}}},
		{Name: "primary", Variants: []projectdomain.DBVariant{{
			Name: "sqlite", Kind: "sqlite", Migrations: &projectdomain.DBMigrations{
				Path: "migrations/primary/sqlite", DatabaseEnv: "DB_PRIMARY_SQLITE_MIGRATIONS_URL",
				DatabaseDefault: "sqlite://./data/primary.db?_pragma=foreign_keys%281%29",
			},
		}}},
	}}
	artifacts := plannedArtifacts(t, manifest)

	require.True(t, artifacts["db/archive/.gitkeep"].CreateOnly)
	require.True(t, artifacts["migrations/primary/sqlite/.gitkeep"].CreateOnly)
	var mise struct {
		Tools map[string]toml.Primitive `toml:"tools"`
		Tasks map[string]struct {
			Run     string `toml:"run"`
			Usage   string `toml:"usage"`
			Confirm any    `toml:"confirm"`
		} `toml:"tasks"`
	}
	metadata, err := toml.Decode(string(artifacts[".mise.toml"].Content), &mise)
	require.NoError(t, err)
	var migrateTool struct {
		Version string   `toml:"version"`
		Tags    []string `toml:"tags"`
	}
	require.NoError(t, metadata.PrimitiveDecode(mise.Tools["go:github.com/golang-migrate/migrate/v4/cmd/migrate"], &migrateTool))
	require.Equal(t, "v4.19.1", migrateTool.Version)
	require.Equal(t, []string{"postgres", "sqlite"}, migrateTool.Tags)
	require.Equal(t, `arg "<name>" help="Migration name"`, mise.Tasks["migrate:primary:sqlite:create"].Usage)
	require.Contains(t, mise.Tasks["migrate:primary:sqlite:create"].Run, `-format "20060102150405" "${usage_name?}"`)
	require.Contains(t, mise.Tasks["migrate:primary:sqlite:up"].Run, `${APP_DB_PRIMARY_SQLITE_MIGRATIONS_URL:-sqlite://./data/primary.db?_pragma=foreign_keys%281%29}`)
	require.Equal(t, `arg "[steps]" default="1" help="Number of migrations"`, mise.Tasks["migrate:primary:sqlite:down"].Usage)
	require.NotNil(t, mise.Tasks["migrate:primary:sqlite:down"].Confirm)
	require.Contains(t, mise.Tasks["migrate:archive:postgres:up"].Run, `${APP_ARCHIVE_MIGRATIONS_URL:?set APP_ARCHIVE_MIGRATIONS_URL}`)
	require.NotContains(t, mise.Tasks["migrate:archive:postgres:down"].Run, "-all")
}

func TestPlanUsesFallbackEnvironmentPrefix(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Project.Name = "sample-api"
	manifest.Components.Logging = &projectdomain.Logging{}
	artifacts := plannedArtifacts(t, manifest)

	requireArtifactContains(t, artifacts, "gen/config/config.gen.go", `env:"SAMPLE_API_LOG_LEVEL"`)
}

func TestPlanUsesCanonicalGeneratedRuntimeConfig(t *testing.T) {
	t.Parallel()

	manifest := minimalScaffoldManifest()
	manifest.Components.HTTP = &projectdomain.HTTP{
		Server: &projectdomain.HTTPServer{Start: &projectdomain.Start{}},
		Env: projectdomain.ComponentEnv{System: []projectdomain.EnvVar{{
			Key: "HTTP_ADDR", Type: "string", Default: ":8080",
		}}},
	}
	artifacts := plannedArtifacts(t, manifest)

	requireArtifactContains(t, artifacts, "gen/config/config.gen.go",
		"type Config struct", "HTTP HTTPConfig", "Enabled bool", "Address string",
		`env:"SAMPLE_HTTP_SERVER_ENABLED" default:"false"`,
		`env:"SAMPLE_HTTP_ADDR" default:":8080"`,
	)
	requireArtifactContains(t, artifacts, ".env.example",
		"SAMPLE_HTTP_ADDR=:8080", "SAMPLE_HTTP_SERVER_ENABLED=false",
	)
	requireArtifactContains(t, artifacts, "internal/deps/config.gen.go",
		`generatedconfig "example.test/sample/gen/config"`,
		"type Config = generatedconfig.Config",
	)
	requireArtifactContains(t, artifacts, "internal/deps/runtime.gen.go",
		"cfg.HTTP.Address", "cfg.HTTP.Enabled",
	)
}

func TestPlanRejectsDuplicateArtifactPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*projectdomain.Manifest)
		path      string
	}{
		{
			name: "OpenAPI seed collides with base artifact",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Components.HTTP = &projectdomain.HTTP{Server: &projectdomain.HTTPServer{OpenAPI: "api/../go.mod"}}
			},
			path: "go.mod",
		},
		{
			name: "client generator configs collide",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Sources = map[string]projectdomain.Source{
					"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
				}
				manifest.Components.HTTP = &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{
					{Name: "catalog", Source: "contracts", Path: "catalog.yaml", OAPIConfig: "tools/oapi/shared.yaml"},
					{Name: "orders", Source: "contracts", Path: "orders.yaml", OAPIConfig: "tools/oapi/shared.yaml"},
				}}
			},
			path: "tools/oapi/shared.yaml",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manifest := minimalScaffoldManifest()
			test.configure(&manifest)

			artifacts, err := plan(manifest)

			require.ErrorContains(t, err, "duplicate artifact path")
			require.ErrorContains(t, err, test.path)
			require.Nil(t, artifacts)
		})
	}
}

func minimalScaffoldManifest() projectdomain.Manifest {
	return projectdomain.Manifest{
		Version:   1,
		Project:   projectdomain.Identity{Name: "sample", Language: "go"},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/sample"}},
	}
}

func TestRenderPolicies(t *testing.T) {
	t.Parallel()

	t.Run("Go names", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "Primary", goName("primary"))
		require.Equal(t, "ReadModel", goName("read-model"))
	})

}

func plannedArtifacts(t *testing.T, manifest projectdomain.Manifest) map[string]Artifact {
	t.Helper()

	planned, err := plan(manifest)
	require.NoError(t, err)
	artifacts := make(map[string]Artifact, len(planned))
	for index, artifact := range planned {
		if index > 0 {
			require.Less(t, planned[index-1].Path, artifact.Path, "artifact paths must be sorted")
		}
		require.NotContains(t, artifacts, artifact.Path, "duplicate artifact path")
		require.Equal(t, fs.FileMode(0o644), artifact.Mode, artifact.Path)
		artifacts[artifact.Path] = artifact
	}
	return artifacts
}

func requireArtifactContains(t *testing.T, artifacts map[string]Artifact, path string, values ...string) {
	t.Helper()

	artifact, exists := artifacts[path]
	require.True(t, exists, path)
	for _, value := range values {
		require.Contains(t, string(artifact.Content), value, path)
	}
}
