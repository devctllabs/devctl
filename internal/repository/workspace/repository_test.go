package workspace_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	"github.com/devctllabs/devctl/internal/domain/contract"
	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	"github.com/stretchr/testify/require"
)

func TestFilesystemRepoPublishesAndPrunesManagedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/external/clienthttp/stale"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/external/clienthttp/stale/openapi.yaml"), []byte("stale"), 0o644))
	repository := workspacerepo.NewFilesystemRepo()

	published, err := repository.PublishDirectory(context.Background(), root, "api/external/clienthttp/active", artifact.Tree{Files: []artifact.File{
		{Path: "openapi.yaml", Content: []byte("active")},
	}})
	require.NoError(t, err)
	require.Equal(t, artifact.PublishCreated, published.Action)
	require.Equal(t, []artifact.PublishChange{{Path: "openapi.yaml", Action: artifact.PublishCreated}}, published.Changes)

	preview, err := repository.PreviewPruneDirectories(context.Background(), root, "api/external/clienthttp", []string{"active"})
	require.NoError(t, err)
	require.Equal(t, []string{"stale"}, preview)
	require.DirExists(t, filepath.Join(root, "api/external/clienthttp/stale"))

	removed, err := repository.PruneDirectories(context.Background(), root, "api/external/clienthttp", []string{"active"})
	require.NoError(t, err)
	require.Equal(t, []string{"stale"}, removed)
	require.NoDirExists(t, filepath.Join(root, "api/external/clienthttp/stale"))
	require.FileExists(t, filepath.Join(root, "api/external/clienthttp/active/openapi.yaml"))
}

func TestFilesystemRepoPreviewAndPruneRejectTheSameUnsafeEntries(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink policy requires symlink support")
	}

	tests := []struct {
		name   string
		parent string
		setup  func(*testing.T, string)
	}{
		{name: "unsafe parent", parent: "../outside", setup: func(*testing.T, string) {}},
		{name: "parent file", parent: "managed", setup: func(t *testing.T, root string) {
			t.Helper()
			require.NoError(t, os.WriteFile(filepath.Join(root, "managed"), []byte("file"), 0o644))
		}},
		{name: "parent symlink", parent: "managed", setup: func(t *testing.T, root string) {
			t.Helper()
			outside := t.TempDir()
			require.NoError(t, os.Symlink(outside, filepath.Join(root, "managed")))
		}},
		{name: "child file", parent: "managed", setup: func(t *testing.T, root string) {
			t.Helper()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "managed"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "managed", "stale"), []byte("file"), 0o644))
		}},
		{name: "child symlink", parent: "managed", setup: func(t *testing.T, root string) {
			t.Helper()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "managed"), 0o755))
			require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "managed", "stale")))
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			test.setup(t, root)
			repository := workspacerepo.NewFilesystemRepo()

			_, previewErr := repository.PreviewPruneDirectories(context.Background(), root, test.parent, nil)
			_, pruneErr := repository.PruneDirectories(context.Background(), root, test.parent, nil)

			require.ErrorIs(t, previewErr, fs.ErrInvalid)
			require.ErrorIs(t, pruneErr, fs.ErrInvalid)
		})
	}
}

func TestFilesystemRepoPublishesManagedFileIdempotently(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repository := workspacerepo.NewFilesystemRepo()

	published, err := repository.PublishFile(context.Background(), root, ".env.example", []byte("LOG_LEVEL=info\n"))
	require.NoError(t, err)
	require.Equal(t, artifact.PublishResult{Action: artifact.PublishCreated}, published)
	require.FileExists(t, filepath.Join(root, ".env.example"))

	published, err = repository.PublishFile(context.Background(), root, ".env.example", []byte("LOG_LEVEL=info\n"))
	require.NoError(t, err)
	require.Equal(t, artifact.PublishResult{Action: artifact.PublishUnchanged}, published)

	published, err = repository.PublishFile(context.Background(), root, ".env.example", []byte("LOG_LEVEL=debug\n"))
	require.NoError(t, err)
	require.Equal(t, artifact.PublishResult{Action: artifact.PublishUpdated}, published)
	content, err := os.ReadFile(filepath.Join(root, ".env.example"))
	require.NoError(t, err)
	require.Equal(t, []byte("LOG_LEVEL=debug\n"), content)
}

func TestFilesystemRepoReportsPreciseDirectoryPublicationChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "generated")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "equal.go"), []byte("equal"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(target, "changed.go"), []byte("before"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(target, "stale.go"), []byte("stale"), 0o644))

	repository := workspacerepo.NewFilesystemRepo()
	desired := artifact.Tree{Files: []artifact.File{
		{Path: "equal.go", Content: []byte("equal"), Mode: 0o644},
		{Path: "changed.go", Content: []byte("after"), Mode: 0o644},
		{Path: "created.go", Content: []byte("created"), Mode: 0o644},
	}}
	published, err := repository.PublishDirectory(context.Background(), root, "generated", desired)

	require.NoError(t, err)
	require.Equal(t, artifact.PublishUpdated, published.Action)
	require.Equal(t, []artifact.PublishChange{
		{Path: "changed.go", Action: artifact.PublishUpdated},
		{Path: "created.go", Action: artifact.PublishCreated},
		{Path: "equal.go", Action: artifact.PublishUnchanged},
		{Path: "stale.go", Action: artifact.PublishRemoved},
	}, published.Changes)
	require.NoFileExists(t, filepath.Join(target, "stale.go"))

	published, err = repository.PublishDirectory(context.Background(), root, "generated", desired)
	require.NoError(t, err)
	require.Equal(t, artifact.PublishUnchanged, published.Action)
	require.Equal(t, []artifact.PublishChange{
		{Path: "changed.go", Action: artifact.PublishUnchanged},
		{Path: "created.go", Action: artifact.PublishUnchanged},
		{Path: "equal.go", Action: artifact.PublishUnchanged},
	}, published.Changes)
}

func TestFilesystemRepoLocatesOneOpenAPIEntrypoint(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/external/clienthttp/remote/spec"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "api/external/clienthttp/remote/spec/openapi.yaml"), []byte("openapi: 3.1.0\n"), 0o644))
	contractPath, err := workspacerepo.NewFilesystemRepo().ResolveContract(context.Background(), contract.Location{Root: root, RelativePath: "api/external/clienthttp/remote"})

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "api/external/clienthttp/remote/spec/openapi.yaml"), contractPath)
}

func TestFilesystemRepoResolvesLocalEntrypointWithoutDuplicatingItsPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	relative := "api/openapi/swagger.yaml"
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api/openapi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte("openapi: 3.1.0\n"), 0o644))

	contractPath, err := workspacerepo.NewFilesystemRepo().ResolveContract(context.Background(), contract.Location{Root: root, RelativePath: relative, Entrypoint: relative, Local: true})

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, filepath.FromSlash(relative)), contractPath)
}

func TestFilesystemRepoListsProtoFilesInProjectOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{
		"api/proto/zeta/zeta.service.proto",
		"api/proto/alpha/alpha.common_types.proto",
		"api/proto/README.md",
	} {
		filename := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
		require.NoError(t, os.WriteFile(filename, []byte(name), 0o644))
	}

	files, err := workspacerepo.NewFilesystemRepo().ListProtoFiles(context.Background(), root, "api/proto")

	require.NoError(t, err)
	require.Equal(t, []string{
		"api/proto/alpha/alpha.common_types.proto",
		"api/proto/zeta/zeta.service.proto",
	}, files)
}
