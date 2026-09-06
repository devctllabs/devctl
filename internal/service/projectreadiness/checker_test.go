package projectreadiness_test

import (
	"context"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/projectreadiness"
	"github.com/devctllabs/devctl/internal/service/projectreadiness/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCheckerReportsMissingGoModule(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{Root: "/project", ManifestPath: "/project/devctl.yaml"}
	workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(false, nil)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{{
		Code: project.IssueGoModMissing, Path: selected.ManifestPath, Field: "go.mod",
	}}, issues)
}

func TestCheckerPreservesGoModuleInspectionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{Root: "/project", ManifestPath: "/project/devctl.yaml"}
	cause := errors.New("permission denied")
	workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(false, cause)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.Empty(t, issues)
	require.ErrorIs(t, err, cause)
	var operationErr *project.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, project.OperationInspectFile, operationErr.Operation)
	require.Equal(t, "go.mod", operationErr.Path)
	require.Equal(t, project.FailureUnavailable, operationErr.Kind)
}

func TestCheckerChecksLocalSourcesInNameOrder(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{Sources: map[string]project.Source{
			"bravo": {Type: project.SourceLocal, Path: "contracts/bravo"},
			"alpha": {
				Type: project.SourceLocal, Path: "contracts/alpha",
				Proto: project.SourceProto{BufConfig: "buf.yaml"},
			},
		}},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "contracts/alpha").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "contracts/alpha/buf.yaml").Return(false, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "contracts/bravo").Return(false, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{
		{Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: "contracts/alpha/buf.yaml"},
		{Code: project.IssueSourceMissing, Path: selected.ManifestPath, Field: "contracts/bravo"},
	}, issues)
}

func TestCheckerPreservesLocalSourceInspectionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{Sources: map[string]project.Source{
			"contracts": {Type: project.SourceLocal, Path: "api/contracts"},
		}},
	}
	cause := errors.New("stat failed")
	workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil)
	workspace.EXPECT().Directory(gomock.Any(), selected.Root, "api/contracts").Return(false, cause)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.Empty(t, issues)
	require.ErrorIs(t, err, cause)
	var operationErr *project.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, project.OperationInspectFile, operationErr.Operation)
	require.Equal(t, "api/contracts", operationErr.Path)
	require.Equal(t, project.FailureUnavailable, operationErr.Kind)
}

func TestCheckerChecksValidMigrationDirectoriesInPathOrder(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{Components: project.Components{DB: &project.DB{
			Connections: []project.DBConnection{{Variants: []project.DBVariant{
				{Kind: "postgres", Migrations: &project.DBMigrations{Path: "migrations/zeta", DatabaseEnv: "DATABASE_URL"}},
				{Kind: "sqlite", Migrations: &project.DBMigrations{Path: "../unsafe", DatabaseEnv: "DATABASE_URL"}},
				{Kind: "clickhouse", Migrations: &project.DBMigrations{Path: "migrations/alpha", DatabaseEnv: "DATABASE_URL"}},
			}}},
		}}},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "migrations/alpha").Return(false, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "migrations/zeta").Return(false, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{
		{Code: project.IssueMigrationPathMissing, Path: selected.ManifestPath, Field: "migrations/alpha"},
		{Code: project.IssueMigrationPathMissing, Path: selected.ManifestPath, Field: "migrations/zeta"},
	}, issues)
}

func TestCheckerChecksMigrationDirectoriesWithMatchingDatabaseDefaults(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{Components: project.Components{DB: &project.DB{
			Connections: []project.DBConnection{{Variants: []project.DBVariant{
				{Kind: "sqlite", Migrations: &project.DBMigrations{Path: "migrations/sqlite", DatabaseEnv: "SQLITE_URL", DatabaseDefault: "sqlite://data/app.db"}},
				{Kind: "postgres", Migrations: &project.DBMigrations{Path: "migrations/postgres", DatabaseEnv: "POSTGRES_URL", DatabaseDefault: "postgresql://localhost/app"}},
				{Kind: "clickhouse", Migrations: &project.DBMigrations{Path: "migrations/clickhouse", DatabaseEnv: "CLICKHOUSE_URL", DatabaseDefault: "clickhouse://localhost/app"}},
				{Kind: "postgres", Migrations: &project.DBMigrations{Path: "migrations/mismatch", DatabaseEnv: "MISMATCH_URL", DatabaseDefault: "sqlite://data/app.db"}},
			}}},
		}}},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "migrations/clickhouse").Return(true, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "migrations/postgres").Return(true, nil),
		workspace.EXPECT().Directory(gomock.Any(), selected.Root, "migrations/sqlite").Return(true, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestCheckerPreservesMigrationDirectoryInspectionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{Components: project.Components{DB: &project.DB{
			Connections: []project.DBConnection{{Variants: []project.DBVariant{{
				Kind: "postgres", Migrations: &project.DBMigrations{Path: "migrations/app", DatabaseEnv: "DATABASE_URL"},
			}}}},
		}}},
	}
	cause := errors.New("migration stat failed")
	workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil)
	workspace.EXPECT().Directory(gomock.Any(), selected.Root, "migrations/app").Return(false, cause)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.Empty(t, issues)
	require.ErrorIs(t, err, cause)
	var operationErr *project.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, project.OperationInspectFile, operationErr.Operation)
	require.Equal(t, "migrations/app", operationErr.Path)
}

func TestCheckerReportsHTTPFilesAndGeneratorAfterGoModule(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{Components: project.Components{HTTP: &project.HTTP{
			Server: &project.HTTPServer{},
		}}},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(false, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "api/openapi/swagger.yaml").Return(false, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{
		{Code: project.IssueGoModMissing, Path: selected.ManifestPath, Field: "go.mod"},
		{Code: project.IssueOpenAPIMissing, Path: selected.ManifestPath, Field: "api/openapi/swagger.yaml"},
		{Code: project.IssueHTTPGeneratorMissing, Path: selected.ManifestPath, Field: "languages.go.generators.http"},
	}, issues)
}

func TestCheckerChecksHTTPConfigsAndOAPICodegenTool(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Sources: map[string]project.Source{
				"contracts": {Type: project.SourceGit, Repo: "example/contracts", Ref: "v1"},
			},
			Components: project.Components{HTTP: &project.HTTP{
				Server: &project.HTTPServer{OpenAPI: "api/service.yaml"},
				Clients: []project.HTTPClient{
					{Name: "zeta", Source: "contracts", Path: "zeta.yaml", OAPIConfig: "tools/oapi/zeta.yaml"},
					{Name: "alpha", Source: "contracts", Path: "alpha.yaml", OAPIConfig: "tools/oapi/alpha.yaml"},
				},
			}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				HTTP: &project.HTTPGenerator{OAPIConfig: "tools/oapi/server.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "api/service.yaml").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/oapi/alpha.yaml").Return(false, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/oapi/server.yaml").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/oapi/zeta.yaml").Return(false, nil),
		workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return([]byte("module example.test/example\n"), nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{
		{Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: "tools/oapi/alpha.yaml"},
		{Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: "tools/oapi/zeta.yaml"},
		{Code: project.IssueToolMissing, Path: selected.ManifestPath, Field: "go.mod"},
	}, issues)
}

func TestCheckerReportsInvalidGoModuleWhenHTTPNeedsTool(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Components: project.Components{HTTP: &project.HTTP{}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				HTTP: &project.HTTPGenerator{},
			}}},
		},
	}
	workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil)
	workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return([]byte("module [invalid"), nil)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{{
		Code: project.IssueGoModInvalid, Path: selected.ManifestPath, Field: "go.mod",
	}}, issues)
}

func TestCheckerPreservesGoModuleReadErrorWhenHTTPNeedsTool(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Components: project.Components{HTTP: &project.HTTP{}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				HTTP: &project.HTTPGenerator{},
			}}},
		},
	}
	cause := errors.New("read failed")
	workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil)
	workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return(nil, cause)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.Empty(t, issues)
	require.ErrorIs(t, err, cause)
	var operationErr *project.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, project.OperationReadFile, operationErr.Operation)
	require.Equal(t, "go.mod", operationErr.Path)
}

func TestCheckerChecksGRPCModuleAndGeneratorConfigsWithoutGoModule(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Components: project.Components{GRPC: &project.GRPC{
				Server: &project.GRPCServer{BufConfig: "buf.yaml"},
			}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				GRPC: &project.GRPCGenerator{BufGenConfig: "tools/buf/grpc.gen.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(false, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "buf.yaml").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/grpc.gen.yaml").Return(false, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{
		{Code: project.IssueGoModMissing, Path: selected.ManifestPath, Field: "go.mod"},
		{Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: "tools/buf/grpc.gen.yaml"},
	}, issues)
}

func TestCheckerChecksGRPCClientOnlyGeneratorAndBufTool(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Sources: map[string]project.Source{
				"contracts": {Type: project.SourceGit, Repo: "example/contracts", Ref: "v1"},
			},
			Components: project.Components{GRPC: &project.GRPC{Clients: []project.GRPCClient{{
				Name: "billing", Source: "contracts", Path: "proto/billing.proto", ProtoRoot: "proto",
			}}}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				GRPC: &project.GRPCGenerator{BufGenConfig: "tools/buf/clients.gen.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/clients.gen.yaml").Return(true, nil),
		workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return([]byte(
			"module example.test/example\n\ntool github.com/bufbuild/buf/cmd/buf\n",
		), nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestCheckerChecksEveryDistinctGRPCConfigInSortedOrder(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Sources: map[string]project.Source{
				"contracts": {Type: project.SourceGit, Repo: "example/contracts", Ref: "v1"},
			},
			Components: project.Components{GRPC: &project.GRPC{Clients: []project.GRPCClient{
				{Name: "zeta", Source: "contracts", Path: "proto/zeta.proto", BufGenConfig: "tools/buf/zeta.gen.yaml"},
				{Name: "alpha", Source: "contracts", Path: "proto/alpha.proto", BufGenConfig: "tools/buf/alpha.gen.yaml"},
				{Name: "alpha-v2", Source: "contracts", Path: "proto/alpha-v2.proto", BufGenConfig: "tools/buf/alpha.gen.yaml"},
			}}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				GRPC: &project.GRPCGenerator{BufGenConfig: "tools/buf/default.gen.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(false, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/alpha.gen.yaml").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/zeta.gen.yaml").Return(false, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{
		{Code: project.IssueGoModMissing, Path: selected.ManifestPath, Field: "go.mod"},
		{Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: "tools/buf/zeta.gen.yaml"},
	}, issues)
}

func TestCheckerDoesNotTreatGoModuleCommentAsToolDirective(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Components: project.Components{GRPC: &project.GRPC{Server: &project.GRPCServer{}}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				GRPC: &project.GRPCGenerator{BufGenConfig: "tools/buf/grpc.gen.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "buf.yaml").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/grpc.gen.yaml").Return(true, nil),
		workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return([]byte(
			"module example.test/example\n\n// tool github.com/bufbuild/buf/cmd/buf\n",
		), nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{{
		Code: project.IssueToolMissing, Path: selected.ManifestPath, Field: "go.mod",
	}}, issues)
}

func TestCheckerChecksKafkaProtoConfigAndBufTool(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Sources: map[string]project.Source{
				"contracts": {Type: project.SourceGit, Repo: "example/contracts", Ref: "v1"},
			},
			Components: project.Components{Kafka: &project.Kafka{Consumers: []project.KafkaConsumer{{
				Name: "billing", Contract: project.KafkaContract{
					Source: "contracts", Path: "proto/billing.proto", Format: "proto", ProtoRoot: "proto",
				},
			}}}},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				Kafka: &project.KafkaGenerator{BufGenConfig: "tools/buf/kafka.gen.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/kafka.gen.yaml").Return(false, nil),
		workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return([]byte(
			"module example.test/example\n\ntool github.com/bufbuild/buf/cmd/buf\n",
		), nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{{
		Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: "tools/buf/kafka.gen.yaml",
	}}, issues)
}

func TestCheckerChecksSharedBufToolOnceForGRPCAndKafka(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Sources: map[string]project.Source{
				"contracts": {Type: project.SourceGit, Repo: "example/contracts", Ref: "v1"},
			},
			Components: project.Components{
				GRPC: &project.GRPC{Server: &project.GRPCServer{BufConfig: "buf.yaml"}},
				Kafka: &project.Kafka{Producers: []project.KafkaProducer{{Contract: project.KafkaContract{
					Source: "contracts", Path: "proto/event.proto", Format: "proto", ProtoRoot: "proto",
				}}}},
			},
			Languages: project.Languages{Go: project.GoLanguage{Generators: project.GoGenerators{
				GRPC:  &project.GRPCGenerator{BufGenConfig: "tools/buf/grpc.gen.yaml"},
				Kafka: &project.KafkaGenerator{BufGenConfig: "tools/buf/kafka.gen.yaml"},
			}}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "buf.yaml").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/grpc.gen.yaml").Return(true, nil),
		workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, "go.mod").Return([]byte(
			"module example.test/example\n\ntool github.com/bufbuild/buf/cmd/buf\n",
		), nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "tools/buf/kafka.gen.yaml").Return(true, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestCheckerRequiresMiseConfigForKafkaJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := kafkaJSONProject()
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, ".mise.toml").Return(false, nil),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.NoError(t, err)
	require.Equal(t, []project.Issue{{
		Code: project.IssueToolConfigMissing, Path: selected.ManifestPath, Field: ".mise.toml",
	}}, issues)
}

func TestCheckerValidatesKafkaJSONMiseToolDeclarations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected []project.Issue
	}{
		{name: "valid", content: "[tools]\nnode = \"24\"\n\"npm:quicktype\" = \"26.0.0\"\n"},
		{name: "missing", content: "[tools]\ngo = \"1.26\"\n", expected: []project.Issue{
			{Code: project.IssueToolMissing, Path: "/project/devctl.yaml", Field: ".mise.toml", Parameters: &project.Parameters{Value: "node"}},
			{Code: project.IssueToolMissing, Path: "/project/devctl.yaml", Field: ".mise.toml", Parameters: &project.Parameters{Value: "npm:quicktype"}},
		}},
		{name: "invalid", content: "[tools\n", expected: []project.Issue{{
			Code: project.IssueToolConfigInvalid, Path: "/project/devctl.yaml", Field: ".mise.toml",
		}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			workspace := mocks.NewMockWorkspace(ctrl)
			selected := kafkaJSONProject()
			gomock.InOrder(
				workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
				workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, ".mise.toml").Return(true, nil),
				workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, ".mise.toml").Return([]byte(test.content), nil),
			)

			issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

			require.NoError(t, err)
			require.Equal(t, test.expected, issues)
		})
	}
}

func TestCheckerPreservesMiseConfigReadError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	workspace := mocks.NewMockWorkspace(ctrl)
	selected := kafkaJSONProject()
	cause := errors.New("mise read failed")
	gomock.InOrder(
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, "go.mod").Return(true, nil),
		workspace.EXPECT().RegularFile(gomock.Any(), selected.Root, ".mise.toml").Return(true, nil),
		workspace.EXPECT().ReadBytes(gomock.Any(), selected.Root, ".mise.toml").Return(nil, cause),
	)

	issues, err := projectreadiness.New(workspace).Check(context.Background(), selected)

	require.Empty(t, issues)
	require.ErrorIs(t, err, cause)
	var operationErr *project.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, project.OperationReadFile, operationErr.Operation)
	require.Equal(t, ".mise.toml", operationErr.Path)
}

func kafkaJSONProject() project.Project {
	return project.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: project.Manifest{
			Sources: map[string]project.Source{
				"contracts": {Type: project.SourceGit, Repo: "example/contracts", Ref: "v1"},
			},
			Components: project.Components{Kafka: &project.Kafka{Consumers: []project.KafkaConsumer{{
				Name: "billing", Contract: project.KafkaContract{
					Source: "contracts", Path: "json/billing.json", Format: "json",
				},
			}}}},
		},
	}
}
