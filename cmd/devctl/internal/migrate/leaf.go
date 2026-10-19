package migrate

import (
	"context"
	"errors"
	"strings"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/devctllabs/go-libs/lifecycle"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

type migrateCmdOpts struct {
	commandruntime.CommonCmdOpts
	target  string
	envFile string
	name    string
	steps   int
}

type migrateCmd struct {
	opts         migrateCmdOpts
	operation    migrationdomain.Operation
	buildRuntime migrateBuilder
}

func newLeaf(operation migrationdomain.Operation, usage string, build migrateBuilder) (*cli.Command, *migrateCmd) {
	cmd := &migrateCmd{operation: operation, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(), &cli.StringFlag{
		Name: "target", Usage: "select Migration Target `connection:variant`", Destination: &cmd.opts.target,
	})
	if operation != migrationdomain.Create {
		flags = append(flags, &cli.StringFlag{Name: "env-file", Usage: "read migration URL from this dotenv `path` after process environment", Destination: &cmd.opts.envFile})
	}
	return &cli.Command{
		Name: string(operation), Usage: usage, Description: usage + ". Migration SQL is selected by migrations.path in the Manifest.", Flags: flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			return cmd.reporter(command).ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}, cmd
}

func (cmd *migrateCmd) reporter(command *cli.Command) *commandruntime.ErrorReporter {
	return commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
}

func (cmd *migrateCmd) validate(command *cli.Command) error {
	if command.Args().Len() != 0 {
		return cli.Exit("unexpected positional arguments", 2)
	}
	if command.IsSet("target") {
		connection, variant, found := strings.Cut(cmd.opts.target, ":")
		if !found || connection == "" || variant == "" || strings.Contains(variant, ":") {
			return cli.Exit("--target must be connection:variant", 2)
		}
	}
	if command.IsSet("env-file") && cmd.opts.envFile == "" {
		return cli.Exit("--env-file must not be empty", 2)
	}
	if cmd.operation == migrationdomain.Create && !migrationdomain.ValidName(cmd.opts.name) {
		return cli.Exit("migration name must start with an ASCII letter or digit and contain only ASCII letters, digits, _ or -", 2)
	}
	if (cmd.operation == migrationdomain.Up && command.IsSet("steps") && cmd.opts.steps <= 0) ||
		(cmd.operation == migrationdomain.Down && cmd.opts.steps <= 0) {
		return cli.Exit("--steps must be positive", 2)
	}
	return nil
}

// Action executes one migration capability after parsing and validating command input.
func (cmd *migrateCmd) Action(ctx context.Context, command *cli.Command) error {
	reporter := cmd.reporter(command)
	if err := cmd.validate(command); err != nil {
		return reporter.ReportError(err)
	}
	runtime, err := cmd.buildRuntime(ctx, cmd.opts.NewStderrLogger(command))
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.migrator.Migrate(ctx, migrationdomain.Command{
		ManifestPath: cmd.opts.ManifestPath, Target: cmd.opts.target, Operation: cmd.operation,
		EnvFile: cmd.opts.envFile, Name: cmd.opts.name, Steps: cmd.opts.steps,
	})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	if err := errors.Join(operationErr, shutdownErr); err != nil {
		var options []commandruntime.ErrorOption
		if result.Changed || operationErr == nil {
			options = append(options, commandruntime.WithPartialResult(migrationDTO(result)))
		}
		return reporter.ReportError(err, options...)
	}
	cmd.opts.NewStdoutLogger(command).Info("migration "+string(cmd.operation)+" completed", zap.Any("data", migrationDTO(result)))
	return nil
}

type stateDTO struct {
	Target   string `json:"target"`
	Version  *uint  `json:"version"`
	Dirty    bool   `json:"dirty"`
	NoChange bool   `json:"no_change,omitempty"`
}

type createDTO struct {
	Target string   `json:"target"`
	Files  []string `json:"files,omitempty"`
}

func migrationDTO(result migrationdomain.Result) any {
	if result.State == nil {
		return createDTO{Target: result.Target, Files: result.Files}
	}
	return stateDTO{Target: result.Target, Version: result.State.Version, Dirty: result.State.Dirty, NoChange: result.NoChange}
}
