package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderReferenceIsDeterministicAndVersionless(t *testing.T) {
	t.Parallel()

	first, err := renderReference()
	require.NoError(t, err)
	second, err := renderReference()
	require.NoError(t, err)
	require.Equal(t, first, second)

	text := string(first)
	require.Contains(t, text, "# Command reference")
	require.Contains(t, text, "### `init` command")
	require.Contains(t, text, "### `init manifest` subcommand")
	require.Contains(t, text, "--preset")
	require.Contains(t, text, "http-service")
	require.NotContains(t, text, "commit:")
	require.NotContains(t, text, "Version:")
}

func TestCheckReferenceDetectsDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "commands.md"), []byte("current\n"), 0o644))
	require.NoError(t, checkReference(root, "commands.md", []byte("current\n")))
	require.ErrorContains(t, checkReference(root, "commands.md", []byte("expected\n")), "command reference is out of date")
}

func TestWriteReferenceCreatesParentDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, writeReference(root, "docs/reference/commands.md", []byte("generated\n")))

	actual, err := os.ReadFile(filepath.Join(root, "docs/reference/commands.md"))
	require.NoError(t, err)
	require.Equal(t, []byte("generated\n"), actual)
}

func TestWriteReferenceRejectsSymlinkTarget(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink policy requires symlink support")
	}

	root := t.TempDir()
	original := filepath.Join(t.TempDir(), "commands.md")
	require.NoError(t, os.WriteFile(original, []byte("original\n"), 0o644))
	target := filepath.Join(root, "commands.md")
	require.NoError(t, os.Symlink(original, target))

	err := writeReference(root, "commands.md", []byte("replacement\n"))

	require.ErrorIs(t, err, fs.ErrInvalid)
	linkTarget, readLinkErr := os.Readlink(target)
	require.NoError(t, readLinkErr)
	require.Equal(t, original, linkTarget)
	actual, readErr := os.ReadFile(original)
	require.NoError(t, readErr)
	require.Equal(t, []byte("original\n"), actual)
}
