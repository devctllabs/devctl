package inspect

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

//go:generate go tool mockgen -destination mocks/inspect.go -package mocks -typed . projectInspector

type projectInspector interface {
	// Inspect returns the effective project view selected by query.
	Inspect(ctx context.Context, query projectdomain.InspectQuery) (projectdomain.InspectResult, error)
}

// inspectRuntime contains the application port and cleanup hook used by Action.
type inspectRuntime struct {
	inspector projectInspector
	shutdown  func(context.Context) error
}

// inspectBuilder isolates dependency construction from command behavior.
type inspectBuilder func(context.Context, *zap.Logger) (inspectRuntime, error)

// inspectCmd owns parsed options and the runtime factory for one invocation.
type inspectCmd struct {
	opts         inspectCmdOpts
	buildRuntime inspectBuilder
}

// inspectCmdOpts receives the common leaf flags bound by urfave/cli.
type inspectCmdOpts struct {
	commandruntime.CommonCmdOpts
}

// inspectProjectDTO is the stable effective-project payload emitted by inspect.
type inspectProjectDTO struct {
	Root         string              `json:"root"`
	ManifestPath string              `json:"manifest_path"`
	Name         string              `json:"name"`
	Language     string              `json:"language"`
	Module       string              `json:"module"`
	EnvPrefix    string              `json:"env_prefix"`
	Paths        inspectPathsDTO     `json:"paths"`
	Targets      []inspectTargetDTO  `json:"targets"`
	Env          []inspectEnvDTO     `json:"env"`
	Resources    inspectResourcesDTO `json:"resources"`
}

type inspectTargetDTO struct {
	ID            string `json:"id"`
	Family        string `json:"family"`
	Format        string `json:"format"`
	Input         string `json:"input,omitempty"`
	ResolvedInput string `json:"resolved_input,omitempty"`
	Config        string `json:"config,omitempty"`
	Output        string `json:"output,omitempty"`
}

type inspectEnvDTO struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Default any    `json:"default,omitempty"`
	Secret  bool   `json:"secret,omitempty"`
}

type inspectResourcesDTO struct {
	DBConnections    []string              `json:"db_connections"`
	RedisConnections []string              `json:"redis_connections"`
	S3Connections    []string              `json:"s3_connections"`
	S3Buckets        []string              `json:"s3_buckets"`
	Migrations       []inspectMigrationDTO `json:"migrations"`
}

type inspectMigrationDTO struct {
	Connection  string `json:"connection"`
	Variant     string `json:"variant"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	DatabaseEnv string `json:"database_env"`
}

// inspectPathsDTO contains effective project-relative output locations.
type inspectPathsDTO struct {
	ExternalContracts string `json:"external_contracts"`
	ConfigOut         string `json:"config_out"`
	ServerOut         string `json:"server_out"`
	ClientOut         string `json:"client_out"`
}

// inspectResultDTO wraps the effective project under the public data schema.
type inspectResultDTO struct {
	Project inspectProjectDTO `json:"project"`
}

// NewCmd constructs the inspect command.
func NewCmd() *cli.Command {
	return newInspectCmd(inspectCmdOpts{}, buildInspect)
}

func newInspectCmd(opts inspectCmdOpts, build inspectBuilder) *cli.Command {
	cmd := &inspectCmd{opts: opts, buildRuntime: build}
	return &cli.Command{
		Name:        "inspect",
		Usage:       "Inspect effective Project configuration",
		Description: "Show the selected Project root, effective paths, Target Catalog, Runtime Config, Resources, and resolved Contract inputs without requiring every external Snapshot to be ready.",
		UsageText:   "devctl inspect [--file <path>]",
		Flags:       cmd.opts.CommonFlags(),
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action inspects the selected project and emits its effective configuration.
func (cmd *inspectCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("inspect accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, inspectErr := runtime.inspector.Inspect(ctx, projectdomain.InspectQuery{ManifestPath: cmd.opts.ManifestPath})
	shutdownErr := runtime.shutdown(ctx)
	dto := inspectDTO(result)
	var options []commandruntime.ErrorOption
	if inspectErr == nil {
		options = append(options, commandruntime.WithPartialResult(dto))
	}
	if finalErr := errors.Join(inspectErr, shutdownErr); finalErr != nil {
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("project inspection completed", zap.Any("data", dto))
	return nil
}

// inspectDTO converts a domain inspection into the stable CLI payload.
func inspectDTO(result projectdomain.InspectResult) inspectResultDTO {
	project := result.Project
	targets := make([]inspectTargetDTO, len(project.Targets))
	for index, target := range project.Targets {
		targets[index] = inspectTargetDTO{
			ID: target.ID, Family: target.Family, Format: target.Format,
			Input: target.Input, ResolvedInput: target.ResolvedInput,
			Config: target.Config, Output: target.Output,
		}
	}
	env := make([]inspectEnvDTO, len(project.Env))
	for index, entry := range project.Env {
		env[index] = inspectEnvDTO{Key: entry.Key, Type: entry.Type, Default: entry.Default, Secret: entry.Secret}
		if entry.Secret {
			env[index].Default = nil
		}
	}
	migrations := make([]inspectMigrationDTO, len(project.Resources.Migrations))
	for index, migration := range project.Resources.Migrations {
		migrations[index] = inspectMigrationDTO{
			Connection: migration.Connection, Variant: migration.Variant, Kind: migration.Kind,
			Path: migration.Path, DatabaseEnv: migration.DatabaseEnv,
		}
	}
	return inspectResultDTO{Project: inspectProjectDTO{
		Root: project.Root, ManifestPath: project.ManifestPath, Name: project.Name,
		Language: project.Language, Module: project.Module, EnvPrefix: project.EnvPrefix,
		Paths: inspectPathsDTO{
			ExternalContracts: project.Paths.ExternalContracts, ConfigOut: project.Paths.ConfigOut,
			ServerOut: project.Paths.ServerOut, ClientOut: project.Paths.ClientOut,
		},
		Targets: targets, Env: env,
		Resources: inspectResourcesDTO{
			DBConnections: project.Resources.DBConnections, RedisConnections: project.Resources.RedisConnections,
			S3Connections: project.Resources.S3Connections, S3Buckets: project.Resources.S3Buckets,
			Migrations: migrations,
		},
	}}
}

// buildInspect constructs and resolves the lazy dependencies owned by inspect.
func buildInspect(ctx context.Context, logger *zap.Logger) (inspectRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return inspectRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return inspectRuntime{}, errors.Join(fmt.Errorf("container.ProjectService: %w", err), shutdownErr)
	}
	return inspectRuntime{
		inspector: service,
		shutdown: func(shutdownCtx context.Context) error {
			return commandruntime.Shutdown(shutdownCtx, container)
		},
	}, nil
}
