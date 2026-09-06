package enable

import (
	"context"
	"errors"
	"fmt"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/go-libs/lifecycle"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/capability.go -package mocks -typed . capabilityEnabler

type capabilityEnabler interface {
	// Enable adds or updates one project capability configuration.
	Enable(ctx context.Context, command projectdomain.EnableCommand) (projectdomain.ManifestResult, error)
}

// capabilityRuntime contains the application port and cleanup hook used by Action.
type capabilityRuntime struct {
	enabler  capabilityEnabler
	shutdown func(context.Context) error
}

// capabilityBuilder isolates dependency construction from capability mutation behavior.
type capabilityBuilder func(context.Context, *zap.Logger) (capabilityRuntime, error)

// capabilityCmd owns parsed options, the selected capability, and its runtime factory.
type capabilityCmd struct {
	opts         capabilityCmdOpts
	capability   string
	buildRuntime capabilityBuilder
}

// capabilityCmdOpts receives common flags and capability mutation policies.
type capabilityCmdOpts struct {
	commandruntime.CommonCmdOpts
	Always bool
	Force  bool
}

// newCapabilityCmd constructs one executable leaf for a fixed capability name.
func newCapabilityCmd(name string, opts capabilityCmdOpts, build capabilityBuilder) *cli.Command {
	cmd := &capabilityCmd{opts: opts, capability: name, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.BoolFlag{Name: "always", Usage: "omit the Runtime Start Policy so the Capability always starts", Destination: &cmd.opts.Always},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing Capability declaration", Destination: &cmd.opts.Force},
	)
	return &cli.Command{
		Name:        name,
		Usage:       "Enable " + name,
		Description: "Add the " + name + " Capability and its canonical defaults to the Manifest. Run init scaffold and gen explicitly when the resulting Project files need to be refreshed.",
		UsageText:   "devctl enable " + name + " [--always] [--force]",
		Flags:       flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action enables one project capability and emits the resulting manifest change.
func (cmd *capabilityCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("enable accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.enabler.Enable(ctx, projectdomain.EnableCommand{
		ManifestPath: cmd.opts.ManifestPath,
		Capability:   cmd.capability,
		Always:       cmd.opts.Always,
		Force:        cmd.opts.Force,
	})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	dto := manifestResultDTO{Manifest: result.Manifest, Change: string(result.Change)}
	var options []commandruntime.ErrorOption
	if result.Change != "" {
		options = append(options, commandruntime.WithPartialResult(dto))
	}
	if finalErr := errors.Join(operationErr, shutdownErr); finalErr != nil {
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("capability enablement completed", zap.Any("data", dto))
	return nil
}

// manifestResultDTO is the stable capability mutation payload.
type manifestResultDTO struct {
	Manifest string `json:"manifest"`
	Change   string `json:"change"`
}

// buildCapability constructs and resolves the lazy dependencies owned by enable leaves.
func buildCapability(ctx context.Context, logger *zap.Logger) (capabilityRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return capabilityRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return capabilityRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), shutdownErr)
	}
	return capabilityRuntime{enabler: service, shutdown: container.Shutdown}, nil
}
