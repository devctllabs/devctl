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

type storageAdder interface {
	// AddRedis adds or updates the Redis connection described by command.
	AddRedis(ctx context.Context, command projectdomain.AddRedisCommand) (projectdomain.ManifestResult, error)
	// AddS3Connection adds or updates the S3 connection described by command.
	AddS3Connection(ctx context.Context, command projectdomain.AddS3ConnectionCommand) (projectdomain.ManifestResult, error)
	// AddS3 adds or updates the S3 bucket described by command.
	AddS3(ctx context.Context, command projectdomain.AddS3Command) (projectdomain.ManifestResult, error)
}

type storageRuntime struct {
	adder    storageAdder
	shutdown func(context.Context) error
}

type storageBuilder func(context.Context, *zap.Logger) (storageRuntime, error)

type storageCmdOpts struct {
	commandruntime.CommonCmdOpts
	Name        string
	AddrEnv     string
	AddrDefault string
	Connection  string
	Credentials string
	Force       bool
}

type storageCmd struct {
	kind         string
	opts         storageCmdOpts
	buildRuntime storageBuilder
}

func newRedisCmd(opts storageCmdOpts, build storageBuilder) *cli.Command {
	cmd := &storageCmd{kind: "redis", opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "addr-env", Usage: "override the generated Redis address environment `key`", Destination: &cmd.opts.AddrEnv},
		&cli.StringFlag{Name: "addr-default", Usage: "override the local Redis `address` default", Destination: &cmd.opts.AddrDefault},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing Redis Connection with the same name", Destination: &cmd.opts.Force},
	)
	return newStorageLeaf(cmd, flags)
}

func newS3ConnectionCmd(opts storageCmdOpts, build storageBuilder) *cli.Command {
	cmd := &storageCmd{kind: "s3-connection", opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "credentials", Usage: "select `ambient` or `static` credentials", Destination: &cmd.opts.Credentials},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing S3 Connection with the same name", Destination: &cmd.opts.Force},
	)
	return newStorageLeaf(cmd, flags)
}

func newS3Cmd(opts storageCmdOpts, build storageBuilder) *cli.Command {
	cmd := &storageCmd{kind: "s3", opts: opts, buildRuntime: build}
	flags := append(cmd.opts.CommonFlags(),
		&cli.StringFlag{Name: "connection", Usage: "attach the bucket to the named S3 `connection`", Destination: &cmd.opts.Connection},
		&cli.BoolFlag{Name: "force", Usage: "replace an existing S3 bucket with the same name", Destination: &cmd.opts.Force},
	)
	return newStorageLeaf(cmd, flags)
}

func newStorageLeaf(cmd *storageCmd, flags []cli.Flag) *cli.Command {
	descriptions := map[string]string{
		"redis":         "Declare a named Redis Connection with an environment-backed address and a local default.",
		"s3-connection": "Declare a named S3 Connection and choose ambient or static credentials.",
		"s3":            "Declare a named S3 bucket attached to an existing Connection, or create the canonical local Connection when omitted.",
	}
	return &cli.Command{
		Name:        cmd.kind,
		Usage:       "Add a " + cmd.kind + " resource",
		Description: descriptions[cmd.kind],
		UsageText:   "devctl add " + cmd.kind + " <" + cmd.kind + "-name> [options]",
		Arguments: []cli.Argument{&cli.StringArg{
			Name: cmd.kind + "-name", UsageText: "<" + cmd.kind + "-name>", Destination: &cmd.opts.Name,
		}},
		Flags: flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

func (cmd *storageCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if cmd.opts.Name == "" {
		return reporter.ReportError(cli.Exit(cmd.kind+" name is required", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := cmd.add(ctx, runtime.adder)
	return finishManifestAddition(ctx, manifestAddition{
		stdout: stdout, reporter: reporter, shutdown: runtime.shutdown,
		result: result, operationErr: operationErr,
	})
}

func (cmd *storageCmd) add(ctx context.Context, adder storageAdder) (projectdomain.ManifestResult, error) {
	var result projectdomain.ManifestResult
	var err error
	operation := "AddS3"
	switch cmd.kind {
	case "redis":
		operation = "AddRedis"
		result, err = adder.AddRedis(ctx, projectdomain.AddRedisCommand{ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, AddrEnv: cmd.opts.AddrEnv, AddrDefault: cmd.opts.AddrDefault, Force: cmd.opts.Force})
	case "s3-connection":
		operation = "AddS3Connection"
		result, err = adder.AddS3Connection(ctx, projectdomain.AddS3ConnectionCommand{ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Credentials: cmd.opts.Credentials, Force: cmd.opts.Force})
	default:
		result, err = adder.AddS3(ctx, projectdomain.AddS3Command{ManifestPath: cmd.opts.ManifestPath, Name: cmd.opts.Name, Connection: cmd.opts.Connection, Force: cmd.opts.Force})
	}
	if err != nil {
		return result, fmt.Errorf("adder.%s: %w", operation, err)
	}
	return result, nil
}

func buildStorage(ctx context.Context, logger *zap.Logger) (storageRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return storageRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		return storageRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), commandruntime.Shutdown(ctx, container))
	}
	return storageRuntime{adder: service, shutdown: container.Shutdown}, nil
}
