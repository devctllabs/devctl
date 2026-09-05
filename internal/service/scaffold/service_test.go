package scaffold_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	scaffolddomain "github.com/devctllabs/devctl/internal/domain/scaffold"
	"github.com/devctllabs/devctl/internal/service/scaffold"
	"github.com/devctllabs/devctl/internal/service/scaffold/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestServiceOwnsScaffoldWorkflowAndOutcome(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Project:   projectdomain.Identity{Name: "sample", Language: "go"},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/sample"}},
	}}
	gomock.InOrder(
		projects.EXPECT().LoadProject(gomock.Any(), "custom.yaml").Return(project, nil),
		workspace.EXPECT().Walk(gomock.Any(), project.Root, gomock.Any()).Return(nil),
	)
	workspace.EXPECT().Lstat(gomock.Any(), project.Root, gomock.Any()).Return(nil, fs.ErrNotExist).AnyTimes()
	workspace.EXPECT().PublishFile(gomock.Any(), project.Root, gomock.Any(), gomock.Any()).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil).AnyTimes()
	service := scaffold.New(zap.NewNop(), projects, workspace)

	result, err := service.Scaffold(context.Background(), scaffolddomain.Command{ManifestPath: "custom.yaml"})

	require.NoError(t, err)
	require.NotEmpty(t, result.Files)
	require.Equal(t, scaffolddomain.FileChange{Path: ".env.example", Action: scaffolddomain.FileCreated}, result.Files[0])
}
