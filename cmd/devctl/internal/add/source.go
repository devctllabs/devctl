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

//go:generate go tool mockgen -destination mocks/source.go -package mocks -typed . sourceAdder

type sourceAdder interface {
	// AddSource adds or updates one named contract source.
	AddSource(ctx context.Context, command projectdomain.AddSourceCommand) (projectdomain.ManifestResult, error)
}

// sourceRuntime contains the application port and cleanup hook used by Action.
type sourceRuntime struct {
	adder    sourceAdder
	shutdown func(context.Context) error
}

// sourceBuilder isolates dependency construction from source mutation behavior.
type sourceBuilder func(context.Context, *zap.Logger) (sourceRuntime, error)

// sourceCmd owns parsed source options and the runtime factory.
type sourceCmd struct {
	opts         sourceCmdOpts
	buildRuntime sourceBuilder
}

// sourceCmdOpts receives the positional name, common flags, and source location settings.
type sourceCmdOpts struct {
	commandruntime.CommonCmdOpts
	Name              string
	Type              string
	Path              string
	URL               string
	Filename          string
	AllowInsecureHTTP bool
	Repo              string
	Ref               string
	BufConfig         string
	Force             bool
}

// newSourceCmd constructs the executable add source leaf.
func newSourceCmd(opts sourceCmdOpts, build sourceBuilder) *cli.Command {
	cmd := &sourceCmd{opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "type", Usage: "select `local`, `url`, `git`, or `devctl`", Destination: &cmd.opts.Type},
		&cli.StringFlag{Name: "path", Usage: "set the project-relative local or Git containment `path`", Destination: &cmd.opts.Path},
		&cli.StringFlag{Name: "url", Usage: "set the base `URL` for a URL Source", Destination: &cmd.opts.URL},
		&cli.StringFlag{Name: "filename", Usage: "store a single URL document under `filename`", Destination: &cmd.opts.Filename},
		&cli.BoolFlag{Name: "allow-insecure-http", Usage: "allow an http URL instead of requiring https", Destination: &cmd.opts.AllowInsecureHTTP},
		&cli.StringFlag{Name: "repo", Usage: "set the Git or Devctl repository `location`", Destination: &cmd.opts.Repo},
		&cli.StringFlag{Name: "ref", Usage: "select the immutable or reviewable repository `ref`", Destination: &cmd.opts.Ref},
		&cli.StringFlag{Name: "buf-config", Usage: "select the Source-relative supplier `buf-config`", Destination: &cmd.opts.BufConfig},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing Source with the same name", Destination: &cmd.opts.Force},
	)
	return &cli.Command{
		Name:                   "source",
		Usage:                  "Add a contract source",
		Description:            "Declare a bounded origin for Contracts. Type-specific flags select a local directory, URL closure, Git checkout, or another Devctl Project.",
		UsageText:              "devctl add source <source-name> --type <local|url|git|devctl> [type-specific flags]",
		UseShortOptionHandling: true,
		Arguments: []cli.Argument{&cli.StringArg{
			Name: "source-name", UsageText: "<source-name>", Destination: &cmd.opts.Name,
		}},
		Flags: flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action adds one named contract source to the selected manifest.
func (cmd *sourceCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if cmd.opts.Name == "" {
		return reporter.ReportError(cli.Exit("source name is required", 2))
	}
	if cmd.opts.Type == "" {
		return reporter.ReportError(cli.Exit("--type is required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.adder.AddSource(ctx, projectdomain.AddSourceCommand{
		ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Type: cmd.opts.Type,
		Path: cmd.opts.Path, URL: cmd.opts.URL, Filename: cmd.opts.Filename,
		AllowInsecureHTTP: cmd.opts.AllowInsecureHTTP, Repo: cmd.opts.Repo, Ref: cmd.opts.Ref, BufConfig: cmd.opts.BufConfig, Force: cmd.opts.Force,
	})
	return finishManifestAddition(ctx, manifestAddition{
		stdout: stdout, reporter: reporter, shutdown: runtime.shutdown,
		result: result, operationErr: operationErr,
	})
}

// buildSource constructs and resolves the lazy dependencies owned by add source.
func buildSource(ctx context.Context, logger *zap.Logger) (sourceRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return sourceRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return sourceRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), shutdownErr)
	}
	return sourceRuntime{adder: service, shutdown: container.Shutdown}, nil
}
