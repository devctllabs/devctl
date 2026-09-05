package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/sync/mocks"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	syncdomain "github.com/devctllabs/devctl/internal/domain/sync"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestSyncCommandBuildsAndRunsOnlySyncService(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	syncer := mocks.NewMocksyncer(ctrl)
	syncer.EXPECT().Sync(gomock.Any(), syncdomain.Command{
		ManifestPath: "custom.yaml",
		Target:       "http-client:remote",
		DryRun:       true,
	}).Return(syncdomain.Result{
		Targets: []string{"http-client:remote"},
		Changes: []syncdomain.Change{{Target: "http-client:remote", Path: "api/external/clienthttp/remote", Action: syncdomain.ChangePlannedPublish}},
		DryRun:  true,
	}, nil)
	command := newSyncCmd(syncCmdOpts{}, func(_ context.Context, logger *zap.Logger) (syncRuntime, error) {
		require.NotNil(t, logger)
		return syncRuntime{syncer: syncer, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{
		"sync", "--file", "custom.yaml", "--json", "--verbose", "--target", "http-client:remote", "--dry-run",
	})

	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"msg":"contract synchronization completed"`)
	require.Contains(t, stdout.String(), `"command":"sync"`)
	require.Contains(t, stdout.String(), `"data":{"targets":["http-client:remote"],"changes":[{"target":"http-client:remote","path":"api/external/clienthttp/remote","action":"planned_publish"}],"dry_run":true}`)
}

func TestSyncReportsOperationAndShutdownFailuresWithPartialData(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	syncer := mocks.NewMocksyncer(ctrl)
	operationErr := errors.New("sync failed")
	shutdownErr := errors.New("shutdown failed")
	syncer.EXPECT().Sync(gomock.Any(), syncdomain.Command{}).Return(
		syncdomain.Result{Targets: []string{"http-client:catalog"}}, operationErr,
	)
	command := newSyncCmd(syncCmdOpts{}, func(context.Context, *zap.Logger) (syncRuntime, error) {
		return syncRuntime{syncer: syncer, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"sync", "--json"})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	require.Equal(t, map[string]any{
		"partial_result": map[string]any{
			"targets": []any{"http-client:catalog"}, "changes": []any{}, "dry_run": false,
		},
	}, event["details"])
}

func TestSyncDryRunFailureOmitsPlannedPartialResult(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	syncer := mocks.NewMocksyncer(ctrl)
	cause := errors.New("planning failed")
	syncer.EXPECT().Sync(gomock.Any(), syncdomain.Command{DryRun: true}).Return(
		syncdomain.Result{Targets: []string{"config"}, DryRun: true}, cause,
	)
	command := newSyncCmd(syncCmdOpts{}, func(context.Context, *zap.Logger) (syncRuntime, error) {
		return syncRuntime{syncer: syncer, shutdown: func(context.Context) error { return nil }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"sync", "--json", "--dry-run"})

	require.ErrorIs(t, err, cause)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	require.NotContains(t, event, "details")
}

func TestSyncReportsMaterializationCategoriesWithoutRawCause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind materializedomain.FailureKind
		code string
	}{
		{name: "invalid input", kind: materializedomain.FailureInvalid, code: "invalid_input"},
		{name: "not found", kind: materializedomain.FailureNotFound, code: "not_found"},
		{name: "unsupported", kind: materializedomain.FailureUnsupported, code: "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			syncer := mocks.NewMocksyncer(ctrl)
			cause := &materializedomain.OperationError{
				Operation: materializedomain.OperationBuildSnapshot, Kind: test.kind,
			}
			syncer.EXPECT().Sync(gomock.Any(), syncdomain.Command{}).Return(syncdomain.Result{}, cause)
			command := newSyncCmd(syncCmdOpts{}, func(context.Context, *zap.Logger) (syncRuntime, error) {
				return syncRuntime{syncer: syncer, shutdown: func(context.Context) error { return nil }}, nil
			})
			command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
			var stderr bytes.Buffer
			command.ErrWriter = &stderr

			err := command.Run(context.Background(), []string{"sync", "--json"})

			require.ErrorIs(t, err, cause)
			var event map[string]any
			require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
			require.Equal(t, test.code, event["code"])
			require.NotContains(t, event, "error")
			require.NotContains(t, stderr.String(), string(materializedomain.OperationBuildSnapshot))
		})
	}
}
