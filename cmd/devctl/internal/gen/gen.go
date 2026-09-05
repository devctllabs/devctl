package gen

import (
	"context"
	"errors"
	"fmt"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	"github.com/devctllabs/go-libs/lifecycle"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/gen.go -package mocks -typed . generator

type generator interface {
	// Generate returns completed target facts even when a later generation step fails.
	Generate(ctx context.Context, command generatedomain.Command) (generatedomain.Result, error)
}

// genRuntime contains the application port and cleanup hook used by Action.
type genRuntime struct {
	generator generator
	shutdown  func(context.Context) error
}

// genBuilder isolates dependency construction from generation behavior.
type genBuilder func(ctx context.Context, logger *zap.Logger) (genRuntime, error)

// genCmd owns parsed options, a generation family selector, and its runtime factory.
type genCmd struct {
	opts         genCmdOpts
	family       string
	buildRuntime genBuilder
}

// genCmdOpts receives common flags plus generation target selection.
type genCmdOpts struct {
	commandruntime.CommonCmdOpts
	Target string
	DryRun bool
}

// genLeafSpec identifies the command node and the generation scope it selects.
type genLeafSpec struct {
	name        string
	family      string
	allowTarget bool
}

// NewCmd constructs the gen command tree.
func NewCmd() *cli.Command {
	return newGenCmd(genCmdOpts{}, buildGen)
}

func newGenCmd(opts genCmdOpts, build genBuilder) *cli.Command {
	command := newGenLeaf(genLeafSpec{name: "gen", allowTarget: true}, opts, build)
	command.Commands = []*cli.Command{newGenConfigCmd(genCmdOpts{}, build), newGenHTTPCmd(genCmdOpts{}, build), newGenLeaf(genLeafSpec{name: "grpc", family: "grpc", allowTarget: true}, genCmdOpts{}, build), newGenLeaf(genLeafSpec{name: "kafka", family: "kafka", allowTarget: true}, genCmdOpts{}, build)}
	return command
}

// newGenLeaf constructs one executable generation node for the selected family.
func newGenLeaf(spec genLeafSpec, opts genCmdOpts, build genBuilder) *cli.Command {
	cmd := &genCmd{opts: opts, family: spec.family, buildRuntime: build}
	usageText := "devctl gen"
	if spec.name != "gen" {
		usageText += " " + spec.name
	}
	if spec.allowTarget {
		usageText += " [--target <target-id>]"
	}
	usageText += " [--dry-run]"
	flags := append(cmd.opts.CommonFlags(), &cli.BoolFlag{Name: "dry-run", Destination: &cmd.opts.DryRun, Usage: "preview Managed Outputs without running generators or writing files"})
	if spec.allowTarget {
		flags = append(flags, &cli.StringFlag{Name: "target", Destination: &cmd.opts.Target, Usage: "select one exact generation Target `id`"})
	}
	return &cli.Command{
		Name:        spec.name,
		Usage:       "Generate Managed Outputs",
		Description: genDescription(spec.family),
		UsageText:   usageText,
		Flags:       flags,
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

func genDescription(family string) string {
	if family == "" {
		return "Run the Project-owned generators for every supported Target and atomically publish each Target's Managed Output. Generation never synchronizes or lints implicitly."
	}
	return "Run the Project-owned generators for " + family + " Targets and atomically publish their Managed Outputs without synchronizing or linting implicitly."
}

// Action generates the selected managed outputs and emits the completed or partial result.
func (cmd *genCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("gen accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.generator.Generate(ctx, generatedomain.Command{
		ManifestPath: cmd.opts.ManifestPath,
		Family:       cmd.family,
		Target:       cmd.opts.Target,
		DryRun:       cmd.opts.DryRun,
	})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	dto := generateDTO(result)
	if finalErr := errors.Join(operationErr, shutdownErr); finalErr != nil {
		var options []commandruntime.ErrorOption
		if !result.DryRun && (len(result.Targets) > 0 || len(result.Changes) > 0) {
			options = append(options, commandruntime.WithPartialResult(dto))
		}
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("managed output generation completed", zap.Any("data", dto))
	return nil
}

// changeDTO is one stable managed-file change fact.
type changeDTO struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Action string `json:"action"`
}

// resultDTO contains every completed generation target and file change.
type resultDTO struct {
	Targets []string    `json:"targets"`
	Changes []changeDTO `json:"changes"`
	DryRun  bool        `json:"dry_run"`
}

// generateDTO converts the domain result into the stable CLI payload.
func generateDTO(result generatedomain.Result) resultDTO {
	targets := make([]string, len(result.Targets))
	copy(targets, result.Targets)
	changes := make([]changeDTO, 0, len(result.Changes))
	for _, change := range result.Changes {
		changes = append(changes, changeDTO{Target: change.Target, Path: change.Path, Action: string(change.Action)})
	}
	return resultDTO{Targets: targets, Changes: changes, DryRun: result.DryRun}
}

// buildGen constructs and resolves the lazy dependencies owned by gen.
func buildGen(ctx context.Context, logger *zap.Logger) (genRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return genRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.GenService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return genRuntime{}, errors.Join(fmt.Errorf("container.GenService: %w", err), shutdownErr)
	}
	return genRuntime{generator: service, shutdown: container.Shutdown}, nil
}
