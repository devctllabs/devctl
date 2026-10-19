package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrateCIWorkflow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := filepath.Join(root, "devctl.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`version: 1
project: {name: orders-api, language: go}
env: {}
paths: {}
sources: {}
exports: {}
components:
  db:
    connections:
      - name: primary
        default: postgres
        variants:
          - name: postgres
            kind: postgres
            dsn_env: DB_DSN
          - name: sqlite
            kind: sqlite
            dsn_env: CI_DSN
            migrations:
              path: db/migrations
              database_env: MIGRATIONS_URL
languages:
  go: {module: example.test/orders}
`), 0o644))
	// No go.mod or scaffold readiness: migration operations use only their inputs.
	created := runCLI(t, buildCLI(t), "migrate", "create", "create_orders", "--file", manifest, "--json")
	var event struct {
		Data struct {
			Target   string   `json:"target"`
			Files    []string `json:"files"`
			Version  *uint    `json:"version"`
			Dirty    bool     `json:"dirty"`
			NoChange bool     `json:"no_change"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(created), &event))
	require.Equal(t, "primary:sqlite", event.Data.Target)
	require.Len(t, event.Data.Files, 2)
	upFile := filepath.Join(root, event.Data.Files[0])
	require.NoError(t, os.WriteFile(filepath.Join(root, event.Data.Files[0]), []byte("CREATE TABLE orders (id INTEGER PRIMARY KEY);"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, event.Data.Files[1]), []byte("DROP TABLE orders;"), 0o644))
	envFile := filepath.Join(root, "migration.env")
	require.NoError(t, os.WriteFile(envFile, []byte("ORDERS_API_MIGRATIONS_URL=sqlite://"+filepath.Join(root, "orders.db")+"\n"), 0o600))
	for _, operation := range []string{"up", "up", "status", "down"} {
		output := runCLI(t, buildCLI(t), "migrate", operation, "--target", "primary:sqlite", "--file", manifest, "--env-file", envFile, "--json")
		event.Data.Version = nil
		require.NoError(t, json.Unmarshal([]byte(output), &event))
		require.Equal(t, "primary:sqlite", event.Data.Target)
		require.False(t, event.Data.Dirty)
		if operation == "down" {
			require.Nil(t, event.Data.Version)
		} else {
			require.NotNil(t, event.Data.Version)
		}
	}
	require.FileExists(t, filepath.Join(root, "orders.db"))

	// SQL errors remain redacted even under --verbose and expose observed dirty state.
	require.NoError(t, os.WriteFile(upFile, []byte("INVALID_SQL SUPER_SECRET_TOKEN"), 0o644))
	command := exec.CommandContext(context.Background(), buildCLI(t), "migrate", "up", "--file", manifest, "--env-file", envFile, "--json", "--verbose")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	var exitError *exec.ExitError
	require.ErrorAs(t, command.Run(), &exitError)
	require.Equal(t, 1, exitError.ExitCode())
	require.Empty(t, stdout.String())
	require.NotContains(t, stderr.String(), "SUPER_SECRET_TOKEN")
	require.NotContains(t, stderr.String(), "sqlite://")
	require.Contains(t, stderr.String(), `"partial_result"`)
	require.Contains(t, stderr.String(), `"dirty":true`)
}
