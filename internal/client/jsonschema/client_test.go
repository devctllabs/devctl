package jsonschema_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/devctllabs/devctl/internal/client/jsonschema"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestClientRejectsSchemaWithoutTitleBeforeRunningQuicktype(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"type":"object"}`)
	t.Setenv("PATH", t.TempDir())
	runner := &recordingRunner{}

	_, err := jsonschema.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, jsonTarget())

	require.ErrorContains(t, err, "root title is required")
	require.Empty(t, runner.commands)
}

func TestClientReportsMissingQuicktypeWithMiseGuidance(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"title":"AuditEvent","type":"object"}`)
	t.Setenv("PATH", t.TempDir())

	_, err := jsonschema.New(&recordingRunner{}).Generate(context.Background(), projectdomain.Project{Root: root}, jsonTarget())

	require.ErrorContains(t, err, "quicktype is unavailable")
	require.ErrorContains(t, err, "run mise install")
}

func TestClientPreservesQuicktypeRunnerFailure(t *testing.T) { //nolint:paralleltest // PATH is part of the LookPath contract.
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"title":"AuditEvent","type":"object"}`)
	installQuicktypePlaceholder(t)
	primary := errors.New("quicktype process failed")
	runner := &recordingRunner{run: func(toolrun.Command) error { return primary }}

	_, err := jsonschema.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, jsonTarget())

	require.ErrorIs(t, err, primary)
	require.ErrorContains(t, err, "runner.Run")
}

func TestClientRejectsMalformedGeneratedGo(t *testing.T) { //nolint:paralleltest // The helper sets PATH for LookPath.
	err := generateWithFakeOutput(t, "not Go")

	require.ErrorContains(t, err, "parser.ParseFile")
}

func TestClientRejectsUnexpectedGeneratedPackage(t *testing.T) { //nolint:paralleltest // The helper sets PATH for LookPath.
	err := generateWithFakeOutput(t, "package wrong\n")

	require.ErrorContains(t, err, `generated package "wrong" does not match "audit_events"`)
}

func TestClientReportsMissingGeneratedOutput(t *testing.T) { //nolint:paralleltest // PATH is part of the LookPath contract.
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"title":"AuditEvent","type":"object"}`)
	installQuicktypePlaceholder(t)

	_, err := jsonschema.New(&recordingRunner{}).Generate(context.Background(), projectdomain.Project{Root: root}, jsonTarget())

	require.ErrorContains(t, err, "toolsafety.ReadRegularFile output")
}

func TestClientRejectsUnsafeOutputNameBeforeResolvingQuicktype(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"title":"AuditEvent","type":"object"}`)
	t.Setenv("PATH", t.TempDir())
	target := jsonTarget()
	target.OutputFile = "../outside.go"

	_, err := jsonschema.New(&recordingRunner{}).Generate(context.Background(), projectdomain.Project{Root: root}, target)

	require.EqualError(t, err, `invalid generated output filename "../outside.go"`)
}

func TestClientHonorsCancellationBeforeGeneration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := jsonschema.New(&recordingRunner{}).Generate(ctx, projectdomain.Project{}, jsonTarget())

	require.ErrorIs(t, err, context.Canceled)
}

func generateWithFakeOutput(t *testing.T, content string) error {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "api/contracts/audit.json", `{"title":"AuditEvent","type":"object"}`)
	installQuicktypePlaceholder(t)
	runner := &recordingRunner{run: func(command toolrun.Command) error {
		outputPath := command.Args[11]
		writeFixture(t, filepath.Dir(outputPath), filepath.Base(outputPath), content)
		return nil
	}}
	_, err := jsonschema.New(runner).Generate(context.Background(), projectdomain.Project{Root: root}, jsonTarget())
	if err != nil {
		return fmt.Errorf("client.Generate: %w", err)
	}
	return nil
}

func jsonTarget() projectdomain.Target {
	return projectdomain.Target{
		ID: "kafka-consumer:audit-events", Family: "kafka", Format: "json",
		Input: "api/contracts/audit.json", OutputDir: "gen/kafka/consumer/audit-events", OutputFile: "schema.gen.go",
	}
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}
