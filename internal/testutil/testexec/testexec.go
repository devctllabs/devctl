// Package testexec provides fixtures for testing subprocess clients.
package testexec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// StubPathCommand installs script as name and returns a PATH that resolves it first.
func StubPathCommand(t *testing.T, name, script string) string {
	t.Helper()
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755))
	return bin + string(os.PathListSeparator) + os.Getenv("PATH")
}
