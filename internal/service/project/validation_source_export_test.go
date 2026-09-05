package project_test

import (
	"context"
	"fmt"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestServiceLoadProjectRejectsPathOnDevctlSource(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Sources: map[string]projectdomain.Source{
				"contracts": {Type: projectdomain.SourceDevctl, Path: "contracts", Repo: "example/contracts", Ref: "v1"},
			},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	_, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "devctl.yaml")

	var invalidManifest *projectdomain.InvalidManifestError
	require.ErrorAs(t, err, &invalidManifest)
	require.Equal(t, []projectdomain.Issue{{
		Code: projectdomain.IssueSourceInvalid, Path: "/project/devctl.yaml", Field: "sources.contracts",
	}}, invalidManifest.Issues)
}

func TestServiceLoadProjectRejectsOpenAPIExportPathMismatch(t *testing.T) {
	t.Parallel()

	manifest := validManifest()
	manifest.Exports = map[string]projectdomain.Export{
		"public-api": {Kind: "openapi", Path: "api/other.yaml"},
	}
	manifest.Components.HTTP = &projectdomain.HTTP{Server: &projectdomain.HTTPServer{OpenAPI: "api/openapi.yaml"}}

	err := loadProjectWithManifest(t, manifest)

	var invalidManifest *projectdomain.InvalidManifestError
	require.ErrorAs(t, err, &invalidManifest)
	require.Equal(t, []projectdomain.Issue{{
		Code: projectdomain.IssueExportInvalid, Path: "/project/devctl.yaml", Field: "exports.public-api",
	}}, invalidManifest.Issues)
}

func TestServiceLoadProjectValidatesExportsAgainstEffectiveSurfaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		exported   projectdomain.Export
		components projectdomain.Components
		valid      bool
	}{
		{
			name: "OpenAPI exact path", exported: projectdomain.Export{Kind: "openapi", Path: "api/openapi.yaml"}, valid: true,
			components: projectdomain.Components{HTTP: &projectdomain.HTTP{Server: &projectdomain.HTTPServer{OpenAPI: "api/openapi.yaml"}}},
		},
		{
			name: "OpenAPI default path", exported: projectdomain.Export{Kind: "openapi", Path: "api/openapi/swagger.yaml"}, valid: true,
			components: projectdomain.Components{HTTP: &projectdomain.HTTP{Server: &projectdomain.HTTPServer{}}},
		},
		{name: "OpenAPI missing server", exported: projectdomain.Export{Kind: "openapi", Path: "api/openapi.yaml"}},
		{
			name: "gRPC exact root", exported: projectdomain.Export{Kind: "grpc", Path: "api/proto/grpc"}, valid: true,
			components: projectdomain.Components{GRPC: &projectdomain.GRPC{Server: &projectdomain.GRPCServer{ProtoRoot: "api/proto/grpc"}}},
		},
		{
			name: "gRPC mismatched root", exported: projectdomain.Export{Kind: "grpc", Path: "api/proto/other"},
			components: projectdomain.Components{GRPC: &projectdomain.GRPC{Server: &projectdomain.GRPCServer{ProtoRoot: "api/proto/grpc"}}},
		},
		{name: "gRPC missing server", exported: projectdomain.Export{Kind: "grpc", Path: "api/proto/grpc"}},
		{
			name: "Kafka existing producer", exported: projectdomain.Export{Kind: "kafka", Producer: "audit"}, valid: true,
			components: projectdomain.Components{Kafka: &projectdomain.Kafka{Producers: []projectdomain.KafkaProducer{{
				Name: "audit", Topic: "audit.events", Contract: projectdomain.KafkaContract{Format: "raw"},
			}}}},
		},
		{name: "Kafka missing producer", exported: projectdomain.Export{Kind: "kafka", Producer: "audit"}},
		{name: "unknown kind", exported: projectdomain.Export{Kind: "graphql", Path: "api/schema.graphql"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := validManifest()
			manifest.Exports = map[string]projectdomain.Export{"public": test.exported}
			manifest.Components = test.components

			err := loadProjectWithManifest(t, manifest)
			if test.valid {
				require.NoError(t, err)
				return
			}
			var invalidManifest *projectdomain.InvalidManifestError
			require.ErrorAs(t, err, &invalidManifest)
			require.Equal(t, []projectdomain.Issue{{
				Code: projectdomain.IssueExportInvalid, Path: "/project/devctl.yaml", Field: "exports.public",
			}}, invalidManifest.Issues)
		})
	}
}

func TestServiceLoadProjectOrdersExportIssuesByName(t *testing.T) {
	t.Parallel()

	manifest := validManifest()
	manifest.Exports = map[string]projectdomain.Export{
		"zulu":  {Kind: "openapi", Path: "api/zulu.yaml"},
		"alpha": {Kind: "grpc", Path: "api/proto/alpha"},
	}

	err := loadProjectWithManifest(t, manifest)

	var invalidManifest *projectdomain.InvalidManifestError
	require.ErrorAs(t, err, &invalidManifest)
	require.Equal(t, []projectdomain.Issue{
		{Code: projectdomain.IssueExportInvalid, Path: "/project/devctl.yaml", Field: "exports.alpha"},
		{Code: projectdomain.IssueExportInvalid, Path: "/project/devctl.yaml", Field: "exports.zulu"},
	}, invalidManifest.Issues)
}

func loadProjectWithManifest(t *testing.T, manifest projectdomain.Manifest) error {
	t.Helper()
	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{Root: "/project", ManifestPath: "/project/devctl.yaml", Manifest: manifest}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	_, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "devctl.yaml")
	if err != nil {
		return fmt.Errorf("service.LoadProject: %w", err)
	}
	return nil
}

func validManifest() projectdomain.Manifest {
	return projectdomain.Manifest{
		Version:   1,
		Project:   projectdomain.Identity{Name: "example", Language: "go"},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
	}
}
