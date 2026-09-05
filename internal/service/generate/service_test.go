package generate_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	"github.com/devctllabs/devctl/internal/service/generate/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestGenServiceOwnsGenerationAndPublicationFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	generator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Project:    projectdomain.Identity{Name: "checkout", Language: "go"},
		Components: projectdomain.Components{Logging: &projectdomain.Logging{}},
		Languages:  projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/checkout"}},
	}}
	target := generationTarget(t, project.Manifest, "config")
	projects.EXPECT().LoadProject(gomock.Any(), "custom.yaml").Return(project, nil)
	gomock.InOrder(
		workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, gomock.Any()).DoAndReturn(
			func(_ context.Context, _, _ string, tree artifact.Tree) (artifact.PublishResult, error) {
				require.Len(t, tree.Files, 1)
				require.Equal(t, "config.gen.go", tree.Files[0].Path)
				requireStructFieldTag(t, tree.Files[0].Content, fieldTagExpectation{
					Struct: "LoggingConfig", Field: "Level", Tag: `env:"CHECKOUT_LOG_LEVEL" default:"info"`,
				})
				golden, err := os.ReadFile("testdata/config_logging.golden")
				require.NoError(t, err)
				require.Equal(t, golden, tree.Files[0].Content)
				return publishedDirectory(tree, artifact.PublishUpdated), nil
			},
		),
		workspace.EXPECT().PublishFile(gomock.Any(), project.Root, ".env.example", []byte("CHECKOUT_LOG_LEVEL=info\n")).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil),
	)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Generator: generator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "custom.yaml", Family: "config"})

	require.NoError(t, err)
	require.Equal(t, []string{"config"}, result.Targets)
	require.Equal(t, []generatedomain.Change{
		{Target: "config", Path: "gen/config/config.gen.go", Action: generatedomain.ChangeUpdated},
		{Target: "config", Path: ".env.example", Action: generatedomain.ChangeUnchanged},
	}, result.Changes)
}

func TestGenHTTPClientReadsCanonicalExternalLayout(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	generator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Paths:   projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{"remote": {Type: projectdomain.SourceURL}},
		Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{{
			Name: "catalog", Source: "remote", Path: "openapi.yaml",
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{HTTP: &projectdomain.HTTPGenerator{
			OAPIConfig: "tools/oapi/client.yaml", ClientOut: "gen/http/client",
		}}}},
	}}
	logical := generationTarget(t, project.Manifest, "http-client:catalog")
	target := logical
	target.Input = "/project/api/external/http/client/catalog/openapi.yaml"
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	inputs.EXPECT().Resolve(gomock.Any(), project, logical).Return(target, nil)
	generator.EXPECT().Generate(gomock.Any(), project, target).Return(generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{Path: "client.gen.go", Content: []byte("package client")}}}}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, gomock.Any()).Return(artifact.PublishResult{
		Action:  artifact.PublishUpdated,
		Changes: []artifact.PublishChange{{Path: "client.gen.go", Action: artifact.PublishUpdated}},
	}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: inputs, Generator: generator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Target: target.ID})

	require.NoError(t, err)
	require.Equal(t, []string{target.ID}, result.Targets)
}

func TestGenConfigHonorsGRPCStartPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		start          *projectdomain.Start
		expectedFields map[string]string
		expectedEnv    []byte
	}{
		{
			name:  "runtime toggle",
			start: &projectdomain.Start{Env: "GRPC_SERVER_ENABLED", Default: boolPointer(false)},
			expectedFields: map[string]string{
				"Address": `env:"SAMPLE_GRPC_ADDR" default:":9090"`,
				"Enabled": `env:"SAMPLE_GRPC_SERVER_ENABLED" default:"false"`,
			},
			expectedEnv: []byte("SAMPLE_GRPC_ADDR=:9090\nSAMPLE_GRPC_SERVER_ENABLED=false\n"),
		},
		{
			name:  "always active",
			start: nil,
			expectedFields: map[string]string{
				"Address": `env:"SAMPLE_GRPC_ADDR" default:":9090"`,
			},
			expectedEnv: []byte("SAMPLE_GRPC_ADDR=:9090\n"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects := mocks.NewMockProjectRepository(ctrl)
			workspace := mocks.NewMockWorkspaceRepository(ctrl)
			project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
				Project: projectdomain.Identity{Name: "sample"},
				Components: projectdomain.Components{GRPC: &projectdomain.GRPC{
					Server: &projectdomain.GRPCServer{Start: test.start},
				}},
				Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
					Config: &projectdomain.ConfigGenerator{Out: "gen/config"},
				}}},
			}}
			projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
			workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "gen/config", gomock.Any()).DoAndReturn(
				func(_ context.Context, _, _ string, tree artifact.Tree) (artifact.PublishResult, error) {
					require.Len(t, tree.Files, 1)
					require.Equal(t, test.expectedFields, structFieldTags(t, tree.Files[0].Content, "GRPCConfig"))
					return publishedDirectory(tree, artifact.PublishUpdated), nil
				},
			)
			workspace.EXPECT().PublishFile(gomock.Any(), project.Root, ".env.example", test.expectedEnv).Return(artifact.PublishResult{Action: artifact.PublishUpdated}, nil)
			service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
				Projects: projects, Workspace: workspace,
			})

			_, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Family: "config"})

			require.NoError(t, err)
		})
	}
}

func TestGenConfigBuildsKafkaRuntimeAndProducerEnv(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Project: projectdomain.Identity{Name: "sample"},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{
			Consumers: []projectdomain.KafkaConsumer{
				{Name: "billing", GroupEnv: "BILLING_GROUP", Start: &projectdomain.Start{Env: "BILLING_KAFKA_ENABLED", Default: boolPointer(false)}},
				{Name: "replay", GroupEnv: "REPLAY_GROUP"},
			},
			Producers: []projectdomain.KafkaProducer{{
				Name: "audit", Topic: "audit_service.audit.events.v1", TopicEnv: "AUDIT_TOPIC",
			}},
		}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			Config: &projectdomain.ConfigGenerator{Out: "gen/config"},
		}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "gen/config", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, tree artifact.Tree) (artifact.PublishResult, error) {
			tags := structFieldTags(t, tree.Files[0].Content, "KafkaConfig")
			require.Subset(t, tags, map[string]string{
				"AuditTopic":     `env:"SAMPLE_AUDIT_TOPIC" default:"audit_service.audit.events.v1"`,
				"BillingEnabled": `env:"SAMPLE_BILLING_KAFKA_ENABLED" default:"false"`,
				"BillingGroup":   `env:"SAMPLE_BILLING_GROUP" default:"sample-billing-group"`,
				"Brokers":        `env:"SAMPLE_KAFKA_BROKERS" default:"localhost:29092"`,
				"ReplayGroup":    `env:"SAMPLE_REPLAY_GROUP" default:"sample-replay-group"`,
			})
			require.Equal(t, `env:"SAMPLE_KAFKA_BILLING_BATCH_MAX_SIZE" default:"1"`, tags["BillingBatchMaxSize"])
			require.Equal(t, `env:"SAMPLE_KAFKA_BILLING_RETRY_MAX_ATTEMPTS" default:"3"`, tags["BillingRetryMaxAttempts"])
			require.Equal(t, `env:"SAMPLE_KAFKA_BILLING_REBALANCE_TIMEOUT" default:"30s"`, tags["BillingRebalanceTimeout"])
			return publishedDirectory(tree, artifact.PublishUpdated), nil
		},
	)
	workspace.EXPECT().PublishFile(gomock.Any(), project.Root, ".env.example", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, content []byte) (artifact.PublishResult, error) {
			for _, line := range []string{
				"SAMPLE_AUDIT_TOPIC=audit_service.audit.events.v1\n",
				"SAMPLE_BILLING_GROUP=sample-billing-group\n",
				"SAMPLE_BILLING_KAFKA_ENABLED=false\n",
				"SAMPLE_KAFKA_BILLING_BATCH_MAX_SIZE=1\n",
				"SAMPLE_KAFKA_BROKERS=localhost:29092\n",
				"SAMPLE_REPLAY_GROUP=sample-replay-group\n",
			} {
				require.Contains(t, string(content), line)
			}
			return artifact.PublishResult{Action: artifact.PublishUpdated}, nil
		},
	)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Workspace: workspace,
	})

	_, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Family: "config"})

	require.NoError(t, err)
}

func TestGenConfigBuildsRedisAndS3EnvWithoutSecretDefaults(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Project: projectdomain.Identity{Name: "sample"},
		Components: projectdomain.Components{
			Redis: &projectdomain.Redis{Connections: []projectdomain.RedisConnection{
				{Name: "cache", AddrEnv: "REDIS_CACHE_ADDR", AddrDefault: "redis://localhost:6379/1"},
				{Name: "ephemeral", AddrEnv: "REDIS_EPHEMERAL_ADDR"},
			}},
			S3: &projectdomain.S3{
				Connections: []projectdomain.S3Connection{{
					Name: "default", Credentials: "static", Endpoint: "http://localhost:9000",
					Region: "us-east-1", PathStyle: true, AccessKeyEnv: "S3_ACCESS_KEY_ID", SecretKeyEnv: "S3_SECRET_ACCESS_KEY",
				}},
				Buckets: []projectdomain.S3Bucket{{Name: "media", Connection: "default", Bucket: "media-local"}},
			},
		},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			Config: &projectdomain.ConfigGenerator{Out: "gen/config"},
		}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "gen/config", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, tree artifact.Tree) (artifact.PublishResult, error) {
			require.Equal(t, map[string]string{
				"CacheAddress":     `env:"SAMPLE_REDIS_CACHE_ADDR" default:"redis://localhost:6379/1"`,
				"EphemeralAddress": `env:"SAMPLE_REDIS_EPHEMERAL_ADDR"`,
			}, structFieldTags(t, tree.Files[0].Content, "RedisConfig"))
			require.Equal(t, map[string]string{
				"AccessKeyID":     `env:"SAMPLE_S3_ACCESS_KEY_ID"`,
				"Endpoint":        `env:"SAMPLE_S3_ENDPOINT" default:"http://localhost:9000"`,
				"ForcePathStyle":  `env:"SAMPLE_S3_FORCE_PATH_STYLE" default:"true"`,
				"MediaBucket":     `env:"SAMPLE_S3_MEDIA_BUCKET" default:"media-local"`,
				"Region":          `env:"SAMPLE_S3_REGION" default:"us-east-1"`,
				"SecretAccessKey": `env:"SAMPLE_S3_SECRET_ACCESS_KEY"`,
			}, structFieldTags(t, tree.Files[0].Content, "S3Config"))
			return publishedDirectory(tree, artifact.PublishUpdated), nil
		},
	)
	workspace.EXPECT().PublishFile(gomock.Any(), project.Root, ".env.example", []byte(
		"SAMPLE_REDIS_CACHE_ADDR=redis://localhost:6379/1\n"+
			"SAMPLE_REDIS_EPHEMERAL_ADDR=\n"+
			"SAMPLE_S3_ACCESS_KEY_ID=\n"+
			"SAMPLE_S3_ENDPOINT=http://localhost:9000\n"+
			"SAMPLE_S3_FORCE_PATH_STYLE=true\n"+
			"SAMPLE_S3_MEDIA_BUCKET=media-local\n"+
			"SAMPLE_S3_REGION=us-east-1\n"+
			"SAMPLE_S3_SECRET_ACCESS_KEY=\n",
	)).Return(artifact.PublishResult{Action: artifact.PublishUpdated}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Workspace: workspace,
	})

	_, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Family: "config"})

	require.NoError(t, err)
}

func TestGenConfigKeepsMigrationURLsOutOfRuntimeConfig(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Project: projectdomain.Identity{Name: "sample"},
		Components: projectdomain.Components{DB: &projectdomain.DB{Connections: []projectdomain.DBConnection{{
			Name: "primary", Default: "sqlite", Variants: []projectdomain.DBVariant{
				{Name: "sqlite", Kind: "sqlite", DSNEnv: "DB_PRIMARY_SQLITE_DSN", DSNDefault: "file:./data/primary.db?_foreign_keys=on", Migrations: &projectdomain.DBMigrations{
					Path: "migrations/primary/sqlite", DatabaseEnv: "DB_PRIMARY_SQLITE_MIGRATIONS_URL", DatabaseDefault: "sqlite://./data/primary.db?_pragma=foreign_keys%281%29",
				}},
				{Name: "postgres", Kind: "postgres", DSNEnv: "DB_PRIMARY_POSTGRES_DSN", Secret: true, Migrations: &projectdomain.DBMigrations{
					Path: "migrations/primary/postgres", DatabaseEnv: "DB_PRIMARY_POSTGRES_MIGRATIONS_URL",
				}},
			},
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			Config: &projectdomain.ConfigGenerator{Out: "gen/config"},
		}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "gen/config", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, tree artifact.Tree) (artifact.PublishResult, error) {
			require.Equal(t, map[string]string{
				"Kind":        `env:"SAMPLE_DB_PRIMARY_KIND" default:"sqlite"`,
				"PostgresDSN": `env:"SAMPLE_DB_PRIMARY_POSTGRES_DSN"`,
				"SqliteDSN":   `env:"SAMPLE_DB_PRIMARY_SQLITE_DSN" default:"file:./data/primary.db?_foreign_keys=on"`,
			}, structFieldTags(t, tree.Files[0].Content, "DBPrimaryConfig"))
			return publishedDirectory(tree, artifact.PublishUpdated), nil
		},
	)
	workspace.EXPECT().PublishFile(gomock.Any(), project.Root, ".env.example", []byte(
		"SAMPLE_DB_PRIMARY_KIND=sqlite\n"+
			"SAMPLE_DB_PRIMARY_POSTGRES_DSN=\n"+
			"SAMPLE_DB_PRIMARY_POSTGRES_MIGRATIONS_URL=\n"+
			"SAMPLE_DB_PRIMARY_SQLITE_DSN=file:./data/primary.db?_foreign_keys=on\n"+
			"SAMPLE_DB_PRIMARY_SQLITE_MIGRATIONS_URL=sqlite://./data/primary.db?_pragma=foreign_keys%281%29\n",
	)).Return(artifact.PublishResult{Action: artifact.PublishUpdated}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Workspace: workspace,
	})

	_, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Family: "config"})

	require.NoError(t, err)
}

func TestGenServiceRoutesGRPCTargetToProtoGeneratorAndPublishesOutput(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	generator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{
			Server: &projectdomain.GRPCServer{ProtoRoot: "api/proto", BufConfig: "buf.yaml"},
		}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			GRPC: &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"},
		}}},
	}}
	target := generationTarget(t, project.Manifest, "grpc-server")
	generated := generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{
		Path: "acme/v1/service.pb.go", Content: []byte("generated"), Mode: 0o644,
	}}}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	generator.EXPECT().Generate(gomock.Any(), project, target).Return(generated, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, generated.Directory).Return(publishedDirectory(generated.Directory, artifact.PublishUpdated), nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: generator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Family: "grpc", Target: "grpc-server",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"grpc-server"}, result.Targets)
	require.Equal(t, []generatedomain.Change{{
		Target: "grpc-server", Path: "gen/grpc/server/acme/v1/service.pb.go", Action: generatedomain.ChangeUpdated,
	}}, result.Changes)
}

func TestGenServiceReportsPreciseChangesInsideOnlyTheSelectedTarget(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	protoGenerator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"local": {Type: projectdomain.SourceLocal, Path: "api/contracts"}},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{
			Server: &projectdomain.GRPCServer{},
			Clients: []projectdomain.GRPCClient{{
				Name: "unrelated", Source: "local", Path: "proto/unrelated.proto", ProtoRoot: "proto",
			}},
		}},
	}}
	target := generationTarget(t, project.Manifest, "grpc-server")
	generated := generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{
		{Path: "changed.pb.go", Content: []byte("changed")},
		{Path: "created.pb.go", Content: []byte("created")},
		{Path: "equal.pb.go", Content: []byte("equal")},
	}}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	protoGenerator.EXPECT().Generate(gomock.Any(), project, target).Return(generated, nil)
	workspace.EXPECT().PublishDirectory(
		gomock.Any(), project.Root, target.OutputDir, generated.Directory,
	).Return(artifact.PublishResult{Action: artifact.PublishUpdated, Changes: []artifact.PublishChange{
		{Path: "changed.pb.go", Action: artifact.PublishUpdated},
		{Path: "created.pb.go", Action: artifact.PublishCreated},
		{Path: "equal.pb.go", Action: artifact.PublishUnchanged},
		{Path: "stale.pb.go", Action: artifact.PublishRemoved},
	}}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: protoGenerator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Target: target.ID,
	})

	require.NoError(t, err)
	require.Equal(t, []string{target.ID}, result.Targets)
	require.Equal(t, []generatedomain.Change{
		{Target: target.ID, Path: "gen/grpc/server/changed.pb.go", Action: generatedomain.ChangeUpdated},
		{Target: target.ID, Path: "gen/grpc/server/created.pb.go", Action: generatedomain.ChangeCreated},
		{Target: target.ID, Path: "gen/grpc/server/equal.pb.go", Action: generatedomain.ChangeUnchanged},
		{Target: target.ID, Path: "gen/grpc/server/stale.pb.go", Action: generatedomain.ChangeRemoved},
	}, result.Changes)
}

func TestGenServiceDefaultsGRPCServerProtoRoot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	protoGenerator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{
			Server: &projectdomain.GRPCServer{},
		}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			GRPC: &projectdomain.GRPCGenerator{},
		}}},
	}}
	target := generationTarget(t, project.Manifest, "grpc-server")
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	protoGenerator.EXPECT().Generate(gomock.Any(), project, target).Return(generatedomain.Output{}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, artifact.Tree{}).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: protoGenerator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Family: "grpc", Target: "grpc-server",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"grpc-server"}, result.Targets)
}

func TestGenServiceResolvesLocalGRPCClientProtoSelection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	generator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{
			"contracts": {Type: "local", Path: "api/contracts"},
		},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
			Name: "billing", Source: "contracts", Path: "proto/acme/billing/v1",
			ProtoRoot: "proto", BufGenConfig: "tools/buf/billing.gen.yaml",
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			GRPC: &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"},
		}}},
	}}
	target := generationTarget(t, project.Manifest, "grpc-client:billing")
	generated := generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{
		Path: "acme/billing/v1/billing.pb.go", Content: []byte("generated"), Mode: 0o644,
	}}}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	generator.EXPECT().Generate(gomock.Any(), project, target).Return(generated, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, generated.Directory).Return(publishedDirectory(generated.Directory, artifact.PublishUpdated), nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: generator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Family: "grpc", Target: "grpc-client:billing",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"grpc-client:billing"}, result.Targets)
}

func TestGenServiceUsesEveryGRPCClientsExplicitConfig(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	protoGenerator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{
			"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
		},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{
			{Name: "billing", Source: "contracts", Path: "proto/billing.proto", BufGenConfig: "tools/buf/billing.gen.yaml"},
			{Name: "orders", Source: "contracts", Path: "proto/orders.proto", BufGenConfig: "tools/buf/orders.gen.yaml"},
		}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			GRPC: &projectdomain.GRPCGenerator{Out: "gen/grpc"},
		}}},
	}}
	billing := generationTarget(t, project.Manifest, "grpc-client:billing")
	orders := generationTarget(t, project.Manifest, "grpc-client:orders")
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	protoGenerator.EXPECT().Generate(gomock.Any(), project, billing).Return(generatedomain.Output{}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, billing.OutputDir, artifact.Tree{}).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil)
	protoGenerator.EXPECT().Generate(gomock.Any(), project, orders).Return(generatedomain.Output{}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, orders.OutputDir, artifact.Tree{}).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: protoGenerator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Family: "grpc",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"grpc-client:billing", "grpc-client:orders"}, result.Targets)
	require.Equal(t, "tools/buf/billing.gen.yaml", billing.Config)
	require.Equal(t, "tools/buf/orders.gen.yaml", orders.Config)
}

func TestGenServiceUsesCommittedModuleRootForDevctlGRPCClient(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	protoGenerator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Paths:   projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceDevctl}},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
			Name: "billing", Source: "contracts", Export: "billing",
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			GRPC: &projectdomain.GRPCGenerator{Out: "gen/grpc", BufGenConfig: "tools/buf/grpc.gen.yaml"},
		}}},
	}}
	logicalTarget := generationTarget(t, project.Manifest, "grpc-client:billing")
	snapshot := contract.Snapshot{
		ModuleRoot: "api/proto/grpc",
		Metadata: &contract.Metadata{
			Kind: "grpc", Format: "proto", ModuleRoot: "api/proto/grpc", BufConfig: "buf.yaml",
		},
	}
	resolvedTarget := logicalTarget.WithSnapshot(snapshot)
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	inputs.EXPECT().Resolve(gomock.Any(), project, logicalTarget).Return(resolvedTarget, nil)
	protoGenerator.EXPECT().Generate(gomock.Any(), project, resolvedTarget).Return(generatedomain.Output{}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, resolvedTarget.OutputDir, artifact.Tree{}).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: inputs, Generator: protoGenerator, Workspace: workspace,
	})

	_, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Target: logicalTarget.ID})

	require.NoError(t, err)
	require.Equal(t, "api/external/grpc/client/billing/api/proto/grpc", resolvedTarget.Input)
}

func TestGenServiceResolvesKafkaProducerProtoSelection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	generator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{
			"events": {Type: "local", Path: "api/events"},
		},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
			Name: "invoice", Topic: "invoice_service.invoice.events.v1",
			Contract: projectdomain.KafkaContract{
				Source: "events", Path: "proto/invoice_service.invoice.events.v1.proto",
				Format: "proto", ProtoRoot: "proto", Message: "acme.invoice.v1.Invoice", Encoding: "binary",
			},
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			Kafka: &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"},
		}}},
	}}
	target := generationTarget(t, project.Manifest, "kafka-producer:invoice")
	generated := generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{
		Path: "acme/invoice/v1/invoice.pb.go", Content: []byte("generated"), Mode: 0o644,
	}}}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	generator.EXPECT().Generate(gomock.Any(), project, target).Return(generated, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, generated.Directory).Return(publishedDirectory(generated.Directory, artifact.PublishUpdated), nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: generator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Family: "kafka", Target: "kafka-producer:invoice",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"kafka-producer:invoice"}, result.Targets)
}

func TestGenServiceResolvesDevctlKafkaProtoFromCommittedMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	protoGenerator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	const topic = "invoice_service.invoice.events.v1"
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"events": {Type: projectdomain.SourceDevctl}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
			Name: "invoice", Topic: topic,
			Contract: projectdomain.KafkaContract{Source: "events", Export: "invoice", Format: "proto"},
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			Kafka: &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"},
		}}},
	}}
	logical := generationTarget(t, project.Manifest, "kafka-producer:invoice")
	snapshot := contract.Snapshot{
		ModuleRoot: "proto", Entrypoint: "proto/event.proto",
		Metadata: &contract.Metadata{
			Kind: "kafka", Topic: topic, Format: "proto",
			Entrypoint: "proto/event.proto", ModuleRoot: "proto", BufConfig: "buf.yaml",
		},
	}
	resolved := logical.WithSnapshot(snapshot)
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	inputs.EXPECT().Resolve(gomock.Any(), project, logical).Return(resolved, nil)
	protoGenerator.EXPECT().Generate(gomock.Any(), project, resolved).Return(generatedomain.Output{}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, resolved.OutputDir, artifact.Tree{}).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: inputs, Generator: protoGenerator, Workspace: workspace,
	})

	_, err := service.Generate(context.Background(), generatedomain.Command{
		ManifestPath: "devctl.yaml", Target: logical.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "api/external/kafka/producer/invoice/proto", resolved.Input)
	require.Equal(t, []string{"event.proto"}, resolved.Paths)
}

func TestGenKafkaJSONUsesJSONSchemaGenerator(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	jsonGenerator := mocks.NewMockGeneratorClient(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"events": {Type: "local", Path: "api/contracts"}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "audit", Topic: "audit_service.audit.created.v1",
			Contract: projectdomain.KafkaContract{Source: "events", Format: "json", Path: "schemas/audit_service.audit.created.v1.json"},
		}}}},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Generators: projectdomain.GoGenerators{
			Kafka: &projectdomain.KafkaGenerator{Out: "gen/kafka", BufGenConfig: "tools/buf/kafka.gen.yaml"},
		}}},
	}}
	target := generationTarget(t, project.Manifest, "kafka-consumer:audit")
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	jsonGenerator.EXPECT().Generate(gomock.Any(), project, target).Return(generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{
		Path: "schema.gen.go", Content: []byte("package audit\n"),
	}}}}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, target.OutputDir, gomock.Any()).Return(artifact.PublishResult{
		Action:  artifact.PublishUpdated,
		Changes: []artifact.PublishChange{{Path: "schema.gen.go", Action: artifact.PublishUpdated}},
	}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
		Projects: projects, Inputs: passthroughTargetResolver(ctrl), Generator: jsonGenerator, Workspace: workspace,
	})

	result, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
	require.Equal(t, []string{"kafka-consumer:audit"}, result.Targets)
	require.Equal(t, []generatedomain.Change{{
		Target: "kafka-consumer:audit", Path: "gen/kafka/consumer/audit/schema.gen.go", Action: generatedomain.ChangeUpdated,
	}}, result.Changes)
}

func TestGenServicePreservesTargetInputFailureSemantics(t *testing.T) {
	t.Parallel()

	t.Run("HTTP locate operation", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		projects := mocks.NewMockProjectRepository(ctrl)
		inputs := mocks.NewMockTargetResolver(ctrl)
		cause := errors.New("entrypoint unavailable")
		selected := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
			Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Server: &projectdomain.HTTPServer{
				OpenAPI: "api/openapi/swagger.yaml",
			}}},
		}}
		target := generationTarget(t, selected.Manifest, "http-server")
		projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(selected, nil)
		inputs.EXPECT().Resolve(gomock.Any(), selected, target).Return(target, cause)
		service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
			Projects: projects, Inputs: inputs,
		})

		_, err := service.Generate(context.Background(), generatedomain.Command{
			ManifestPath: "devctl.yaml", Target: target.ID,
		})

		var operationErr *generatedomain.OperationError
		require.ErrorAs(t, err, &operationErr)
		require.Equal(t, generatedomain.OperationLocateContract, operationErr.Operation)
		require.Equal(t, target.ID, operationErr.Target)
		require.Equal(t, target.Location.Entrypoint, operationErr.Path)
		require.Equal(t, generatedomain.FailureUnavailable, operationErr.Kind)
		require.ErrorIs(t, err, cause)
	})

	t.Run("committed metadata category", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		projects := mocks.NewMockProjectRepository(ctrl)
		inputs := mocks.NewMockTargetResolver(ctrl)
		metadataErr := &contract.SnapshotMetadataError{
			Field: "topic", Reason: contract.MetadataMismatch, Hint: "devctl sync",
		}
		selected := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
			Sources: map[string]projectdomain.Source{"upstream": {Type: projectdomain.SourceDevctl}},
			Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
				Name: "billing", Source: "upstream", Export: "billing",
			}}}},
		}}
		target := generationTarget(t, selected.Manifest, "grpc-client:billing")
		projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(selected, nil)
		inputs.EXPECT().Resolve(gomock.Any(), selected, target).Return(target, metadataErr)
		service := generateservice.New(zap.NewNop(), generateservice.Dependencies{
			Projects: projects, Inputs: inputs,
		})

		_, err := service.Generate(context.Background(), generatedomain.Command{
			ManifestPath: "devctl.yaml", Target: target.ID,
		})

		require.ErrorIs(t, err, metadataErr)
		require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	})
}

func TestGenServiceAppliesCatalogSelectionContract(t *testing.T) {
	t.Parallel()

	manifest := projectdomain.Manifest{Project: projectdomain.Identity{Language: "go"}}
	tests := []struct {
		name     string
		command  generatedomain.Command
		targets  []string
		category failure.Category
	}{
		{name: "known empty family", command: generatedomain.Command{Family: "grpc", DryRun: true}, targets: []string{}},
		{name: "known configured family", command: generatedomain.Command{Family: "config", DryRun: true}, targets: []string{"config"}},
		{name: "unknown family", command: generatedomain.Command{Family: "other", DryRun: true}, category: failure.InvalidInput},
		{name: "unknown target", command: generatedomain.Command{Target: "grpc-client:missing", DryRun: true}, category: failure.NotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects := mocks.NewMockProjectRepository(ctrl)
			projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(projectdomain.Project{Root: "/project", Manifest: manifest}, nil)
			service := generateservice.New(zap.NewNop(), generateservice.Dependencies{Projects: projects})
			command := test.command
			command.ManifestPath = "devctl.yaml"

			result, err := service.Generate(context.Background(), command)

			if test.category != "" {
				require.Equal(t, test.category, failure.CategoryOf(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.targets, result.Targets)
		})
	}
}

func TestGenExecutionOrderIsIndependentFromCatalogIDOrder(t *testing.T) {
	t.Parallel()

	manifest := projectdomain.Manifest{
		Project: projectdomain.Identity{Language: "go"},
		Sources: map[string]projectdomain.Source{
			"local": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
		},
		Components: projectdomain.Components{
			HTTP: &projectdomain.HTTP{
				Server:  &projectdomain.HTTPServer{},
				Clients: []projectdomain.HTTPClient{{Name: "billing", Source: "local", Path: "openapi.yaml"}},
			},
			GRPC: &projectdomain.GRPC{
				Server:  &projectdomain.GRPCServer{},
				Clients: []projectdomain.GRPCClient{{Name: "billing", Source: "local", Path: "proto/billing.proto", ProtoRoot: "proto"}},
			},
			Kafka: &projectdomain.Kafka{
				Consumers: []projectdomain.KafkaConsumer{{Name: "audit", Topic: "sample.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"}}},
				Producers: []projectdomain.KafkaProducer{{Name: "audit", Topic: "sample.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"}}},
			},
		},
	}
	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(projectdomain.Project{Root: "/project", Manifest: manifest}, nil)
	service := generateservice.New(zap.NewNop(), generateservice.Dependencies{Projects: projects})

	result, err := service.Generate(context.Background(), generatedomain.Command{ManifestPath: "devctl.yaml", DryRun: true})

	require.NoError(t, err)
	require.Equal(t, []string{
		"config",
		"http-server", "http-client:billing",
		"grpc-server", "grpc-client:billing",
		"kafka-consumer:audit", "kafka-producer:audit",
	}, result.Targets)
	require.Equal(t, []string{
		"config",
		"grpc-client:billing", "grpc-server",
		"http-client:billing", "http-server",
		"kafka-consumer:audit", "kafka-producer:audit",
	}, targetIDs(projectdomain.NewTargetCatalog(manifest).All()))
}

func targetIDs(targets []projectdomain.Target) []string {
	ids := make([]string, len(targets))
	for index, target := range targets {
		ids[index] = target.ID
	}
	return ids
}

func passthroughTargetResolver(ctrl *gomock.Controller) *mocks.MockTargetResolver {
	resolver := mocks.NewMockTargetResolver(ctrl)
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ projectdomain.Project, target projectdomain.Target) (projectdomain.Target, error) {
			return target, nil
		},
	).AnyTimes()
	return resolver
}

func publishedDirectory(tree artifact.Tree, action artifact.PublishAction) artifact.PublishResult {
	changes := make([]artifact.PublishChange, len(tree.Files))
	for index, file := range tree.Files {
		changes[index] = artifact.PublishChange{Path: file.Path, Action: action}
	}
	return artifact.PublishResult{Action: action, Changes: changes}
}

type fieldTagExpectation struct {
	Struct string
	Field  string
	Tag    string
}

func generationTarget(t *testing.T, manifest projectdomain.Manifest, id string) projectdomain.Target {
	t.Helper()
	targets := projectdomain.NewTargetCatalog(manifest).Select(projectdomain.TargetOperationGenerate, "", id)
	require.Len(t, targets, 1)
	return targets[0]
}

func requireStructFieldTag(t *testing.T, source []byte, expected fieldTagExpectation) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "config.gen.go", source, parser.AllErrors)
	require.NoError(t, err)
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		declaration, ok := node.(*ast.TypeSpec)
		if !ok || declaration.Name.Name != expected.Struct {
			return true
		}
		structure, ok := declaration.Type.(*ast.StructType)
		require.True(t, ok)
		for _, field := range structure.Fields.List {
			if len(field.Names) == 1 && field.Names[0].Name == expected.Field {
				require.NotNil(t, field.Tag)
				tag, unquoteErr := strconv.Unquote(field.Tag.Value)
				require.NoError(t, unquoteErr)
				require.Equal(t, expected.Tag, tag)
				found = true
			}
		}
		return false
	})
	require.True(t, found, "%s.%s was not generated", expected.Struct, expected.Field)
}

func structFieldTags(t *testing.T, source []byte, structName string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "config.gen.go", source, parser.AllErrors)
	require.NoError(t, err)
	fields := map[string]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		declaration, ok := node.(*ast.TypeSpec)
		if !ok || declaration.Name.Name != structName {
			return true
		}
		structure, ok := declaration.Type.(*ast.StructType)
		require.True(t, ok)
		for _, field := range structure.Fields.List {
			if len(field.Names) != 1 || field.Tag == nil {
				continue
			}
			tag, unquoteErr := strconv.Unquote(field.Tag.Value)
			require.NoError(t, unquoteErr)
			fields[field.Names[0].Name] = tag
		}
		return false
	})
	return fields
}

func boolPointer(value bool) *bool { return &value }
