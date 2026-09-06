package project_test

import (
	"context"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestServiceLoadProjectRejectsInvalidGRPCClients(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sources map[string]projectdomain.Source
		clients []projectdomain.GRPCClient
		field   string
	}{
		{
			name:    "invalid name",
			sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"}},
			clients: []projectdomain.GRPCClient{{Name: "Billing", Source: "contracts", Path: "proto/billing.proto"}},
			field:   "components.grpc.clients.Billing",
		},
		{
			name:    "duplicate name",
			sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"}},
			clients: []projectdomain.GRPCClient{
				{Name: "billing", Source: "contracts", Path: "proto/billing.proto"},
				{Name: "billing", Source: "contracts", Path: "proto/billing-v2.proto"},
			},
			field: "components.grpc.clients.billing",
		},
		{
			name:    "devctl source without export",
			sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"}},
			clients: []projectdomain.GRPCClient{{Name: "billing", Source: "contracts"}},
			field:   "components.grpc.clients.billing",
		},
		{
			name:    "devctl source with path",
			sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"}},
			clients: []projectdomain.GRPCClient{{Name: "billing", Source: "contracts", Export: "billing", Path: "proto/billing.proto"}},
			field:   "components.grpc.clients.billing",
		},
		{
			name:    "git source without path",
			sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"}},
			clients: []projectdomain.GRPCClient{{Name: "billing", Source: "contracts"}},
			field:   "components.grpc.clients.billing",
		},
		{
			name:    "git source with export",
			sources: map[string]projectdomain.Source{"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"}},
			clients: []projectdomain.GRPCClient{{Name: "billing", Source: "contracts", Export: "billing", Path: "proto/billing.proto"}},
			field:   "components.grpc.clients.billing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			manifests := mocks.NewMockManifestRepository(ctrl)
			workspace := mocks.NewMockManifestLocator(ctrl)
			selected := projectdomain.Project{
				Root: "/project", ManifestPath: "/project/devctl.yaml",
				Manifest: projectdomain.Manifest{
					Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
					Sources:    test.sources,
					Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: test.clients}},
					Languages:  projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
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
				Code: projectdomain.IssueGRPCClientInvalid, Path: "/project/devctl.yaml", Field: test.field,
			}}, invalidManifest.Issues)
		})
	}
}

func TestServiceLoadProjectAcceptsValidGRPCClientSelections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source projectdomain.Source
		client projectdomain.GRPCClient
	}{
		{
			name:   "git source path",
			source: projectdomain.Source{Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"},
			client: projectdomain.GRPCClient{Name: "billing", Source: "contracts", Path: "proto/billing.proto"},
		},
		{
			name:   "devctl source export",
			source: projectdomain.Source{Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"},
			client: projectdomain.GRPCClient{Name: "billing", Source: "contracts", Export: "billing"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			manifests := mocks.NewMockManifestRepository(ctrl)
			workspace := mocks.NewMockManifestLocator(ctrl)
			selected := projectdomain.Project{
				Root: "/project", ManifestPath: "/project/devctl.yaml",
				Manifest: projectdomain.Manifest{
					Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
					Sources:    map[string]projectdomain.Source{"contracts": test.source},
					Components: projectdomain.Components{GRPC: &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{test.client}}},
					Languages:  projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
				},
			}
			gomock.InOrder(
				workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
				manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
			)

			loaded, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "devctl.yaml")

			require.NoError(t, err)
			require.Equal(t, selected, loaded)
		})
	}
}

func TestServiceLoadProjectRejectsUnsafeBufPathsAtTheirManifestFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*projectdomain.Manifest)
		field     string
	}{
		{
			name: "traversing server config",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Components.GRPC.Server.BufConfig = "../buf.yaml"
			},
			field: "components.grpc.server.buf_config",
		},
		{
			name: "absolute client config",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Components.GRPC.Clients[0].BufGenConfig = "/tmp/client.gen.yaml"
			},
			field: "components.grpc.clients.billing.buf_gen_config",
		},
		{
			name: "traversing shared generator config",
			configure: func(manifest *projectdomain.Manifest) {
				manifest.Languages.Go.Generators.GRPC.BufGenConfig = "../grpc.gen.yaml"
			},
			field: "languages.go.generators.grpc.buf_gen_config",
		},
		{
			name: "absolute source config",
			configure: func(manifest *projectdomain.Manifest) {
				source := manifest.Sources["contracts"]
				source.Proto.BufConfig = "/tmp/buf.yaml"
				manifest.Sources["contracts"] = source
			},
			field: "sources.contracts.proto.buf_config",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			manifests := mocks.NewMockManifestRepository(ctrl)
			workspace := mocks.NewMockManifestLocator(ctrl)
			manifest := projectdomain.Manifest{
				Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
				Sources: map[string]projectdomain.Source{
					"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"},
				},
				Components: projectdomain.Components{GRPC: &projectdomain.GRPC{
					Server: &projectdomain.GRPCServer{BufConfig: "buf.yaml"},
					Clients: []projectdomain.GRPCClient{{
						Name: "billing", Source: "contracts", Path: "proto/billing.proto",
					}},
				}},
				Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{
					Module: "example.test/example", Generators: projectdomain.GoGenerators{
						GRPC: &projectdomain.GRPCGenerator{BufGenConfig: "tools/buf/grpc.gen.yaml"},
					},
				}},
			}
			test.configure(&manifest)
			selected := projectdomain.Project{Root: "/project", ManifestPath: "/project/devctl.yaml", Manifest: manifest}
			gomock.InOrder(
				workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
				manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
			)

			_, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "devctl.yaml")

			var invalidManifest *projectdomain.InvalidManifestError
			require.ErrorAs(t, err, &invalidManifest)
			require.Contains(t, invalidManifest.Issues, projectdomain.Issue{
				Code: projectdomain.IssuePathInvalid, Path: "/project/devctl.yaml", Field: test.field,
			})
		})
	}
}
