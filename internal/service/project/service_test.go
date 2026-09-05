package project_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	domainfailure "github.com/devctllabs/devctl/internal/domain/failure"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestServiceValidateSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version:   1,
			Project:   projectdomain.Identity{Name: "example", Language: "go"},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"})

	require.NoError(t, err)
	require.True(t, result.IsValid())
	require.Empty(t, result.Issues)
}

func TestServiceValidateRejectsRuntimeConfigConflicts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1,
			Project: projectdomain.Identity{Name: "example", Language: "go"},
			Env: projectdomain.Env{Custom: []projectdomain.EnvGroup{
				{Group: "service", Vars: []projectdomain.EnvVar{{Key: "MODE", Type: "string"}}},
				{Group: "worker", Vars: []projectdomain.EnvVar{{Key: "MODE", Type: "bool"}}},
			}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(
		context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, projectdomain.ValidationResult{Issues: []projectdomain.Issue{{
		Code: projectdomain.IssueRuntimeConfigConflict, Path: "/project/devctl.yaml", Field: "env",
	}}}, result)
}

func TestServiceValidateAcceptsGitSourceRoot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Sources: map[string]projectdomain.Source{"contracts": {
				Type: projectdomain.SourceGit, Path: "contracts", Repo: "example/contracts", Ref: "v1",
			}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"})

	require.NoError(t, err)
	require.True(t, result.IsValid())
}

func TestServiceValidateDiscoversManifestFromWorkingDirectory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version:   1,
			Project:   projectdomain.Identity{Name: "example", Language: "go"},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project/nested", nil),
		workspace.EXPECT().RegularFile(gomock.Any(), "/project/nested", "devctl.yaml").Return(false, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), "/project", "devctl.yaml").Return(true, nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(context.Background(), projectdomain.ValidateQuery{})

	require.NoError(t, err)
	require.True(t, result.IsValid())
}

func TestServiceValidateReturnsAllManifestIssues(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{Version: 1, Project: projectdomain.Identity{Language: "go"}},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	readiness := mocks.NewMockReadinessChecker(ctrl)
	readiness.EXPECT().Check(gomock.Any(), selected).Return([]projectdomain.Issue{{
		Code: projectdomain.IssueGoModMissing, Path: selected.ManifestPath, Field: "go.mod",
	}}, nil)
	result, err := project.New(zap.NewNop(), project.Dependencies{
		Manifests: manifests, Locator: workspace, Readiness: readiness,
	}).Validate(context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"})

	require.NoError(t, err)
	require.Equal(t, projectdomain.ValidationResult{Issues: []projectdomain.Issue{
		{Code: "name_invalid", Path: "/project/devctl.yaml", Field: "project.name"},
		{Code: "go_module_required", Path: "/project/devctl.yaml", Field: "languages.go.module"},
		{Code: "go_mod_missing", Path: "/project/devctl.yaml", Field: "go.mod"},
	}}, result)
}

func TestServiceValidatePreservesReadinessErrorAfterSemanticIssues(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	readiness := mocks.NewMockReadinessChecker(ctrl)
	cause := errors.New("workspace unavailable")
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version:   1,
			Project:   projectdomain.Identity{Language: "go"},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
		readiness.EXPECT().Check(gomock.Any(), selected).Return(nil, &projectdomain.OperationError{
			Operation: projectdomain.OperationInspectFile,
			Path:      "go.mod",
			Kind:      projectdomain.FailureUnavailable,
			Cause:     cause,
		}),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{
		Manifests: manifests, Locator: workspace, Readiness: readiness,
	}).Validate(context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"})

	var operationErr *projectdomain.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, projectdomain.OperationInspectFile, operationErr.Operation)
	require.Equal(t, "go.mod", operationErr.Path)
	require.Equal(t, projectdomain.FailureUnavailable, operationErr.Kind)
	require.ErrorIs(t, err, cause)
	require.Equal(t, projectdomain.ValidationResult{}, result)
}

func TestServiceValidateAppendsInjectedReadinessIssuesAfterSemanticIssues(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	readiness := mocks.NewMockReadinessChecker(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Language: "go"},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	readinessIssue := projectdomain.Issue{
		Code: projectdomain.IssueGoModMissing, Path: selected.ManifestPath, Field: "go.mod",
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
		readiness.EXPECT().Check(gomock.Any(), selected).Return([]projectdomain.Issue{readinessIssue}, nil),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{
		Manifests: manifests, Locator: workspace, Readiness: readiness,
	}).Validate(context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"})

	require.NoError(t, err)
	require.Equal(t, []projectdomain.Issue{
		{Code: projectdomain.IssueNameInvalid, Path: selected.ManifestPath, Field: "project.name"},
		readinessIssue,
	}, result.Issues)
}

func TestServiceValidateRejectsGRPCClientWithUnknownSource(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
				Name: "billing", Source: "missing", Path: "proto/billing", ProtoRoot: "proto",
			}}}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(
		context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, projectdomain.ValidationResult{Issues: []projectdomain.Issue{{
		Code: projectdomain.IssueSourceNotFound, Path: "/project/devctl.yaml",
		Field: "components.grpc.clients.billing.source",
	}}}, result)
}

func TestServiceInspectReportsGRPCAndKafkaTargets(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Sources: map[string]projectdomain.Source{"contracts": {Type: "local", Path: "api/contracts"}},
			Components: projectdomain.Components{
				GRPC: &projectdomain.GRPC{
					Server: &projectdomain.GRPCServer{ProtoRoot: "api/proto"},
					Clients: []projectdomain.GRPCClient{{
						Name: "billing", Source: "contracts", Path: "proto/billing", ProtoRoot: "proto",
						BufGenConfig: "tools/buf/billing.gen.yaml",
					}},
				},
				Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
					Name: "audit", Topic: "audit_service.audit.events.v1", Contract: projectdomain.KafkaContract{Format: "raw"},
				}}},
			},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{
				Module: "example.test/example",
				Generators: projectdomain.GoGenerators{GRPC: &projectdomain.GRPCGenerator{
					Out: "generated/grpc", BufGenConfig: "tools/buf/shared.gen.yaml",
				}},
			}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)
	service := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace})

	result, err := service.Inspect(context.Background(), projectdomain.InspectQuery{ManifestPath: "devctl.yaml"})

	require.NoError(t, err)
	require.Equal(t, []projectdomain.InspectionTarget{
		{ID: "config", Family: "config", Format: "go", Output: "gen/config"},
		{ID: "grpc-client:billing", Family: "grpc", Format: "proto", Input: "api/contracts/proto", Config: "tools/buf/billing.gen.yaml", Output: "generated/grpc/client/billing"},
		{ID: "grpc-server", Family: "grpc", Format: "proto", Input: "api/proto", Config: "tools/buf/shared.gen.yaml", Output: "generated/grpc/server"},
		{ID: "kafka-producer:audit", Family: "kafka", Format: "raw"},
	}, result.Project.Targets)
}

func TestServiceInspectAddsResolvedInputOnlyFromValidCommittedMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Sources: map[string]projectdomain.Source{
				"upstream": {Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"},
			},
			Components: projectdomain.Components{
				GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
					Name: "billing", Source: "upstream", Export: "billing",
				}}},
				Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
					Name: "audit", Topic: "audit_service.audit.events.v1",
					Contract: projectdomain.KafkaContract{Format: "json", Source: "upstream", Export: "audit"},
				}}},
			},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	grpcSnapshot := contract.Snapshot{
		ModuleRoot: "api/proto/grpc",
		Metadata: &contract.Metadata{
			Kind: "grpc", Format: "proto", ModuleRoot: "api/proto/grpc", BufConfig: "buf.yaml",
		},
	}
	staleMetadata := &contract.SnapshotMetadataError{
		Field: "entrypoint", Reason: contract.MetadataMismatch, Hint: "devctl sync",
	}
	targets := projectdomain.NewTargetCatalog(selected.Manifest).All()
	grpcTarget, kafkaTarget := targets[1], targets[2]
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)
	inputs.EXPECT().Resolve(gomock.Any(), selected, grpcTarget).Return(grpcTarget.WithSnapshot(grpcSnapshot), nil)
	inputs.EXPECT().Resolve(gomock.Any(), selected, kafkaTarget).Return(kafkaTarget, staleMetadata)

	result, err := project.New(zap.NewNop(), project.Dependencies{
		Manifests: manifests, Locator: workspace, Inputs: inputs,
	}).Inspect(
		context.Background(), projectdomain.InspectQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.InspectionTarget{
		{ID: "config", Family: "config", Format: "go", Output: "gen/config"},
		{
			ID: "grpc-client:billing", Family: "grpc", Format: "proto",
			Input:         "api/external/grpc/client/billing",
			ResolvedInput: "api/external/grpc/client/billing/api/proto/grpc",
			Config:        "tools/buf/grpc.gen.yaml", Output: "gen/grpc/client/billing",
		},
		{
			ID: "kafka-consumer:audit", Family: "kafka", Format: "json",
			Input: "api/external/kafka/consumer/audit", Output: "gen/kafka/consumer/audit",
		},
	}, result.Project.Targets)
}

func TestServiceInspectRejectsUnsafeCatalogOutputPathsForEveryGeneratorFamily(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Sources: map[string]projectdomain.Source{
				"contracts": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
			},
			Components: projectdomain.Components{
				GRPC: &projectdomain.GRPC{Server: &projectdomain.GRPCServer{}},
				Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
					Name: "audit", Topic: "audit_service.audit.events.v1",
					Contract: projectdomain.KafkaContract{
						Format: "proto", Source: "contracts", Path: "proto/audit.proto", ProtoRoot: "proto",
					},
				}}},
			},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{
				Module: "example.test/example",
				Generators: projectdomain.GoGenerators{
					GRPC:  &projectdomain.GRPCGenerator{Out: "/tmp/grpc"},
					Kafka: &projectdomain.KafkaGenerator{Out: "../kafka"},
				},
			}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	_, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Inspect(
		context.Background(), projectdomain.InspectQuery{ManifestPath: "devctl.yaml"},
	)

	var invalid *projectdomain.InvalidManifestError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, []projectdomain.Issue{
		{Code: projectdomain.IssuePathInvalid, Path: selected.ManifestPath, Field: "languages.go.generators.grpc.out"},
		{Code: projectdomain.IssuePathInvalid, Path: selected.ManifestPath, Field: "languages.go.generators.kafka.out"},
	}, invalid.Issues)
}

func TestServiceInspectReportsMigrationResourcesAndEnvironment(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{DB: &projectdomain.DB{Connections: []projectdomain.DBConnection{{
				Name: "primary", Default: "sqlite", Variants: []projectdomain.DBVariant{
					{Name: "sqlite", Kind: "sqlite", Migrations: &projectdomain.DBMigrations{Path: "migrations/primary/sqlite", DatabaseEnv: "DB_PRIMARY_SQLITE_MIGRATIONS_URL", DatabaseDefault: "sqlite://./data/primary.db"}},
					{Name: "memory", Kind: "sqlite"},
					{Name: "postgres", Kind: "postgres", Migrations: &projectdomain.DBMigrations{Path: "migrations/primary/postgres", DatabaseEnv: "DB_PRIMARY_POSTGRES_MIGRATIONS_URL"}},
				},
			}}}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Inspect(
		context.Background(), projectdomain.InspectQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"primary"}, result.Project.Resources.DBConnections)
	require.Equal(t, []projectdomain.InspectionMigration{
		{Connection: "primary", Variant: "postgres", Kind: "postgres", Path: "migrations/primary/postgres", DatabaseEnv: "EXAMPLE_DB_PRIMARY_POSTGRES_MIGRATIONS_URL"},
		{Connection: "primary", Variant: "sqlite", Kind: "sqlite", Path: "migrations/primary/sqlite", DatabaseEnv: "EXAMPLE_DB_PRIMARY_SQLITE_MIGRATIONS_URL"},
	}, result.Project.Resources.Migrations)
	require.Equal(t, []projectdomain.EffectiveEnv{
		{Key: "EXAMPLE_DB_PRIMARY_KIND", Type: "string", Default: "sqlite"},
		{Key: "EXAMPLE_DB_PRIMARY_MEMORY_DSN", Type: "string"},
		{Key: "EXAMPLE_DB_PRIMARY_POSTGRES_DSN", Type: "string"},
		{Key: "EXAMPLE_DB_PRIMARY_POSTGRES_MIGRATIONS_URL", Type: "string", Secret: true},
		{Key: "EXAMPLE_DB_PRIMARY_SQLITE_DSN", Type: "string"},
		{Key: "EXAMPLE_DB_PRIMARY_SQLITE_MIGRATIONS_URL", Type: "string", Default: "sqlite://./data/primary.db"},
	}, result.Project.Env)
}

func TestServiceInspectUsesCanonicalRuntimeConfigPolicy(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "sample-api", Language: "go"},
			Components: projectdomain.Components{
				Logging: &projectdomain.Logging{},
				HTTP: &projectdomain.HTTP{Server: &projectdomain.HTTPServer{Start: &projectdomain.Start{
					Env: "SERVE_HTTP",
				}}},
			},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/sample-api"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Inspect(
		context.Background(), projectdomain.InspectQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.EffectiveEnv{
		{Key: "SAMPLE_API_HTTP_ADDR", Type: "string", Default: ":8080"},
		{Key: "SAMPLE_API_LOG_LEVEL", Type: "string", Default: "info"},
		{Key: "SAMPLE_API_SERVE_HTTP", Type: "bool", Default: false},
	}, result.Project.Env)
}

func TestServiceValidateReturnsMissingManifestAsExecutionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/missing.yaml").Return(projectdomain.LoadManifestResult{}, fs.ErrNotExist),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Validate(context.Background(), projectdomain.ValidateQuery{ManifestPath: "missing.yaml"})

	require.Equal(t, domainfailure.NotFound, domainfailure.CategoryOf(err))
	var operationErr *projectdomain.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, projectdomain.OperationLoadManifest, operationErr.Operation)
	require.Equal(t, "/project/missing.yaml", operationErr.Path)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.EqualError(t, err, "manifests.Load: load_manifest failed: file does not exist")
	require.Equal(t, projectdomain.ValidationResult{}, result)
}

func TestServiceValidatePreservesLoadErrorWhenResultAlsoContainsIssues(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	loadErr := errors.New("storage unavailable")
	workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil)
	manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{
		Project: projectdomain.Project{ManifestPath: "/project/devctl.yaml"},
		Issues:  []projectdomain.DecodeIssue{{Kind: projectdomain.DecodeYAMLInvalid}},
	}, loadErr)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Validate(
		context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
	)

	require.ErrorIs(t, err, loadErr)
	require.Equal(t, domainfailure.Unavailable, domainfailure.CategoryOf(err))
	require.Equal(t, projectdomain.ValidationResult{}, result)
}

func TestServiceLoadProjectPreservesLoadErrorWhenResultAlsoContainsIssues(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	loadErr := errors.New("storage unavailable")
	workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil)
	manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{
		Project: projectdomain.Project{ManifestPath: "/project/devctl.yaml"},
		Issues:  []projectdomain.DecodeIssue{{Kind: projectdomain.DecodeYAMLInvalid}},
	}, loadErr)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "devctl.yaml")

	require.ErrorIs(t, err, loadErr)
	require.Equal(t, domainfailure.Unavailable, domainfailure.CategoryOf(err))
	require.Equal(t, projectdomain.Project{}, result)
}

func TestServiceLoadProjectUsesAbsoluteManifestPathWithoutWorkingDirectory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version:   1,
			Project:   projectdomain.Identity{Name: "example", Language: "go"},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "/project/devctl.yaml")

	require.NoError(t, err)
	require.Equal(t, selected, result)
}

func TestServiceOwnsEnableCommandOutcome(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{ManifestPath: "custom.yaml", Manifest: projectdomain.Manifest{
		Version:    1,
		Project:    projectdomain.Identity{Name: "example", Language: "go"},
		Components: projectdomain.Components{},
		Languages:  projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
	}}
	updated := selected
	updated.Manifest.Components.Logging = &projectdomain.Logging{}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/custom.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
		manifests.EXPECT().Save(gomock.Any(), updated).Return(true, nil),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Enable(context.Background(), projectdomain.EnableCommand{
		ManifestPath: "custom.yaml", Capability: "logging",
	})

	require.NoError(t, err)
	require.Equal(t, projectdomain.ManifestResult{Manifest: "custom.yaml", Change: projectdomain.ChangeUpdated}, result)
}

func TestServiceReturnsTypedMutationFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{ManifestPath: "custom.yaml", Manifest: projectdomain.Manifest{
		Version:   1,
		Project:   projectdomain.Identity{Name: "example", Language: "go"},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
	}}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/custom.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).Enable(context.Background(), projectdomain.EnableCommand{
		ManifestPath: "custom.yaml", Capability: "logging", Always: true,
	})

	var mutationErr *projectdomain.MutationError
	require.ErrorAs(t, err, &mutationErr)
	require.Equal(t, projectdomain.MutationUnsupportedOption, mutationErr.Reason)
	require.Equal(t, "always", mutationErr.Field)
	require.Equal(t, domainfailure.InvalidInput, domainfailure.CategoryOf(err))
	require.Equal(t, projectdomain.ManifestResult{Manifest: "custom.yaml"}, result)
}

func TestServiceAddSourceRejectsPathForDevctlSource(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{ManifestPath: "custom.yaml", Manifest: projectdomain.Manifest{
		Version:   1,
		Project:   projectdomain.Identity{Name: "example", Language: "go"},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
	}}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/custom.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).AddSource(context.Background(), projectdomain.AddSourceCommand{
		ManifestPath: "custom.yaml", Name: "contracts", Type: "devctl", Path: "contracts",
		Repo: "example/contracts", Ref: "v1",
	})

	var mutationErr *projectdomain.MutationError
	require.ErrorAs(t, err, &mutationErr)
	require.Equal(t, projectdomain.MutationInvalidOptions, mutationErr.Reason)
	require.Equal(t, "source", mutationErr.Field)
	require.Equal(t, domainfailure.InvalidInput, domainfailure.CategoryOf(err))
	require.Equal(t, projectdomain.ManifestResult{Manifest: "custom.yaml"}, result)
}

func TestServiceOwnsInitManifestOutcome(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	command := projectdomain.InitManifestCommand{Destination: "/project/custom.yaml", Language: "go", Preset: "cli", Name: "sample", Module: "example.test/sample"}
	gomock.InOrder(
		manifests.EXPECT().Load(gomock.Any(), "/project/custom.yaml").Return(projectdomain.LoadManifestResult{}, fs.ErrNotExist),
		manifests.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, saved projectdomain.Project) (bool, error) {
			require.Nil(t, saved.Manifest.Languages.Go.Generators.Config)
			return true, nil
		}),
	)

	result, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests}).InitManifest(context.Background(), command)

	require.NoError(t, err)
	require.Equal(t, projectdomain.ManifestResult{Manifest: "/project/custom.yaml", Change: projectdomain.ChangeCreated}, result)
}
