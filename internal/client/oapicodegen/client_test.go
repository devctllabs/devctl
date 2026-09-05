package oapicodegen_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	oapicodegen "github.com/devctllabs/devctl/internal/client/oapicodegen"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestClientPreservesOAPIRunnerFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/service\n\ngo 1.26\n\ntool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen\n")
	writeFixture(t, root, "api/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: service\n  version: 1.0.0\npaths: {}\n")
	writeFixture(t, root, "tools/oapi/client.yaml", "package: clienthttp\ngenerate:\n  client: true\n")
	primary := errors.New("oapi process failed")
	runner := &recordingRunner{run: func(toolrun.Command) error { return primary }}
	target := projectdomain.Target{
		ID: "http-client:payments", Family: "http", Input: "api/openapi.yaml",
		Config: "tools/oapi/client.yaml", OutputFile: "client.gen.go",
	}

	_, err := oapicodegen.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, target)

	require.ErrorIs(t, err, primary)
	require.ErrorContains(t, err, "runner.Run")
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}
