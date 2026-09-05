package scaffold_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	scaffolddomain "github.com/devctllabs/devctl/internal/domain/scaffold"
	"github.com/devctllabs/devctl/internal/service/scaffold"
	"github.com/devctllabs/devctl/internal/service/scaffold/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestServiceRejectsPreflightConflictsBeforePublishing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		entryKind string
		different bool
	}{
		{name: "planned symlink", path: "go.mod", entryKind: "symlink"},
		{name: "planned directory", path: "go.mod", entryKind: "directory"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newPreflightFixture(t, minimalServiceManifest())
			info := scaffoldFileInfo(t, test.path, test.entryKind)
			fixture.expectWalkEntry(test.path, info)
			if test.path == "go.mod" {
				fixture.workspace.EXPECT().Lstat(gomock.Any(), fixture.root, test.path).Return(info, nil)
			}
			if test.different {
				fixture.workspace.EXPECT().ReadBytes(gomock.Any(), fixture.root, test.path).Return([]byte("different"), nil)
			}

			result, err := fixture.service.Scaffold(context.Background(), scaffolddomain.Command{})

			var operationErr *scaffolddomain.OperationError
			require.ErrorAs(t, err, &operationErr)
			require.Equal(t, scaffolddomain.OperationPreflight, operationErr.Operation)
			require.Equal(t, scaffolddomain.FailureConflict, operationErr.Kind)
			require.Equal(t, test.path, operationErr.Path)
			require.Empty(t, result.Files)
		})
	}
}

func TestServiceReportsPreflightFailuresAsUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stage string
	}{
		{name: "cancelled context", stage: "context"},
		{name: "walk failure", stage: "walk"},
		{name: "lstat failure", stage: "lstat"},
		{name: "read failure", stage: "read"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			failure := errors.New(test.stage + " failed")
			fixture := newPreflightFixture(t, minimalServiceManifest())
			ctx := context.Background()
			switch test.stage {
			case "context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				failure = context.Canceled
			case "walk":
				fixture.workspace.EXPECT().Walk(gomock.Any(), fixture.root, gomock.Any()).Return(failure)
			case "lstat", "read":
				info := scaffoldFileInfo(t, "go.mod", "regular")
				fixture.expectWalkEntry("go.mod", info)
				if test.stage == "lstat" {
					fixture.workspace.EXPECT().Lstat(gomock.Any(), fixture.root, "go.mod").Return(nil, failure)
				} else {
					fixture.workspace.EXPECT().Lstat(gomock.Any(), fixture.root, "go.mod").Return(info, nil)
					fixture.workspace.EXPECT().ReadBytes(gomock.Any(), fixture.root, "go.mod").Return(nil, failure)
				}
			}

			result, err := fixture.service.Scaffold(ctx, scaffolddomain.Command{})

			var operationErr *scaffolddomain.OperationError
			require.ErrorAs(t, err, &operationErr)
			require.Equal(t, scaffolddomain.OperationPreflight, operationErr.Operation)
			require.Equal(t, scaffolddomain.FailureUnavailable, operationErr.Kind)
			require.ErrorIs(t, err, failure)
			require.Empty(t, result.Files)
		})
	}
}

func TestServiceRefreshesManagedFilesAndPreservesSeeds(t *testing.T) {
	t.Parallel()

	t.Run("refresh updates a different managed file", func(t *testing.T) {
		t.Parallel()

		fixture := newPreflightFixture(t, minimalServiceManifest())
		info := scaffoldFileInfo(t, "go.mod", "regular")
		fixture.expectWalkEntry("go.mod", info)
		fixture.allowPublication("go.mod", info, []byte("different"), "")

		result, err := fixture.service.Scaffold(context.Background(), scaffolddomain.Command{})

		require.NoError(t, err)
		requireFileAction(t, result, "go.mod", scaffolddomain.FileUpdated)
	})

	t.Run("refresh preserves a different user-owned seed", func(t *testing.T) {
		t.Parallel()

		manifest := minimalServiceManifest()
		manifest.Components.HTTP = &projectdomain.HTTP{Server: &projectdomain.HTTPServer{}}
		fixture := newPreflightFixture(t, manifest)
		path := "api/openapi/swagger.yaml"
		info := scaffoldFileInfo(t, path, "regular")
		fixture.expectWalkEntry(path, info)
		fixture.allowPublication(path, info, []byte("user-owned OpenAPI"), "")

		result, err := fixture.service.Scaffold(context.Background(), scaffolddomain.Command{})

		require.NoError(t, err)
		requireFileAction(t, result, path, scaffolddomain.FileUnchanged)
	})
}

func TestServiceRefreshesManagedFilesAndPreservesUserOwnedEntrypoints(t *testing.T) {
	t.Parallel()

	manifest := minimalServiceManifest()
	fixture := newPreflightFixture(t, manifest)
	mainPath := "cmd/sample/main.go"
	mainInfo := scaffoldFileInfo(t, mainPath, "regular")
	fixture.expectWalkEntry(mainPath, mainInfo)
	fixture.allowPublication(mainPath, mainInfo, []byte("package main\n// user changes\n"), mainPath)

	result, err := fixture.service.Scaffold(context.Background(), scaffolddomain.Command{})

	require.NoError(t, err)
	requireFileAction(t, result, mainPath, scaffolddomain.FileUnchanged)
}

func TestServiceRefreshNeverReadsOrPublishesCustomBufGenerationConfig(t *testing.T) {
	t.Parallel()

	manifest := minimalServiceManifest()
	manifest.Sources = map[string]projectdomain.Source{
		"contracts": {Type: projectdomain.SourceGit, Repo: "example/contracts", Ref: "v1"},
	}
	const customConfig = "tools/buf/billing.gen.yaml"
	manifest.Components.GRPC = &projectdomain.GRPC{Clients: []projectdomain.GRPCClient{{
		Name: "billing", Source: "contracts", Path: "proto/billing.proto", BufGenConfig: customConfig,
	}}}
	fixture := newPreflightFixture(t, manifest)
	fixture.expectWalkEntry(customConfig, scaffoldFileInfo(t, customConfig, "regular"))
	fixture.workspace.EXPECT().Lstat(gomock.Any(), fixture.root, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, target string) (fs.FileInfo, error) {
			require.NotEqual(t, customConfig, target)
			return nil, fs.ErrNotExist
		},
	).AnyTimes()
	fixture.workspace.EXPECT().PublishFile(gomock.Any(), fixture.root, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, target string, _ []byte) (artifact.PublishResult, error) {
			require.NotEqual(t, customConfig, target)
			return artifact.PublishResult{Action: artifact.PublishCreated}, nil
		},
	).AnyTimes()

	result, err := fixture.service.Scaffold(context.Background(), scaffolddomain.Command{})

	require.NoError(t, err)
	for _, change := range result.Files {
		require.NotEqual(t, customConfig, change.Path)
	}
}

func TestServiceIgnoresGitMetadataDuringPreflight(t *testing.T) {
	t.Parallel()

	fixture := newPreflightFixture(t, minimalServiceManifest())
	path := ".git/hooks/generated.go"
	fixture.expectWalkEntry(path, scaffoldFileInfo(t, path, "regular"))
	fixture.workspace.EXPECT().Lstat(gomock.Any(), fixture.root, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, name string) (fs.FileInfo, error) {
			require.NotEqual(t, path, name)
			return nil, fs.ErrNotExist
		},
	).AnyTimes()
	fixture.workspace.EXPECT().PublishFile(gomock.Any(), fixture.root, gomock.Any(), gomock.Any()).Return(artifact.PublishResult{Action: artifact.PublishCreated}, nil).AnyTimes()

	result, err := fixture.service.Scaffold(context.Background(), scaffolddomain.Command{})

	require.NoError(t, err)
	require.NotEmpty(t, result.Files)
}

type preflightFixture struct {
	t         *testing.T
	root      string
	workspace *mocks.MockWorkspaceRepository
	service   *scaffold.Service
}

func newPreflightFixture(t *testing.T, manifest projectdomain.Manifest) *preflightFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	projects := mocks.NewMockProjectRepository(ctrl)
	workspace := mocks.NewMockWorkspaceRepository(ctrl)
	project := projectdomain.Project{Root: "/project", Manifest: manifest}
	projects.EXPECT().LoadProject(gomock.Any(), "").Return(project, nil)
	return &preflightFixture{
		t:         t,
		root:      project.Root,
		workspace: workspace,
		service:   scaffold.New(zap.NewNop(), projects, workspace),
	}
}

func (f *preflightFixture) expectWalkEntry(path string, info fs.FileInfo) {
	f.workspace.EXPECT().Walk(gomock.Any(), f.root, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, visit fs.WalkDirFunc) error {
			return visit(path, fs.FileInfoToDirEntry(info), nil)
		},
	)
}

func (f *preflightFixture) allowPublication(
	existingPath string,
	info fs.FileInfo,
	existing []byte,
	forbiddenPublishPath string,
) {
	f.t.Helper()

	f.workspace.EXPECT().Lstat(gomock.Any(), f.root, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, name string) (fs.FileInfo, error) {
			if name == existingPath {
				return info, nil
			}
			return nil, fs.ErrNotExist
		},
	).AnyTimes()
	f.workspace.EXPECT().ReadBytes(gomock.Any(), f.root, existingPath).Return(existing, nil).AnyTimes()
	f.workspace.EXPECT().PublishFile(gomock.Any(), f.root, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, target string, _ []byte) (artifact.PublishResult, error) {
			require.NotEqual(f.t, forbiddenPublishPath, target)
			action := artifact.PublishCreated
			if target == existingPath {
				action = artifact.PublishUpdated
			}
			return artifact.PublishResult{Action: action}, nil
		},
	).AnyTimes()
}

func scaffoldFileInfo(t *testing.T, name, kind string) fs.FileInfo {
	t.Helper()

	files := fstest.MapFS{}
	switch kind {
	case "regular":
		files[name] = &fstest.MapFile{Data: []byte("existing"), Mode: 0o644}
	case "symlink":
		files[name] = &fstest.MapFile{Mode: fs.ModeSymlink | 0o777}
	case "directory":
		files[name+"/child"] = &fstest.MapFile{Data: []byte("child"), Mode: 0o644}
	default:
		require.FailNow(t, "unknown file kind", kind)
	}
	info, err := fs.Stat(files, name)
	require.NoError(t, err)
	return info
}

func minimalServiceManifest() projectdomain.Manifest {
	return projectdomain.Manifest{
		Version:   1,
		Project:   projectdomain.Identity{Name: "sample", Language: "go"},
		Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/sample"}},
	}
}

func requireFileAction(t *testing.T, result scaffolddomain.Result, path string, action scaffolddomain.FileAction) {
	t.Helper()

	for _, change := range result.Files {
		if change.Path == path {
			require.Equal(t, action, change.Action)
			return
		}
	}
	require.Fail(t, "file change not found", path)
}
