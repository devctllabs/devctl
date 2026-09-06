package jsonschema_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/devctllabs/devctl/internal/client/jsonschema"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/devctllabs/devctl/internal/domain/artifact"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestClientDelegatesQuicktypeGenerationToRunner(t *testing.T) { //nolint:paralleltest // PATH is part of the LookPath contract.
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"title":"AuditEvent","type":"object"}`)
	quicktype := installQuicktypePlaceholder(t)
	var temporary string
	runner := &recordingRunner{run: func(command toolrun.Command) error {
		outputPath := command.Args[11]
		temporary = filepath.Dir(outputPath)
		writeFixture(t, temporary, filepath.Base(outputPath), "package audit_events\n\ntype AuditEvent struct{}\n")
		return nil
	}}

	output, err := jsonschema.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, jsonTarget())

	require.NoError(t, err)
	require.Equal(t, []toolrun.Command{{
		Name: quicktype,
		Args: []string{
			"--src", "api/contracts/audit.json", "--src-lang", "schema", "--lang", "go",
			"--package", "audit_events", "--top-level", "AuditEvent",
			"--out", filepath.Join(temporary, "schema.gen.go"), "--omit-empty",
		},
		Dir: root,
	}}, runner.commands)
	require.Equal(t, artifact.Tree{Files: []artifact.File{{
		Path: "schema.gen.go", Content: []byte("package audit_events\n\ntype AuditEvent struct{}\n"), Mode: 0o644,
	}}}, output.Directory)
	require.NoDirExists(t, temporary)
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

func installQuicktypePlaceholder(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	executable := filepath.Join(directory, "quicktype")
	require.NoError(t, os.WriteFile(executable, nil, 0o755))
	t.Setenv("PATH", directory)
	return executable
}
