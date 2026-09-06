package git_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitclient "github.com/devctllabs/devctl/internal/client/git"
	"github.com/stretchr/testify/require"
)

func TestClientChecksOutDetachedWorktreeAndCleansItUp(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repository, "openapi.yaml"), []byte("openapi: 3.1.0\n"), 0o644))
	commit := commitFixture(t, repository)

	var root string
	err := gitclient.New().WithCheckout(context.Background(), repository, commit, func(checkoutRoot string) error {
		root = checkoutRoot
		require.FileExists(t, filepath.Join(root, "openapi.yaml"))
		return nil
	})

	require.NoError(t, err)
	require.NoDirExists(t, root)
}

func commitFixture(t *testing.T, repository string) string {
	t.Helper()
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"config", "user.email", "devctl@example.test"},
		{"config", "user.name", "Devctl Test"},
		{"add", "."},
		{"commit", "--quiet", "-m", "fixture"},
	} {
		command := exec.CommandContext(context.Background(), "git", arguments...)
		command.Dir = repository
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	command := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	command.Dir = repository
	output, err := command.Output()
	require.NoError(t, err)
	return string(bytes.TrimSpace(output))
}
