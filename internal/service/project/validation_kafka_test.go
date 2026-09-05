package project_test

import (
	"context"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceValidateRejectsKafkaContractWithUnknownSource(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
				Name: "billing", Topic: "billing_service.invoice.events.v1",
				Contract: projectdomain.KafkaContract{Source: "missing", Path: "invoice.proto", Format: "proto"},
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
		Field: "components.kafka.consumers.billing.contract.source",
	}}}, result)
}

func TestServiceValidateRejectsNonRawKafkaContractWithoutSource(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{
				Name: "billing", Topic: "billing_service.invoice.events.v1",
				Contract: projectdomain.KafkaContract{Path: "invoice.json", Format: "json"},
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
		Code: projectdomain.IssueKafkaContractInvalid, Path: "/project/devctl.yaml",
		Field: "components.kafka.consumers.billing.contract",
	}}}, result)
}
