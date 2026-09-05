package lint_test

import (
	"context"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	lintdomain "github.com/devctllabs/devctl/internal/domain/lint"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	"github.com/devctllabs/devctl/internal/service/lint/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestLintServiceOwnsContractCatalogAndOutcome(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Paths:   projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{"remote": {Type: "url"}},
		Components: projectdomain.Components{HTTP: &projectdomain.HTTP{
			Server: &projectdomain.HTTPServer{OpenAPI: "api/openapi/swagger.yaml"},
			Clients: []projectdomain.HTTPClient{
				{Name: "remote", Source: "remote", Path: "openapi.yaml"},
			},
		}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "custom.yaml").Return(project, nil)
	serverContract := "/project/api/openapi/swagger.yaml"
	remoteContract := "/project/api/external/http/client/remote/openapi.yaml"
	targets := projectdomain.NewTargetCatalog(project.Manifest).Select(projectdomain.TargetOperationLint, "http", "")
	remoteTarget, serverTarget := targets[0], targets[1]
	resolvedRemote, resolvedServer := remoteTarget, serverTarget
	resolvedRemote.Input, resolvedServer.Input = remoteContract, serverContract
	gomock.InOrder(
		inputs.EXPECT().Resolve(gomock.Any(), project, remoteTarget).Return(resolvedRemote, nil),
		contracts.EXPECT().ReadContract(gomock.Any(), remoteContract).Return([]byte(`openapi: 3.1.0
info: {title: Remote, version: 1.0.0}
paths:
  /first:
    get: {operationId: duplicate, responses: {"200": {description: ok}}}
  /second:
    get: {operationId: duplicate, responses: {"200": {description: ok}}}
`), nil),
		inputs.EXPECT().Resolve(gomock.Any(), project, serverTarget).Return(resolvedServer, nil),
		contracts.EXPECT().ReadContract(gomock.Any(), serverContract).Return([]byte(`openapi: 3.1.0
info: {title: Server, version: 1.0.0}
paths:
  /health:
    get: {operationId: health, responses: {"200": {description: ok}}}
`), nil),
	)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: inputs,
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "custom.yaml", Family: "http"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []string{"http-client:remote", "http-server"}, result.Contracts)
	require.Len(t, result.Issues, 1)
	require.Equal(t, "operation_id_duplicate", result.Issues[0].Code)
	require.Equal(t, "http-client:remote", result.Issues[0].Target)
}

func TestLintServicePreservesHTTPInputFailureSemantics(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	cause := errors.New("entrypoint unavailable")
	selected := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Server: &projectdomain.HTTPServer{
			OpenAPI: "api/openapi/swagger.yaml",
		}}},
	}}
	target := projectdomain.NewTargetCatalog(selected.Manifest).Select(projectdomain.TargetOperationLint, "http", "")[0]
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(selected, nil)
	inputs.EXPECT().Resolve(gomock.Any(), selected, target).Return(target, cause)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: inputs,
	})

	_, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "http"})

	var operationErr *lintdomain.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Equal(t, lintdomain.OperationLocateContract, operationErr.Operation)
	require.Equal(t, target.ID, operationErr.Target)
	require.Equal(t, target.Reference.Entrypoint, operationErr.Path)
	require.Equal(t, lintdomain.FailureUnavailable, operationErr.Kind)
	require.ErrorIs(t, err, cause)
}

func TestLintServiceReportsInvalidGRPCProtoFilename(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	proto := mocks.NewMockProtoLinter(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{
			Server: &projectdomain.GRPCServer{ProtoRoot: "api/proto", BufConfig: "buf.yaml"},
		}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	contracts.EXPECT().ListProtoFiles(gomock.Any(), project.Root, "api/proto").Return([]string{
		"api/proto/acme/v1/sample.common_types.proto",
		"api/proto/acme/v1/service.proto",
	}, nil)
	target := projectdomain.NewTargetCatalog(project.Manifest).Select(projectdomain.TargetOperationLint, "grpc", "")[0]
	proto.EXPECT().Lint(gomock.Any(), project, target).Return(nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl), Proto: proto,
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "grpc"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []string{"grpc-server"}, result.Contracts)
	require.Equal(t, []lintdomain.Issue{{
		Code: "proto_filename", Target: "grpc-server", Path: "api/proto/acme/v1/service.proto",
	}}, result.Issues)
}

func TestLintServiceChecksGRPCClientsWithoutServer(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	proto := mocks.NewMockProtoLinter(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"contracts": {Type: "local", Path: "api/contracts"}},
		Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
			Name: "billing", Source: "contracts", Path: "proto/acme/billing/v1", ProtoRoot: "proto",
		}}}},
	}}
	target := projectdomain.NewTargetCatalog(project.Manifest).Select(projectdomain.TargetOperationLint, "grpc", "")[0]
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	contracts.EXPECT().ListProtoFiles(gomock.Any(), project.Root, target.Input).Return([]string{
		"api/contracts/proto/acme/billing/v1/billing_service.invoice_service.proto",
	}, nil)
	proto.EXPECT().Lint(gomock.Any(), project, target).Return(nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl), Proto: proto,
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "grpc"})

	require.NoError(t, err)
	require.True(t, result.Valid)
	require.Equal(t, []string{"grpc-client:billing"}, result.Contracts)
	require.Empty(t, result.Issues)
}

func TestLintServiceReportsInvalidKafkaTopic(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
			Name: "audit", Topic: "audit.events", Contract: projectdomain.KafkaContract{Format: "raw"},
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl),
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []string{"kafka-producer:audit"}, result.Contracts)
	require.Equal(t, []lintdomain.Issue{{
		Code: "kafka_topic", Target: "kafka-producer:audit",
	}}, result.Issues)
}

func TestLintServiceRejectsCommittedKafkaMetadataThatDoesNotMatchTarget(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Paths:   projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{"upstream": {Type: projectdomain.SourceDevctl}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "events", Topic: "downstream_service.domain.events.v1", Contract: projectdomain.KafkaContract{
				Format: "proto", Source: "upstream", Export: "events",
			},
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	metadataErr := &contract.SnapshotMetadataError{
		Field: "topic", Reason: contract.MetadataMismatch, Hint: "devctl sync",
	}
	target := projectdomain.NewTargetCatalog(project.Manifest).Select(projectdomain.TargetOperationLint, "kafka", "")[0]
	inputs.EXPECT().Resolve(gomock.Any(), project, target).Return(target, metadataErr)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: inputs,
	})

	_, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.ErrorIs(t, err, metadataErr)
	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
}

func TestLintServiceReportsMismatchedLocalKafkaSchemaFilename(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"events": {Type: "local", Path: "api/events"}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
			Name: "audit", Topic: "audit_service.audit.created.v1",
			Contract: projectdomain.KafkaContract{Source: "events", Format: "json", Path: "wrong_name.json"},
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	contracts.EXPECT().ResolveContract(gomock.Any(), contract.Location{Root: "/project", RelativePath: "api/events", Entrypoint: "wrong_name.json", Local: true}).Return("/project/api/events/wrong_name.json", nil)
	contracts.EXPECT().ReadContract(gomock.Any(), "/project/api/events/wrong_name.json").Return([]byte(`{"title":"AuditEvent","type":"object"}`), nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl),
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []string{"kafka-producer:audit"}, result.Contracts)
	require.Equal(t, []lintdomain.Issue{{
		Code: "kafka_schema_filename", Target: "kafka-producer:audit", Path: "wrong_name.json",
	}}, result.Issues)
}

func TestLintServiceCompilesKafkaJSONSchema(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"events": {Type: "local", Path: "api/contracts"}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "audit", Topic: "audit_service.audit.created.v1",
			Contract: projectdomain.KafkaContract{Format: "json", Source: "events", Path: "schemas/audit_service.audit.created.v1.json"},
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	location := contract.Location{Root: project.Root, RelativePath: "api/contracts", Entrypoint: "schemas/audit_service.audit.created.v1.json", Local: true}
	contracts.EXPECT().ResolveContract(gomock.Any(), location).Return("/project/api/contracts/schemas/audit_service.audit.created.v1.json", nil)
	contracts.EXPECT().ReadContract(gomock.Any(), "/project/api/contracts/schemas/audit_service.audit.created.v1.json").Return([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":42}`), nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl),
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []lintdomain.Issue{{
		Code: "json_schema", Target: "kafka-consumer:audit", Path: "/project/api/contracts/schemas/audit_service.audit.created.v1.json",
	}}, result.Issues)
}

func TestLintServiceResolvesDevctlKafkaJSONFromCommittedMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	inputs := mocks.NewMockTargetResolver(ctrl)
	const topic = "audit_service.audit.created.v1"
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"events": {Type: projectdomain.SourceDevctl}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "audit", Topic: topic,
			Contract: projectdomain.KafkaContract{Format: "json", Source: "events", Export: "audit"},
		}}}},
	}}
	target := projectdomain.NewTargetCatalog(project.Manifest).Select(projectdomain.TargetOperationLint, "kafka", "")[0]
	snapshot := contract.Snapshot{
		Entrypoint: "schemas/event.json",
		Metadata: &contract.Metadata{
			Kind: "kafka", Topic: topic, Format: "json", Entrypoint: "schemas/event.json",
		},
	}
	resolvedInput := target.WithSnapshot(snapshot)
	resolved := resolvedInput
	resolved.Location.Root = project.Root
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	inputs.EXPECT().Resolve(gomock.Any(), project, target).Return(resolvedInput, nil)
	contracts.EXPECT().ResolveContract(gomock.Any(), resolved.Location).Return(
		"/project/api/external/kafka/consumer/audit/schemas/event.json", nil,
	)
	contracts.EXPECT().ReadContract(
		gomock.Any(), "/project/api/external/kafka/consumer/audit/schemas/event.json",
	).Return([]byte(`{"title":"Event","type":"object"}`), nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: inputs,
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
	require.True(t, result.Valid)
}

func TestLintServiceRequiresKafkaJSONSchemaTitle(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Sources: map[string]projectdomain.Source{"events": {Type: "local", Path: "api/contracts"}},
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "audit", Topic: "audit_service.audit.created.v1",
			Contract: projectdomain.KafkaContract{Format: "json", Source: "events", Path: "schemas/audit_service.audit.created.v1.json"},
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	location := contract.Location{Root: project.Root, RelativePath: "api/contracts", Entrypoint: "schemas/audit_service.audit.created.v1.json", Local: true}
	contracts.EXPECT().ResolveContract(gomock.Any(), location).Return("/project/api/contracts/schemas/audit_service.audit.created.v1.json", nil)
	contracts.EXPECT().ReadContract(gomock.Any(), "/project/api/contracts/schemas/audit_service.audit.created.v1.json").Return([]byte(`{"type":"object"}`), nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl),
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: "kafka"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []lintdomain.Issue{{
		Code: "json_schema_title", Target: "kafka-consumer:audit",
		Path: "/project/api/contracts/schemas/audit_service.audit.created.v1.json", Field: "title",
	}}, result.Issues)
}

func TestLintServiceWithoutFamilyIncludesKafkaContracts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	contracts := mocks.NewMockContractLocator(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
			Name: "audit", Topic: "audit.events", Contract: projectdomain.KafkaContract{Format: "raw"},
		}}}},
	}}
	projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(project, nil)
	service := lintservice.New(zap.NewNop(), lintservice.Dependencies{
		Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl),
	})

	result, err := service.Lint(context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml"})

	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Equal(t, []string{"kafka-consumer:audit"}, result.Contracts)
	require.Equal(t, []lintdomain.Issue{{Code: "kafka_topic", Target: "kafka-consumer:audit"}}, result.Issues)
}

func TestLintServiceAppliesCatalogFamilySelectionContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		family   string
		category failure.Category
	}{
		{name: "known empty family", family: "grpc"},
		{name: "unknown family", family: "other", category: failure.InvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects := mocks.NewMockProjectRepository(ctrl)
			contracts := mocks.NewMockContractLocator(ctrl)
			projects.EXPECT().LoadProject(gomock.Any(), "devctl.yaml").Return(projectdomain.Project{
				Root: "/project", Manifest: projectdomain.Manifest{Project: projectdomain.Identity{Language: "go"}},
			}, nil)

			result, err := lintservice.New(zap.NewNop(), lintservice.Dependencies{
				Projects: projects, Contracts: contracts, Inputs: passthroughTargetResolver(ctrl),
			}).Lint(
				context.Background(), lintdomain.Command{ManifestPath: "devctl.yaml", Family: test.family},
			)

			if test.category != "" {
				require.Equal(t, test.category, failure.CategoryOf(err))
				return
			}
			require.NoError(t, err)
			require.True(t, result.Valid)
			require.Empty(t, result.Contracts)
		})
	}
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
