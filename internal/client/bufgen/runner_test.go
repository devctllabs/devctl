package bufgen_test

import (
	"context"
	"testing"

	bufgen "github.com/devctllabs/devctl/internal/client/bufgen"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/devctllabs/devctl/internal/domain/artifact"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestClientDelegatesBufGenerationToRunner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/service\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n")
	writeFixture(t, root, "api/proto/acme/v1/service.proto", "syntax = \"proto3\";\npackage acme.v1;\n")
	writeFixture(t, root, "tools/buf/grpc.gen.yaml", "version: v2\nplugins: []\n")
	var temporary string
	runner := &recordingRunner{run: func(command toolrun.Command) error {
		temporary = command.Args[len(command.Args)-1]
		writeFixture(t, temporary, "acme/v1/service.pb.go", "generated")
		return nil
	}}
	target := projectdomain.Target{
		ID: "grpc-server", Family: "grpc", Input: "api/proto",
		Paths: []string{"acme/v1"}, Config: "tools/buf/grpc.gen.yaml",
	}

	output, err := bufgen.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, target)

	require.NoError(t, err)
	require.Equal(t, []toolrun.Command{{
		Name: "go",
		Args: []string{"tool", "buf", "generate", "api/proto", "--template", "tools/buf/grpc.gen.yaml", "--path", "acme/v1", "--output", temporary},
		Dir:  root,
	}}, runner.commands)
	require.Equal(t, artifact.Tree{Files: []artifact.File{{Path: "acme/v1/service.pb.go", Content: []byte("generated"), Mode: 0o644}}}, output.Directory)
	require.NoDirExists(t, temporary)
}

func TestClientDelegatesBufLintToRunnerWithoutTemporaryOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/service\n\ngo 1.25\n\ntool github.com/bufbuild/buf/cmd/buf\n")
	writeFixture(t, root, "api/proto/acme/v1/service.proto", "syntax = \"proto3\";\npackage acme.v1;\n")
	runner := &recordingRunner{}
	target := projectdomain.Target{
		ID: "grpc-server", Family: "grpc", Input: "api/proto", Paths: []string{"acme/v1"},
	}

	err := bufgen.New(runner).Lint(context.Background(), projectdomain.Project{Root: root}, target)

	require.NoError(t, err)
	require.Equal(t, []toolrun.Command{{
		Name: "go", Args: []string{"tool", "buf", "lint", "api/proto", "--path", "acme/v1"}, Dir: root,
	}}, runner.commands)
}

type recordingRunner struct {
	commands []toolrun.Command
	run      func(toolrun.Command) error
}

func (r *recordingRunner) Run(_ context.Context, command toolrun.Command) error {
	command.Args = append([]string(nil), command.Args...)
	r.commands = append(r.commands, command)
	if r.run != nil {
		return r.run(command)
	}
	return nil
}
