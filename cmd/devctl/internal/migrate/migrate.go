package migrate

import (
	"context"
	"errors"
	"fmt"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/migrate.go -package mocks -typed . migrator

type migrator interface {
	// Migrate executes one operation on a selected Migration Target.
	Migrate(ctx context.Context, command migrationdomain.Command) (migrationdomain.Result, error)
}

type migrateRuntime struct {
	migrator migrator
	shutdown func(context.Context) error
}

type migrateBuilder func(ctx context.Context, logger *zap.Logger) (migrateRuntime, error)

// NewCmd constructs the migration command namespace and its lazy executable leaves.
func NewCmd() *cli.Command { return newMigrateCmd(buildMigrate) }

func newMigrateCmd(build migrateBuilder) *cli.Command {
	return &cli.Command{
		Name:        "migrate",
		Usage:       "Create and run database migrations",
		Description: "Operate on one Manifest Migration Target. Select connection:variant explicitly, or omit --target when exactly one Migration Target exists.",
		Commands:    []*cli.Command{newCreateCmd(build), newUpCmd(build), newDownCmd(build), newStatusCmd(build)},
	}
}

func buildMigrate(ctx context.Context, logger *zap.Logger) (migrateRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return migrateRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.MigrationService()
	if err != nil {
		return migrateRuntime{}, errors.Join(fmt.Errorf("container.MigrationService: %w", err), commandruntime.Shutdown(ctx, container))
	}
	return migrateRuntime{migrator: service, shutdown: container.Shutdown}, nil
}
