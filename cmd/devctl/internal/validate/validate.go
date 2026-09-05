package validate

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

//go:generate go tool mockgen -destination mocks/validate.go -package mocks -typed . projectValidator

type projectValidator interface {
	// Validate returns all available project issues; invalid project data is not an execution error.
	Validate(ctx context.Context, query projectdomain.ValidateQuery) (projectdomain.ValidationResult, error)
}

// validateRuntime contains only the behavior and cleanup hook needed after command construction.
type validateRuntime struct {
	validator projectValidator
	shutdown  func(context.Context) error
}

// validateBuilder owns dependency construction so Action remains testable without a container.
type validateBuilder func(ctx context.Context, logger *zap.Logger) (validateRuntime, error)

// validateCmd owns parsed options and the runtime factory for one invocation.
type validateCmd struct {
	opts         validateCmdOpts
	buildRuntime validateBuilder
}

// validateCmdOpts receives the common leaf flags bound by urfave/cli.
type validateCmdOpts struct {
	commandruntime.CommonCmdOpts
}

// NewCmd constructs the validate command.
func NewCmd() *cli.Command { return newValidateCmd(validateCmdOpts{}, buildValidate) }

func newValidateCmd(opts validateCmdOpts, build validateBuilder) *cli.Command {
	cmd := &validateCmd{opts: opts, buildRuntime: build}
	return &cli.Command{
		Name:        "validate",
		Usage:       "Validate the selected Project",
		Description: "Check Manifest structure, semantic validity, references, safe paths, and Project Readiness. Validation findings are normal results and exit with status 1 when any issue is present.",
		UsageText:   "devctl validate [--file <path>]",
		Flags:       cmd.opts.CommonFlags(),
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

// Action validates the selected project and emits its complete result as one event.
func (cmd *validateCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("validate accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, validateErr := runtime.validator.Validate(ctx, projectdomain.ValidateQuery{ManifestPath: cmd.opts.ManifestPath})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	dto := validationDTO(result)
	var options []commandruntime.ErrorOption
	if validateErr == nil {
		options = append(options, commandruntime.WithPartialResult(dto))
	}
	if finalErr := errors.Join(validateErr, shutdownErr); finalErr != nil {
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("project validation completed", zap.Any("data", dto))
	if !result.IsValid() {
		return cli.Exit("", 1)
	}
	return nil
}

type validationResultDTO struct {
	Valid  bool                                `json:"valid"`
	Issues []commandruntime.ValidationIssueDTO `json:"issues"`
}

// validationDTO converts the domain result into the stable CLI payload.
func validationDTO(result projectdomain.ValidationResult) validationResultDTO {
	return validationResultDTO{Valid: result.IsValid(), Issues: commandruntime.ValidationIssueDTOs(result.Issues)}
}

// buildValidate constructs and resolves the lazy dependencies owned by validate.
func buildValidate(ctx context.Context, logger *zap.Logger) (validateRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return validateRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.ProjectService()
	if err != nil {
		return validateRuntime{}, errors.Join(err, lifecycle.Shutdown(ctx, 5*time.Second, container.Shutdown))
	}
	return validateRuntime{validator: service, shutdown: container.Shutdown}, nil
}
