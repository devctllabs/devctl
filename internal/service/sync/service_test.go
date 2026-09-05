package sync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	syncdomain "github.com/devctllabs/devctl/internal/domain/sync"
	syncservice "github.com/devctllabs/devctl/internal/service/sync"
	"github.com/devctllabs/devctl/internal/service/sync/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestServiceSyncOwnsTargetSelectionAndOutcome(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{
		Root:         "/project",
		ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Paths: projectdomain.ManifestPaths{ExternalContracts: "api/external"},
			Sources: map[string]projectdomain.Source{
				"alpha":   {Type: "url", URL: "https://example.test/alpha.yaml"},
				"bravo":   {Type: "git", Repo: "example/repo", Ref: "main"},
				"charlie": {Type: "url", URL: "https://example.test/charlie.yaml"},
				"local":   {Type: "local", Path: "api/local.yaml"},
			},
			Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{
				{Name: "local", Source: "local", Path: "openapi.yaml"},
				{Name: "bravo", Source: "bravo", Path: "openapi.yaml"},
				{Name: "alpha", Source: "alpha", Path: "openapi.yaml"},
				{Name: "charlie", Source: "charlie", Path: "openapi.yaml"},
			}}},
		},
	}
	projects.EXPECT().LoadProject(gomock.Any(), "custom.yaml").Return(project, nil)
	gomock.InOrder(
		sources.EXPECT().Materialize(gomock.Any(), project.Root, projectdomain.Source{Type: projectdomain.SourceURL, URL: "https://example.test/alpha.yaml"}, contract.Reference{Entrypoint: "openapi.yaml"}).Return(snapshot(t, "alpha"), nil),
		workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "api/external/http/client/alpha", artifact.Tree{Files: []artifact.File{{Path: "openapi.yaml", Content: []byte("alpha")}}}).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil),
		sources.EXPECT().Materialize(gomock.Any(), project.Root, projectdomain.Source{Type: projectdomain.SourceGit, Repo: "example/repo", Ref: "main"}, contract.Reference{Entrypoint: "openapi.yaml"}).Return(snapshot(t, "bravo"), nil),
		workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "api/external/http/client/bravo", artifact.Tree{Files: []artifact.File{{Path: "openapi.yaml", Content: []byte("bravo")}}}).Return(artifact.PublishResult{Action: artifact.PublishUnchanged}, nil),
		sources.EXPECT().Materialize(gomock.Any(), project.Root, projectdomain.Source{Type: projectdomain.SourceURL, URL: "https://example.test/charlie.yaml"}, contract.Reference{Entrypoint: "openapi.yaml"}).Return(snapshot(t, "charlie"), nil),
		workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "api/external/http/client/charlie", artifact.Tree{Files: []artifact.File{{Path: "openapi.yaml", Content: []byte("charlie")}}}).Return(artifact.PublishResult{Action: artifact.PublishUpdated}, nil),
	)
	gomock.InOrder(
		workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/http/client", []string{"alpha", "bravo", "charlie"}).Return([]string{"stale"}, nil),
		workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/grpc/client", []string{}).Return(nil, nil),
		workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/kafka/consumer", []string{}).Return(nil, nil),
		workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/kafka/producer", []string{}).Return(nil, nil),
	)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	result, err := service.Sync(context.Background(), syncdomain.Command{ManifestPath: "custom.yaml"})

	require.NoError(t, err)
	require.Equal(t, []string{"http-client:alpha", "http-client:bravo", "http-client:charlie", "http-client:local"}, result.Targets)
	require.Equal(t, []syncdomain.Change{
		{Target: "http-client:alpha", Path: "api/external/http/client/alpha", Action: syncdomain.ChangeCreated},
		{Target: "http-client:bravo", Path: "api/external/http/client/bravo", Action: syncdomain.ChangeUnchanged},
		{Target: "http-client:charlie", Path: "api/external/http/client/charlie", Action: syncdomain.ChangeUpdated},
		{Target: "http-client:stale", Path: "api/external/http/client/stale", Action: syncdomain.ChangeRemoved},
	}, result.Changes)
}

func TestServiceSyncReturnsAppliedChangesWithLateFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{
			"alpha": {Type: "url", URL: "https://example.test/alpha.yaml"},
			"bravo": {Type: "url", URL: "https://example.test/bravo.yaml"},
		},
		Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{
			{Name: "bravo", Source: "bravo", Path: "openapi.yaml"},
			{Name: "alpha", Source: "alpha", Path: "openapi.yaml"},
		}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	gomock.InOrder(
		sources.EXPECT().Materialize(gomock.Any(), project.Root, projectdomain.Source{Type: projectdomain.SourceURL, URL: "https://example.test/alpha.yaml"}, contract.Reference{Entrypoint: "openapi.yaml"}).Return(snapshot(t, "alpha"), nil),
		workspace.EXPECT().PublishDirectory(gomock.Any(), "/project", "api/external/http/client/alpha", gomock.Any()).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil),
		sources.EXPECT().Materialize(gomock.Any(), project.Root, projectdomain.Source{Type: projectdomain.SourceURL, URL: "https://example.test/bravo.yaml"}, contract.Reference{Entrypoint: "openapi.yaml"}).Return(contract.Snapshot{}, errors.New("upstream failed")),
	)

	result, err := syncservice.New(zap.NewNop(), projects, sources, workspace).Sync(context.Background(), syncdomain.Command{ManifestPath: "devctl.yaml"})

	require.Equal(t, failure.Unavailable, failure.CategoryOf(err))
	require.Equal(t, []string{"http-client:alpha"}, result.Targets)
	require.Equal(t, []syncdomain.Change{{
		Target: "http-client:alpha", Path: "api/external/http/client/alpha", Action: syncdomain.ChangeCreated,
	}}, result.Changes)
}

func TestServiceSyncPreservesStaleSnapshotDiagnosticDuringReexport(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	source := projectdomain.Source{Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"}
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"upstream": source},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "events", Topic: "sample.events.created.v1", Contract: projectdomain.KafkaContract{
				Source: "upstream", Export: "events", Format: "json",
			},
		}}}},
	}}
	reference := contract.Reference{Export: "events", Topic: "sample.events.created.v1", Format: "json"}
	metadataErr := &contract.SnapshotMetadataError{
		Field: "entrypoint", Reason: contract.MetadataRequired, Hint: "devctl sync",
	}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	sources.EXPECT().Materialize(gomock.Any(), project.Root, source, reference).Return(contract.Snapshot{}, metadataErr)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	_, err := service.Sync(context.Background(), syncdomain.Command{
		ManifestPath: "devctl.yaml", Target: "kafka-consumer:events",
	})

	require.ErrorIs(t, err, metadataErr)
	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestServiceSyncPreservesMaterializationFailureCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cause    error
		category failure.Category
	}{
		{name: "invalid input", cause: &materializedomain.OperationError{Kind: materializedomain.FailureInvalid}, category: failure.InvalidInput},
		{name: "not found", cause: &materializedomain.OperationError{Kind: materializedomain.FailureNotFound}, category: failure.NotFound},
		{name: "unsupported", cause: &materializedomain.OperationError{Kind: materializedomain.FailureUnsupported}, category: failure.Unsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects := mocks.NewMockProjectRepository(ctrl)
			sources := mocks.NewMockMaterializer(ctrl)
			workspace := mocks.NewMockWorkspaceRepository(ctrl)
			source := projectdomain.Source{Type: projectdomain.SourceURL, URL: "https://example.test/openapi.yaml"}
			project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
				Sources: map[string]projectdomain.Source{"upstream": source},
				Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{{
					Name: "upstream", Source: "upstream", Path: "openapi.yaml",
				}}}},
			}}
			projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
			sources.EXPECT().Materialize(
				gomock.Any(), project.Root, source, contract.Reference{Entrypoint: "openapi.yaml"},
			).Return(contract.Snapshot{}, test.cause)
			service := syncservice.New(zap.NewNop(), projects, sources, workspace)

			_, err := service.Sync(context.Background(), syncdomain.Command{
				ManifestPath: "devctl.yaml", Target: "http-client:upstream",
			})

			require.Equal(t, test.category, failure.CategoryOf(err))
			var operationErr *syncdomain.OperationError
			require.ErrorAs(t, err, &operationErr)
			require.Equal(t, "http-client:upstream", operationErr.Target)
			require.Equal(t, "upstream", operationErr.Source)
			require.Equal(t, "openapi.yaml", operationErr.Path)
		})
	}
}

func TestServiceSyncCarriesProtoRootIntoMaterialization(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	source := projectdomain.Source{Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1", Proto: projectdomain.SourceProto{BufConfig: "buf.yaml"}}
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Paths:   projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{"contracts": source},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
			Name: "billing", Source: "contracts", Path: "proto/acme/billing/v1/service.proto", ProtoRoot: "proto",
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	sources.EXPECT().Materialize(gomock.Any(), project.Root, source, contract.Reference{
		Entrypoint: "proto/acme/billing/v1/service.proto", Format: "proto", ProtoRoot: "proto",
	}).Return(contract.Snapshot{Entrypoint: "proto/acme/billing/v1/service.proto", Files: []contract.File{{
		Path: "proto/acme/billing/v1/service.proto", Content: []byte("syntax = \"proto3\";\n"),
	}}}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "api/external/grpc/client/billing", gomock.Any()).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil)
	workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/grpc/client", []string{"billing"}).Return(nil, nil)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	result, err := service.Sync(context.Background(), syncdomain.Command{ManifestPath: "devctl.yaml", Family: "grpc"})

	require.NoError(t, err)
	require.Equal(t, []string{"grpc-client:billing"}, result.Targets)
}

func TestServiceSyncPublishesDevctlGRPCMetadataAtTargetRoot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	source := projectdomain.Source{Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"}
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"contracts": source},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
			Name: "billing", Source: "contracts", Export: "billing",
		}}}},
	}}
	reference := contract.Reference{Export: "billing", Format: "proto"}
	snapshot := contract.Snapshot{
		ModuleRoot: "api/proto/grpc",
		Files: []contract.File{
			{Path: "api/proto/grpc/service.proto", Content: []byte("syntax = \"proto3\";\n")},
			{Path: "buf.yaml", Content: []byte("version: v2\n")},
		},
		Metadata: &contract.Metadata{
			Kind: "grpc", Format: "proto", ModuleRoot: "api/proto/grpc", BufConfig: "buf.yaml",
		},
	}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	sources.EXPECT().Materialize(gomock.Any(), project.Root, source, reference).Return(snapshot, nil)
	workspace.EXPECT().PublishDirectory(
		gomock.Any(), project.Root, "api/external/grpc/client/billing", artifact.Tree{Files: []artifact.File{
			{Path: "api/proto/grpc/service.proto", Content: []byte("syntax = \"proto3\";\n")},
			{Path: "buf.yaml", Content: []byte("version: v2\n")},
			{Path: ".devctl-contract.json", Content: []byte("{\"kind\":\"grpc\",\"format\":\"proto\",\"module_root\":\"api/proto/grpc\",\"buf_config\":\"buf.yaml\"}\n"), Mode: 0o644},
		}},
	).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	_, err := service.Sync(context.Background(), syncdomain.Command{
		ManifestPath: "devctl.yaml", Target: "grpc-client:billing",
	})

	require.NoError(t, err)
}

func TestServiceSyncPublishesKafkaSidecarAtConsumerAndProducerRoots(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	source := projectdomain.Source{Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"}
	selected := projectdomain.KafkaContract{Source: "contracts", Export: "events", Format: "json"}
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"contracts": source},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{
			Consumers: []projectdomain.KafkaConsumer{{Name: "audit", Topic: "sample.audit.events.v1", Contract: selected}},
			Producers: []projectdomain.KafkaProducer{{Name: "audit", Topic: "sample.audit.events.v1", Contract: selected}},
		}},
	}}
	reference := contract.Reference{Export: "events", Format: "json", Topic: "sample.audit.events.v1"}
	snapshot := contract.Snapshot{
		Entrypoint: "schemas/event.json",
		Files:      []contract.File{{Path: "schemas/event.json", Content: []byte(`{"title":"Event"}`)}},
		Metadata: &contract.Metadata{
			Kind: "kafka", Topic: "sample.audit.events.v1", Format: "json", Entrypoint: "schemas/event.json",
		},
	}
	expectedTree := artifact.Tree{Files: []artifact.File{
		{Path: "schemas/event.json", Content: []byte(`{"title":"Event"}`)},
		{Path: ".devctl-contract.json", Content: []byte("{\"kind\":\"kafka\",\"topic\":\"sample.audit.events.v1\",\"format\":\"json\",\"entrypoint\":\"schemas/event.json\"}\n"), Mode: 0o644},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	sources.EXPECT().Materialize(gomock.Any(), project.Root, source, reference).Return(snapshot, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "api/external/kafka/consumer/audit", expectedTree).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil)
	workspace.EXPECT().PublishDirectory(gomock.Any(), project.Root, "api/external/kafka/producer/audit", expectedTree).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil)
	workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/kafka/consumer", []string{"audit"}).Return(nil, nil)
	workspace.EXPECT().PruneDirectories(gomock.Any(), project.Root, "api/external/kafka/producer", []string{"audit"}).Return(nil, nil)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	_, err := service.Sync(context.Background(), syncdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
}

func TestServiceSyncDryRunDoesNotMaterializeURLSource(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Paths: projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{
			"billing": {Type: projectdomain.SourceURL, URL: "https://example.test/billing/openapi.yaml"},
		},
		Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{{
			Name: "billing", Source: "billing", Path: "spec/openapi.yaml",
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	workspace.EXPECT().PreviewPruneDirectories(
		gomock.Any(), project.Root, "api/external/http/client", []string{"billing"},
	).Return(nil, nil)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	result, err := service.Sync(context.Background(), syncdomain.Command{
		ManifestPath: "devctl.yaml", Family: "http", DryRun: true,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"http-client:billing"}, result.Targets)
	require.Equal(t, []syncdomain.Change{{
		Target: "http-client:billing", Path: "api/external/http/client/billing", Action: syncdomain.ChangePlannedPublish,
	}}, result.Changes)
}

func TestServiceSyncAppliesCatalogSelectionContract(t *testing.T) {
	t.Parallel()

	manifest := projectdomain.Manifest{
		Project: projectdomain.Identity{Language: "go"},
		Sources: map[string]projectdomain.Source{
			"local": {Type: projectdomain.SourceLocal, Path: "api/contracts"},
		},
		Components: projectdomain.Components{
			HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{{
				Name: "local", Source: "local", Path: "openapi.yaml",
			}}},
			Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
				Name: "raw", Topic: "sample.events.raw.v1", Contract: projectdomain.KafkaContract{Format: "raw"},
			}}},
		},
	}
	tests := []struct {
		name     string
		command  syncdomain.Command
		targets  []string
		category failure.Category
	}{
		{name: "known empty family", command: syncdomain.Command{Family: "grpc", DryRun: true}, targets: []string{}},
		{name: "local sync no-op", command: syncdomain.Command{Target: "http-client:local", DryRun: true}, targets: []string{"http-client:local"}},
		{name: "unknown family", command: syncdomain.Command{Family: "other", DryRun: true}, category: failure.InvalidInput},
		{name: "unknown target", command: syncdomain.Command{Target: "grpc-client:missing", DryRun: true}, category: failure.NotFound},
		{name: "config does not sync", command: syncdomain.Command{Target: "config", DryRun: true}, category: failure.Unsupported},
		{name: "raw Kafka does not sync", command: syncdomain.Command{Target: "kafka-consumer:raw", DryRun: true}, category: failure.Unsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects := mocks.NewMockProjectRepository(ctrl)
			workspace := mocks.NewMockWorkspaceRepository(ctrl)
			projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(projectdomain.Project{Root: "/project", Manifest: manifest}, nil)
			if test.name == "known empty family" {
				workspace.EXPECT().PreviewPruneDirectories(
					gomock.Any(), "/project", "api/external/grpc/client", []string{},
				).Return(nil, nil)
			}
			service := syncservice.New(
				zap.NewNop(), projects, mocks.NewMockMaterializer(ctrl), workspace,
			)
			command := test.command
			command.ManifestPath = "devctl.yaml"

			result, err := service.Sync(context.Background(), command)

			if test.category != "" {
				require.Equal(t, test.category, failure.CategoryOf(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.targets, result.Targets)
			require.Empty(t, result.Changes)
		})
	}
}

func TestServiceSyncDryRunReportsPlannedStaleRemovalWithoutMutation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	sources := mocks.NewMockMaterializer(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	workspace.EXPECT().PreviewPruneDirectories(
		gomock.Any(), project.Root, "api/external/http/client", []string{},
	).Return([]string{"stale"}, nil)
	service := syncservice.New(zap.NewNop(), projects, sources, workspace)

	result, err := service.Sync(context.Background(), syncdomain.Command{
		ManifestPath: "devctl.yaml", Family: "http", DryRun: true,
	})

	require.NoError(t, err)
	require.Empty(t, result.Targets)
	require.Equal(t, []syncdomain.Change{{
		Target: "http-client:stale", Path: "api/external/http/client/stale", Action: syncdomain.ChangePlannedRemove,
	}}, result.Changes)
}

func snapshot(t *testing.T, content string) contract.Snapshot {
	t.Helper()
	return contract.Snapshot{Entrypoint: "openapi.yaml", Files: []contract.File{{Path: "openapi.yaml", Content: []byte(content)}}}
}
