package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/lint/mocks"
	lintdomain "github.com/devctllabs/devctl/internal/domain/lint"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestLintCommandUsesLintService(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	linter := mocks.NewMocklinter(ctrl)
	linter.EXPECT().Lint(gomock.Any(), lintdomain.Command{ManifestPath: "custom.yaml"}).Return(lintdomain.Result{Valid: true, Contracts: []string{"http-server"}, Issues: []lintdomain.Issue{}}, nil)
	command := newLintCmd(lintCmdOpts{}, func(_ context.Context, logger *zap.Logger) (lintRuntime, error) {
		require.NotNil(t, logger)
		return lintRuntime{linter: linter, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{"lint", "--file", "custom.yaml", "--json", "--verbose"})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"contract lint completed"`)
	require.Contains(t, stdout.String(), `"command":"lint"`)
	require.Contains(t, stdout.String(), `"data":{"valid":true,"contracts":["http-server"],"issues":[]}`)
}

func TestLintActionEmitsInvalidResultAndExitsOneWithoutErrorEvent(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	linter := mocks.NewMocklinter(ctrl)
	linter.EXPECT().Lint(gomock.Any(), lintdomain.Command{}).Return(lintdomain.Result{
		Valid: false, Contracts: []string{"http-server"}, Issues: []lintdomain.Issue{{Code: "operation_id_missing"}},
	}, nil)
	command := newLintCmd(lintCmdOpts{}, func(context.Context, *zap.Logger) (lintRuntime, error) {
		return lintRuntime{linter: linter, shutdown: func(context.Context) error { return nil }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Writer = &stdout
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"lint", "--json"})

	var exitCoder cli.ExitCoder
	require.ErrorAs(t, err, &exitCoder)
	require.Equal(t, 1, exitCoder.ExitCode())
	require.Contains(t, stdout.String(), `"data":{"valid":false`)
	require.Empty(t, stderr.String())
}

func TestLintReportsOperationAndShutdownFailuresWithPartialData(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	linter := mocks.NewMocklinter(ctrl)
	operationErr := errors.New("lint failed")
	shutdownErr := errors.New("shutdown failed")
	linter.EXPECT().Lint(gomock.Any(), lintdomain.Command{}).Return(
		lintdomain.Result{Contracts: []string{"http-server"}}, operationErr,
	)
	command := newLintCmd(lintCmdOpts{}, func(context.Context, *zap.Logger) (lintRuntime, error) {
		return lintRuntime{linter: linter, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"lint", "--json"})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"valid": false, "contracts": []any{"http-server"}, "issues": []any{},
	}, details["partial_result"])
}
