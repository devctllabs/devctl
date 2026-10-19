package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/cmd/devctl/internal/migrate/mocks"
	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestMigrationCommandsPassTypedInputAndEmitOneEvent(t *testing.T) {
	t.Parallel()
	for _, operation := range []migrationdomain.Operation{migrationdomain.Create, migrationdomain.Up, migrationdomain.Down, migrationdomain.Status} {
		t.Run(string(operation), func(t *testing.T) {
			t.Parallel()
			input := migrationdomain.Command{Operation: operation, ManifestPath: "custom.yaml", Target: "primary:postgres"}
			args := []string{"migrate", string(operation), "--file", "custom.yaml", "--target", "primary:postgres", "--json"}
			result := migrationdomain.Result{Target: input.Target, State: &migrationdomain.State{Dirty: true}}
			if operation == migrationdomain.Create {
				input.Name = "create_orders"
				args = append(args, input.Name)
				result.State = nil
				result.Files = []string{"migrations/1_create_orders.up.sql", "migrations/1_create_orders.down.sql"}
			} else {
				input.EnvFile = ".env.ci"
				args = append(args, "--env-file", input.EnvFile)
			}
			if operation == migrationdomain.Up || operation == migrationdomain.Down {
				input.Steps = 3
				args = append(args, "--steps", "3")
			}
			capability := mocks.NewMockmigrator(gomock.NewController(t))
			capability.EXPECT().Migrate(gomock.Any(), input).Return(result, nil)
			closed := false
			cmd := newMigrateCmd(func(context.Context, *zap.Logger) (migrateRuntime, error) {
				return migrateRuntime{migrator: capability, shutdown: func(context.Context) error { closed = true; return nil }}, nil
			})
			var stdout, stderr bytes.Buffer
			cmd.Writer, cmd.ErrWriter = &stdout, &stderr
			cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
			require.NoError(t, cmd.Run(context.Background(), args))
			require.True(t, closed)
			require.Empty(t, stderr.String())
			var event struct {
				Command string `json:"command"`
				Data    struct {
					Target  string   `json:"target"`
					Version *uint    `json:"version"`
					Dirty   bool     `json:"dirty"`
					Files   []string `json:"files"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &event))
			require.Equal(t, "migrate "+string(operation), event.Command)
			require.Equal(t, input.Target, event.Data.Target)
			if operation == migrationdomain.Create {
				require.Equal(t, result.Files, event.Data.Files)
			} else {
				require.Nil(t, event.Data.Version)
				require.True(t, event.Data.Dirty)
			}
		})
	}
}

func TestMigrationHelpAndInvalidInputDoNotBuildRuntime(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--help"}, {"up", "--help"}, {"create", "--help"},
		{"create"}, {"create", "../escape"}, {"create", "one", "two"},
		{"up", "unexpected"}, {"up", "--target", "primary"},
		{"up", "--steps", "0"}, {"up", "--steps", "-1"}, {"up", "--steps", "bad"},
		{"down", "--steps", "0"}, {"down", "--steps", "-1"}, {"down", "--steps", "bad"},
		{"status", "--env-file", ""},
	} {
		t.Run("input_"+strings.Join(args, "_"), func(t *testing.T) {
			t.Parallel()
			cmd := newMigrateCmd(func(context.Context, *zap.Logger) (migrateRuntime, error) {
				t.Fatal("runtime constructed for help or invalid input")
				return migrateRuntime{}, nil
			})
			var output bytes.Buffer
			cmd.Writer, cmd.ErrWriter = &output, &output
			cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
			err := cmd.Run(context.Background(), append([]string{"migrate"}, args...))
			if args[len(args)-1] == "--help" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, 2, commandruntime.ExitCode(err))
			}
		})
	}
}

func TestMigrationErrorsJoinCleanupAndReportPartialWork(t *testing.T) {
	t.Parallel()
	capability := mocks.NewMockmigrator(gomock.NewController(t))
	cause, cleanup := errors.New("migration failed"), errors.New("cleanup failed")
	version := uint(2)
	capability.EXPECT().Migrate(gomock.Any(), migrationdomain.Command{Operation: migrationdomain.Down, Steps: 1}).Return(
		migrationdomain.Result{Target: "primary:sqlite", State: &migrationdomain.State{Version: &version, Dirty: true}, Changed: true}, cause)
	cmd := newMigrateCmd(func(context.Context, *zap.Logger) (migrateRuntime, error) {
		return migrateRuntime{migrator: capability, shutdown: func(context.Context) error { return cleanup }}, nil
	})
	var stdout, stderr bytes.Buffer
	cmd.Writer, cmd.ErrWriter = &stdout, &stderr
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	err := cmd.Run(context.Background(), []string{"migrate", "down", "--json"})
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, cleanup)
	require.Empty(t, stdout.String())
	require.Equal(t, 1, bytes.Count(stderr.Bytes(), []byte(`"level":"error"`)))
	require.Contains(t, stderr.String(), `"partial_result":{"target":"primary:sqlite"`)
	require.Contains(t, stderr.String(), `"dirty":true`)
}

func TestMigrationReportsActionableSafeContext(t *testing.T) {
	t.Parallel()
	capability := mocks.NewMockmigrator(gomock.NewController(t))
	cause := &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "target_required", Candidates: []string{"primary:postgres", "primary:sqlite"}}
	capability.EXPECT().Migrate(gomock.Any(), migrationdomain.Command{Operation: migrationdomain.Up}).Return(migrationdomain.Result{}, cause)
	cmd := newMigrateCmd(func(context.Context, *zap.Logger) (migrateRuntime, error) {
		return migrateRuntime{migrator: capability, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout, stderr bytes.Buffer
	cmd.Writer, cmd.ErrWriter = &stdout, &stderr
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	require.ErrorIs(t, cmd.Run(context.Background(), []string{"migrate", "up", "--json"}), cause)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), `"reason":"target_required"`)
	require.Contains(t, stderr.String(), `"targets":["primary:postgres","primary:sqlite"]`)
}

func TestMigrationPartialResultDoesNotInventStateAfterReadFailure(t *testing.T) {
	t.Parallel()
	capability := mocks.NewMockmigrator(gomock.NewController(t))
	cause := errors.New("version read failed after SQL")
	capability.EXPECT().Migrate(gomock.Any(), migrationdomain.Command{Operation: migrationdomain.Up}).Return(
		migrationdomain.Result{Target: "primary:sqlite", Changed: true}, cause)
	cmd := newMigrateCmd(func(context.Context, *zap.Logger) (migrateRuntime, error) {
		return migrateRuntime{migrator: capability, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout, stderr bytes.Buffer
	cmd.Writer, cmd.ErrWriter = &stdout, &stderr
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	require.ErrorIs(t, cmd.Run(context.Background(), []string{"migrate", "up", "--json"}), cause)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), `"partial_result":{"target":"primary:sqlite"}`)
}
