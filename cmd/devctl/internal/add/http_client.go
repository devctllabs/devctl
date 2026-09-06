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

//go:generate go tool mockgen -destination mocks/http_client.go -package mocks -typed . httpClientAdder

type httpClientAdder interface {
	// AddHTTPClient adds or updates one named generated HTTP client.
	AddHTTPClient(ctx context.Context, command projectdomain.AddHTTPClientCommand) (projectdomain.ManifestResult, error)
}

// httpClientRuntime contains the application port and cleanup hook used by Action.
type httpClientRuntime struct {
	adder    httpClientAdder
	shutdown func(context.Context) error
}

// httpClientBuilder isolates dependency construction from HTTP client mutation behavior.
type httpClientBuilder func(context.Context, *zap.Logger) (httpClientRuntime, error)

// httpClientCmd owns parsed client options and the runtime factory.
type httpClientCmd struct {
	opts         httpClientCmdOpts
	buildRuntime httpClientBuilder
}

// httpClientCmdOpts receives the positional name, common flags, and contract selection.
type httpClientCmdOpts struct {
	commandruntime.CommonCmdOpts
	Name       string
	Source     string
	Export     string
	Path       string
	BaseURLEnv string
	Force      bool
}

// newHTTPClientCmd constructs the executable add http-client leaf.
func newHTTPClientCmd(opts httpClientCmdOpts, build httpClientBuilder) *cli.Command {
	cmd := &httpClientCmd{opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "source", Usage: "select the named contract `source`", Destination: &cmd.opts.Source},
		&cli.StringFlag{Name: "export", Usage: "select a named Export from a Devctl Source", Destination: &cmd.opts.Export},
		&cli.StringFlag{Name: "path", Usage: "select an OpenAPI Entrypoint from a non-Devctl Source", Destination: &cmd.opts.Path},
		&cli.StringFlag{Name: "base-url-env", Usage: "override the generated runtime base URL environment `key`", Destination: &cmd.opts.BaseURLEnv},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing HTTP client with the same name", Destination: &cmd.opts.Force},
	)
	return &cli.Command{
		Name:                   "http-client",
		Usage:                  "Add an HTTP client",
		Description:            "Declare a named OpenAPI client Target. Use --path with ordinary Sources or --export with a Devctl Source.",
		UsageText:              "devctl add http-client <http-client-name> --source <source> (--path <entrypoint> | --export <export>)",
		UseShortOptionHandling: true,
		Arguments: []cli.Argument{&cli.StringArg{
			Name: "http-client-name", UsageText: "<http-client-name>", Destination: &cmd.opts.Name,
		}},
		Flags: flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action adds one named HTTP client to the selected manifest.
func (cmd *httpClientCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if cmd.opts.Name == "" {
		return reporter.ReportError(cli.Exit("HTTP client name is required", 2))
	}
	if cmd.opts.Source == "" {
		return reporter.ReportError(cli.Exit("--source is required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.adder.AddHTTPClient(ctx, projectdomain.AddHTTPClientCommand{
		ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Source: cmd.opts.Source,
		Export: cmd.opts.Export, Path: cmd.opts.Path, BaseURLEnv: cmd.opts.BaseURLEnv, Force: cmd.opts.Force,
	})
	return finishManifestAddition(ctx, manifestAddition{
		stdout: stdout, reporter: reporter, shutdown: runtime.shutdown,
		result: result, operationErr: operationErr,
	})
}

// buildHTTPClient constructs and resolves the lazy dependencies owned by add http-client.
func buildHTTPClient(ctx context.Context, logger *zap.Logger) (httpClientRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return httpClientRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return httpClientRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), shutdownErr)
	}
	return httpClientRuntime{adder: service, shutdown: container.Shutdown}, nil
}
