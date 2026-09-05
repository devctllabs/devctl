package toolsafety_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devctllabs/devctl/internal/client/toolsafety"
	"github.com/devctllabs/devctl/internal/domain/artifact"
	"github.com/stretchr/testify/require"
)

func TestReadRegularFileAcceptsContainedRelativeAndAbsolutePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	filename := filepath.Join(root, "nested", "input.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte("schema"), 0o600))

	for _, name := range []string{"nested/input.json", filename} {
		file, err := toolsafety.ReadRegularFile(root, name)

		require.NoError(t, err)
		require.Equal(t, []byte("schema"), file.Content)
		require.Equal(t, os.FileMode(0o600), file.Mode.Perm())
	}
}

func TestReadRegularFileRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*testing.T, string) string
	}{
		{name: "relative traversal", setup: func(t *testing.T, _ string) string {
			t.Helper()
			outside := filepath.Join(t.TempDir(), "outside.txt")
			require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
			return "../" + filepath.Base(filepath.Dir(outside)) + "/outside.txt"
		}},
		{name: "absolute outside", setup: func(t *testing.T, _ string) string {
			t.Helper()
			outside := filepath.Join(t.TempDir(), "outside.txt")
			require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
			return outside
		}},
		{name: "leaf symlink", setup: func(t *testing.T, root string) string {
			t.Helper()
			outside := filepath.Join(t.TempDir(), "outside.txt")
			require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
			require.NoError(t, os.Symlink(outside, filepath.Join(root, "input.json")))
			return "input.json"
		}},
		{name: "ancestor symlink", setup: func(t *testing.T, root string) string {
			t.Helper()
			outside := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(outside, "input.json"), []byte("outside"), 0o644))
			require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked")))
			return "linked/input.json"
		}},
		{name: "directory", setup: func(t *testing.T, root string) string {
			t.Helper()
			require.NoError(t, os.Mkdir(filepath.Join(root, "input.json"), 0o755))
			return "input.json"
		}},
		{name: "socket", setup: func(t *testing.T, root string) string {
			t.Helper()
			return listenUnixSocket(t, root, "input.sock")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if test.name == "socket" {
				root = shortTempDir(t)
			}

			_, err := toolsafety.ReadRegularFile(root, test.setup(t, root))

			require.Error(t, err)
		})
	}
}

func TestRequireDirectoryRejectsFilesAndSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "contracts"), 0o755))
	require.NoError(t, toolsafety.RequireDirectory(root, "contracts"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file"), []byte("data"), 0o644))
	require.Error(t, toolsafety.RequireDirectory(root, "file"))
	require.NoError(t, os.Symlink(filepath.Join(root, "contracts"), filepath.Join(root, "linked")))
	require.Error(t, toolsafety.RequireDirectory(root, "linked"))
}

func TestReadRegularTreeReturnsStableFilesAndRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, artifact.File{Path: "generated/zeta.go", Content: []byte("zeta"), Mode: 0o644})
	writeFile(t, root, artifact.File{Path: "generated/alpha/nested.go", Content: []byte("alpha"), Mode: 0o600})

	tree, err := toolsafety.ReadRegularTree(root, "generated")

	require.NoError(t, err)
	require.Equal(t, artifact.Tree{Files: []artifact.File{
		{Path: "alpha/nested.go", Content: []byte("alpha"), Mode: 0o600},
		{Path: "zeta.go", Content: []byte("zeta"), Mode: 0o644},
	}}, tree)

	require.NoError(t, os.Symlink(filepath.Join(root, "generated", "zeta.go"), filepath.Join(root, "generated", "linked.go")))
	_, err = toolsafety.ReadRegularTree(root, "generated")
	require.Error(t, err)

	socketRoot := shortTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(socketRoot, "generated"), 0o755))
	listenUnixSocket(t, filepath.Join(socketRoot, "generated"), "output.sock")
	_, err = toolsafety.ReadRegularTree(socketRoot, "generated")
	require.Error(t, err)
}

func TestBoundedOutputTrimsAndLimitsDiagnostics(t *testing.T) {
	t.Parallel()

	require.Equal(t, "message", toolsafety.BoundedOutput([]byte("  message\n")))
	exact := strings.Repeat("x", 4096)
	require.Equal(t, exact, toolsafety.BoundedOutput([]byte(exact)))
	require.Equal(t, exact, toolsafety.BoundedOutput([]byte(exact+"overflow")))
}

func writeFile(t *testing.T, root string, file artifact.File) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(file.Path))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, file.Content, os.FileMode(file.Mode)))
}

func listenUnixSocket(t *testing.T, root, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket file-kind check is not available on Windows")
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", filepath.Join(root, name))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	return name
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	//nolint:usetesting // A regular t.TempDir path exceeds the Unix socket path limit on macOS.
	root, err := os.MkdirTemp("/tmp", "devctl-toolsafety-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })
	return root
}
