package add

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddSourceWritesManifestChangeJSON(t *testing.T) {
	t.Parallel()
	manifestPath := writeManifest(t)
	command := newSourceCmd(sourceCmdOpts{}, buildSource)
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{
		"source", "catalog", "--file", manifestPath, "--json", "--type", "local", "--path", "api/catalog.yaml",
	})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"project resource addition completed"`)
	require.Contains(t, stdout.String(), `"command":"source"`)
	require.Contains(t, stdout.String(), `"data":{"manifest":"`+manifestPath+`","change":"updated"}`)
}

func writeManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
project: {name: sample, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components: {}
languages:
  go: {module: example.test/sample}
`), 0o644))
	return path
}
