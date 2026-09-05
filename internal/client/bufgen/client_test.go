package bufgen_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	bufgen "github.com/devctllabs/devctl/internal/client/bufgen"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestClientPreservesBufRunnerFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/service\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n")
	writeFixture(t, root, "api/proto/acme/v1/service.proto", "syntax = \"proto3\";\npackage acme.v1;\n")
	writeFixture(t, root, "tools/buf/grpc.gen.yaml", "version: v2\nplugins: []\n")
	primary := errors.New("buf process failed")
	runner := &recordingRunner{run: func(toolrun.Command) error { return primary }}
	target := projectdomain.Target{
		ID: "grpc-server", Family: "grpc", Input: "api/proto", Config: "tools/buf/grpc.gen.yaml",
	}

	_, err := bufgen.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, target)

	require.ErrorIs(t, err, primary)
	require.ErrorContains(t, err, "runner.Run")
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}
