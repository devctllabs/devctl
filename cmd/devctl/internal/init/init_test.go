package initcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/init/mocks"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	scaffolddomain "github.com/devctllabs/devctl/internal/domain/scaffold"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestInitManifestWritesCommandSpecificJSON(t *testing.T) {
	t.Parallel()
	destination := filepath.Join(t.TempDir(), "devctl.yaml")
	command := newManifestCmd(manifestCmdOpts{}, buildManifest)
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{
		"manifest", "--file", destination, "--json",
		"--lang", "go", "--preset", "cli", "--name", "sample", "--module", "example.test/sample",
	})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"manifest initialization completed"`)
	require.Contains(t, stdout.String(), `"command":"manifest"`)
	require.Contains(t, stdout.String(), `"data":{"manifest":"`+destination+`","change":"created"}`)
	_, err = os.Stat(destination)
	require.NoError(t, err)
}

func TestInitManifestForceReplacesDifferentRegularFile(t *testing.T) {
	t.Parallel()
	destination := filepath.Join(t.TempDir(), "devctl.yaml")
	require.NoError(t, os.WriteFile(destination, []byte("different\n"), 0o644))
	command := newManifestCmd(manifestCmdOpts{}, buildManifest)
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{
		"manifest", "--file", destination, "--json", "--force",
		"--lang", "go", "--preset", "cli", "--name", "sample", "--module", "example.test/sample",
	})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"data":{"manifest":"`+destination+`","change":"updated"}`)
}

func TestInitManifestReportsOperationAndShutdownFailuresWithManifestData(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	initializer := mocks.NewMockmanifestInitializer(ctrl)
	operationErr := errors.New("initialization failed")
	shutdownErr := errors.New("shutdown failed")
	initializer.EXPECT().InitManifest(gomock.Any(), projectdomain.InitManifestCommand{
		Language: "go", Preset: "cli", Name: "sample", Module: "example.test/sample",
	}).Return(projectdomain.ManifestResult{Manifest: "/project/devctl.yaml", Change: projectdomain.ChangeUpdated}, operationErr)
	command := newManifestCmd(manifestCmdOpts{}, func(context.Context, *zap.Logger) (manifestRuntime, error) {
		return manifestRuntime{initializer: initializer, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{
		"manifest", "--json", "--lang", "go", "--preset", "cli", "--name", "sample", "--module", "example.test/sample",
	})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"manifest": "/project/devctl.yaml", "change": "updated"}, details["partial_result"])
}

func TestInitScaffoldReportsOperationAndShutdownFailuresWithPartialData(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	scaffolder := mocks.NewMockscaffolder(ctrl)
	operationErr := errors.New("scaffold failed")
	shutdownErr := errors.New("shutdown failed")
	scaffolder.EXPECT().Scaffold(gomock.Any(), scaffolddomain.Command{}).Return(scaffolddomain.Result{
		Files: []scaffolddomain.FileChange{{Path: "go.mod", Action: scaffolddomain.FileCreated}},
	}, operationErr)
	command := newScaffoldCmd(scaffoldCmdOpts{}, func(context.Context, *zap.Logger) (scaffoldRuntime, error) {
		return scaffoldRuntime{scaffolder: scaffolder, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"scaffold", "--json"})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"files": []any{map[string]any{"path": "go.mod", "action": "created"}}}, details["partial_result"])
}

func TestInitScaffoldDoesNotExposeForceFlag(t *testing.T) {
	t.Parallel()

	command := newScaffoldCmd(scaffoldCmdOpts{}, func(context.Context, *zap.Logger) (scaffoldRuntime, error) {
		require.FailNow(t, "runtime must not be built for an unknown flag")
		return scaffoldRuntime{}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"scaffold", "--force"})

	require.Error(t, err)
	var exitCoder cli.ExitCoder
	require.ErrorAs(t, err, &exitCoder)
	require.Equal(t, 2, exitCoder.ExitCode())
}
