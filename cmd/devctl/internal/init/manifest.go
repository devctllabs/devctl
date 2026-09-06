package initcmd

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

//go:generate go tool mockgen -destination mocks/manifest.go -package mocks -typed . manifestInitializer

type manifestInitializer interface {
	// InitManifest creates or updates the canonical project manifest.
	InitManifest(ctx context.Context, command projectdomain.InitManifestCommand) (projectdomain.ManifestResult, error)
}

// manifestRuntime contains the application port and cleanup hook used by Action.
type manifestRuntime struct {
	initializer manifestInitializer
	shutdown    func(context.Context) error
}

// manifestBuilder isolates dependency construction from manifest initialization.
type manifestBuilder func(context.Context, *zap.Logger) (manifestRuntime, error)

// manifestCmd owns parsed options and the runtime factory for one invocation.
type manifestCmd struct {
	opts         manifestCmdOpts
	buildRuntime manifestBuilder
}

// manifestCmdOpts receives common flags plus the manifest identity and overwrite policy.
type manifestCmdOpts struct {
	commandruntime.CommonCmdOpts
	Language string
	Preset   string
	Name     string
	Module   string
	Force    bool
}

// newManifestCmd constructs the executable init manifest leaf.
func newManifestCmd(opts manifestCmdOpts, build manifestBuilder) *cli.Command {
	cmd := &manifestCmd{opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "lang", Usage: "set the project language; supported value: `go`", Destination: &cmd.opts.Language},
		&cli.StringFlag{Name: "preset", Usage: "seed the Manifest from `cli` or `http-service`", Destination: &cmd.opts.Preset},
		&cli.StringFlag{Name: "name", Usage: "set the kebab-case `project-name`", Destination: &cmd.opts.Name},
		&cli.StringFlag{Name: "module", Usage: "set the Go `module-path`", Destination: &cmd.opts.Module},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing Manifest instead of returning a conflict", Destination: &cmd.opts.Force},
	)
	return &cli.Command{
		Name:        "manifest",
		Usage:       "Create devctl.yaml",
		Description: "Create a complete v1 Manifest from a supported preset. This command writes only the Manifest; it does not scaffold files, install tools, synchronize Contracts, lint, or generate code.",
		UsageText:   "devctl init manifest --lang go --preset <cli|http-service> --name <project-name> --module <module-path>",
		Flags:       flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action creates the selected project manifest and emits the resulting change.
func (cmd *manifestCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("init manifest accepts no positional arguments", 2))
	}
	if cmd.opts.Language == "" || cmd.opts.Preset == "" || cmd.opts.Name == "" || cmd.opts.Module == "" {
		return reporter.ReportError(cli.Exit("--lang, --preset, --name, and --module are required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.initializer.InitManifest(ctx, projectdomain.InitManifestCommand{
		Destination: cmd.opts.ManifestPath,
		Language:    cmd.opts.Language,
		Preset:      cmd.opts.Preset,
		Name:        cmd.opts.Name,
		Module:      cmd.opts.Module,
		Force:       cmd.opts.Force,
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
	stdout.Info("manifest initialization completed", zap.Any("data", dto))
	return nil
}

// manifestResultDTO is the stable manifest mutation payload.
type manifestResultDTO struct {
	Manifest string `json:"manifest"`
	Change   string `json:"change"`
}

// buildManifest constructs and resolves the lazy dependencies owned by init manifest.
func buildManifest(ctx context.Context, logger *zap.Logger) (manifestRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return manifestRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return manifestRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), shutdownErr)
	}
	return manifestRuntime{initializer: service, shutdown: container.Shutdown}, nil
}
