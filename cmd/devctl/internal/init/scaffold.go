package initcmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	scaffolddomain "github.com/devctllabs/devctl/internal/domain/scaffold"
	"github.com/devctllabs/go-libs/lifecycle"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/scaffold.go -package mocks -typed . scaffolder

type scaffolder interface {
	// Scaffold creates or refreshes the generated project foundation.
	Scaffold(ctx context.Context, command scaffolddomain.Command) (scaffolddomain.Result, error)
}

// scaffoldRuntime contains the application port and cleanup hook used by Action.
type scaffoldRuntime struct {
	scaffolder scaffolder
	shutdown   func(context.Context) error
}

// scaffoldBuilder isolates dependency construction from scaffold behavior.
type scaffoldBuilder func(context.Context, *zap.Logger) (scaffoldRuntime, error)

// scaffoldCmd owns parsed options and the runtime factory for one invocation.
type scaffoldCmd struct {
	opts         scaffoldCmdOpts
	buildRuntime scaffoldBuilder
}

// scaffoldCmdOpts receives common scaffold flags.
type scaffoldCmdOpts struct {
	commandruntime.CommonCmdOpts
}

// newScaffoldCmd constructs the executable init scaffold leaf.
func newScaffoldCmd(opts scaffoldCmdOpts, build scaffoldBuilder) *cli.Command {
	cmd := &scaffoldCmd{opts: opts, buildRuntime: build}
	return &cli.Command{
		Name:        "scaffold",
		Usage:       "Create or refresh the Go project foundation",
		Description: "Publish Devctl-managed project files and create missing Scaffold Seeds. Managed Outputs may be replaced; existing user-owned Seeds are never deliberately overwritten or deleted.",
		UsageText:   "devctl init scaffold [--file <path>]",
		Flags:       cmd.opts.CommonFlags(),
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action scaffolds the selected project and emits every resulting file change.
func (cmd *scaffoldCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("init scaffold accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.scaffolder.Scaffold(ctx, scaffolddomain.Command{ManifestPath: cmd.opts.ManifestPath})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	dto := scaffoldDTO(result)
	if finalErr := errors.Join(operationErr, shutdownErr); finalErr != nil {
		var options []commandruntime.ErrorOption
		if len(result.Files) > 0 {
			options = append(options, commandruntime.WithPartialResult(dto))
		}
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("project scaffolding completed", zap.Any("data", dto))
	return nil
}

// scaffoldFileDTO is one stable generated-file change fact.
type scaffoldFileDTO struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// scaffoldResultDTO contains every file considered by the scaffold operation.
type scaffoldResultDTO struct {
	Files []scaffoldFileDTO `json:"files"`
}

// scaffoldDTO converts the domain result into the stable CLI payload.
func scaffoldDTO(result scaffolddomain.Result) scaffoldResultDTO {
	files := make([]scaffoldFileDTO, 0, len(result.Files))
	for _, file := range result.Files {
		files = append(files, scaffoldFileDTO{Path: file.Path, Action: string(file.Action)})
	}
	return scaffoldResultDTO{Files: files}
}

// buildScaffold constructs and resolves the lazy dependencies owned by init scaffold.
func buildScaffold(ctx context.Context, logger *zap.Logger) (scaffoldRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return scaffoldRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ScaffoldService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return scaffoldRuntime{}, errors.Join(fmt.Errorf("container.ScaffoldService: %w", err), shutdownErr)
	}
	return scaffoldRuntime{scaffolder: service, shutdown: container.Shutdown}, nil
}
