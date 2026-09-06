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

type grpcClientAdder interface {
	// AddGRPCClient adds or updates the gRPC client described by command.
	AddGRPCClient(ctx context.Context, command projectdomain.AddGRPCClientCommand) (projectdomain.ManifestResult, error)
}

type grpcClientRuntime struct {
	adder    grpcClientAdder
	shutdown func(context.Context) error
}

type grpcClientBuilder func(context.Context, *zap.Logger) (grpcClientRuntime, error)

type grpcClientCmdOpts struct {
	commandruntime.CommonCmdOpts
	Name         string
	Source       string
	Export       string
	Path         string
	ProtoRoot    string
	BufGenConfig string
	AddrEnv      string
	Force        bool
}

type grpcClientCmd struct {
	opts         grpcClientCmdOpts
	buildRuntime grpcClientBuilder
}

func newGRPCClientCmd(opts grpcClientCmdOpts, build grpcClientBuilder) *cli.Command {
	cmd := &grpcClientCmd{opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "source", Usage: "select the named contract `source`", Destination: &cmd.opts.Source},
		&cli.StringFlag{Name: "export", Usage: "select a named Export from a Devctl Source", Destination: &cmd.opts.Export},
		&cli.StringFlag{Name: "path", Usage: "select the Contract path from a non-Devctl Source", Destination: &cmd.opts.Path},
		&cli.StringFlag{Name: "proto-root", Usage: "set the Source-relative Proto `module-root`", Destination: &cmd.opts.ProtoRoot},
		&cli.StringFlag{Name: "buf-gen-config", Usage: "use the project-owned generator `config-path`", Destination: &cmd.opts.BufGenConfig},
		&cli.StringFlag{Name: "addr-env", Usage: "override the generated runtime address environment `key`", Destination: &cmd.opts.AddrEnv},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing gRPC client with the same name", Destination: &cmd.opts.Force},
	)
	return &cli.Command{
		Name:        "grpc-client",
		Usage:       "Add a gRPC client",
		Description: "Declare a named Proto client Target. Use --path with ordinary Sources or --export with a Devctl Source; custom generator configs remain user-owned.",
		UsageText:   "devctl add grpc-client <grpc-client-name> --source <source> (--path <path> | --export <export>)",
		Arguments: []cli.Argument{&cli.StringArg{
			Name: "grpc-client-name", UsageText: "<grpc-client-name>", Destination: &cmd.opts.Name,
		}},
		Flags:  flags,
		Action: cmd.Action,
	}
}

func (cmd *grpcClientCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if cmd.opts.Name == "" {
		return reporter.ReportError(cli.Exit("gRPC client name is required", 2))
	}
	if cmd.opts.Source == "" {
		return reporter.ReportError(cli.Exit("--source is required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.adder.AddGRPCClient(ctx, projectdomain.AddGRPCClientCommand{
		ManifestPath: cmd.opts.ManifestPath,
		Name:         cmd.opts.Name,
		Source:       cmd.opts.Source,
		Export:       cmd.opts.Export,
		Path:         cmd.opts.Path,
		ProtoRoot:    cmd.opts.ProtoRoot,
		BufGenConfig: cmd.opts.BufGenConfig,
		AddrEnv:      cmd.opts.AddrEnv,
		Force:        cmd.opts.Force,
	})
	return finishManifestAddition(ctx, manifestAddition{
		stdout: stdout, reporter: reporter, shutdown: runtime.shutdown,
		result: result, operationErr: operationErr,
	})
}

func buildGRPCClient(ctx context.Context, logger *zap.Logger) (grpcClientRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return grpcClientRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		return grpcClientRuntime{}, errors.Join(
			fmt.Errorf("container.ProjectService: %w", err),
			commandruntime.Shutdown(ctx, container),
		)
	}
	return grpcClientRuntime{adder: service, shutdown: container.Shutdown}, nil
}
