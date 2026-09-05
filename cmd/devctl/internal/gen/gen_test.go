package gen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/gen/mocks"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestGenCommandUsesGenService(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	generator := mocks.NewMockgenerator(ctrl)
	generator.EXPECT().Generate(gomock.Any(), generatedomain.Command{
		ManifestPath: "custom.yaml",
		Target:       "config",
		DryRun:       true,
	}).Return(generatedomain.Result{
		Targets: []string{"config"},
		Changes: []generatedomain.Change{{Target: "config", Path: "gen/config/config.gen.go", Action: generatedomain.ChangePlannedPublish}},
		DryRun:  true,
	}, nil)
	command := newGenCmd(genCmdOpts{}, func(_ context.Context, logger *zap.Logger) (genRuntime, error) {
		require.NotNil(t, logger)
		return genRuntime{generator: generator, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{
		"gen", "--file", "custom.yaml", "--json", "--verbose", "--target", "config", "--dry-run",
	})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"managed output generation completed"`)
	require.Contains(t, stdout.String(), `"command":"gen"`)
	require.Contains(t, stdout.String(), `"data":{"targets":["config"],"changes":[{"target":"config","path":"gen/config/config.gen.go","action":"planned_publish"}],"dry_run":true}`)
}

func TestGenReportsOperationAndShutdownFailuresWithPartialData(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	generator := mocks.NewMockgenerator(ctrl)
	operationErr := errors.New("generation failed")
	shutdownErr := errors.New("shutdown failed")
	generator.EXPECT().Generate(gomock.Any(), generatedomain.Command{}).Return(
		generatedomain.Result{Targets: []string{"config"}}, operationErr,
	)
	command := newGenCmd(genCmdOpts{}, func(context.Context, *zap.Logger) (genRuntime, error) {
		return genRuntime{generator: generator, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"gen", "--json"})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"targets": []any{"config"}, "changes": []any{}, "dry_run": false,
	}, details["partial_result"])
}
