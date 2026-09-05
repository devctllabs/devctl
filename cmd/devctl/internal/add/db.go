package add

import (
	"context"
	"errors"
	"fmt"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/db.go -package mocks -typed . databaseAdder

type databaseAdder interface {
	// AddDB adds or updates one named database variant.
	AddDB(ctx context.Context, command projectdomain.AddDBCommand) (projectdomain.ManifestResult, error)
}

// dbRuntime contains the application port and cleanup hook used by Action.
type dbRuntime struct {
	adder    databaseAdder
	shutdown func(context.Context) error
}

// dbBuilder isolates dependency construction from database mutation behavior.
type dbBuilder func(context.Context, *zap.Logger) (dbRuntime, error)

// dbCmd owns parsed database options and the runtime factory.
type dbCmd struct {
	opts         dbCmdOpts
	buildRuntime dbBuilder
}

// dbCmdOpts receives the positional name, common flags, and database settings.
type dbCmdOpts struct {
	commandruntime.CommonCmdOpts
	Name           string
	Kind           string
	Default        bool
	NoMigrations   bool
	MigrationsPath string
	Force          bool
}

// newDBCmd constructs the executable add db leaf.
func newDBCmd(opts dbCmdOpts, build dbBuilder) *cli.Command {
	cmd := &dbCmd{opts: opts, buildRuntime: build}
	noMigrationsFlag := &cli.BoolFlag{Name: "no-migrations", Usage: "do not declare a migration target for this Variant", Destination: &cmd.opts.NoMigrations}
	migrationsPathFlag := &cli.StringFlag{Name: "migrations-path", Usage: "override the project-relative migration `path`", Destination: &cmd.opts.MigrationsPath}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "kind", Usage: "select `sqlite`, `postgres`, or `clickhouse`", Destination: &cmd.opts.Kind},
		&cli.BoolFlag{Name: "default", Usage: "make this Variant the Connection default", Destination: &cmd.opts.Default},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing Variant with the same identity", Destination: &cmd.opts.Force},
	)
	return &cli.Command{
		Name:                   "db",
		Usage:                  "Add a database variant",
		Description:            "Add a SQLite, PostgreSQL, or ClickHouse Variant to a named database Connection. A migration target is declared by default; Devctl never writes SQL or applies migrations.",
		UsageText:              "devctl add db <database-name> --kind <sqlite|postgres|clickhouse>",
		UseShortOptionHandling: true,
		Arguments: []cli.Argument{&cli.StringArg{
			Name: "database-name", UsageText: "<database-name>", Destination: &cmd.opts.Name,
		}},
		Flags:                  flags,
		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{{Flags: [][]cli.Flag{{noMigrationsFlag}, {migrationsPathFlag}}}},
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action adds one named database variant to the selected manifest.
func (cmd *dbCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if cmd.opts.Name == "" {
		return reporter.ReportError(cli.Exit("database name is required", 2))
	}
	if cmd.opts.Kind == "" {
		return reporter.ReportError(cli.Exit("--kind is required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.adder.AddDB(ctx, projectdomain.AddDBCommand{
		ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Kind: cmd.opts.Kind,
		Default: cmd.opts.Default, NoMigrations: cmd.opts.NoMigrations,
		MigrationsPath: cmd.opts.MigrationsPath, Force: cmd.opts.Force,
	})
	return finishManifestAddition(ctx, manifestAddition{
		stdout: stdout, reporter: reporter, shutdown: runtime.shutdown,
		result: result, operationErr: operationErr,
	})
}

// buildDB constructs and resolves the lazy dependencies owned by add db.
func buildDB(ctx context.Context, logger *zap.Logger) (dbRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return dbRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return dbRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), shutdownErr)
	}
	return dbRuntime{adder: service, shutdown: container.Shutdown}, nil
}
