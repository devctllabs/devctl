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

func TestServiceLoadProjectDoesNotResolveDevctlHTTPExportLocally(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Sources: map[string]projectdomain.Source{
				"contracts": {Type: projectdomain.SourceDevctl, Repo: "example/contracts", Ref: "v1"},
			},
			Components: projectdomain.Components{HTTP: &projectdomain.HTTP{Clients: []projectdomain.HTTPClient{{
				Name: "billing", Source: "contracts", Export: "billing",
			}}}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	loaded, err := project.New(zap.NewNop(), project.Dependencies{Manifests: manifests, Locator: workspace}).LoadProject(context.Background(), "devctl.yaml")

	require.NoError(t, err)
	require.Equal(t, selected, loaded)
}
