package lint

import (
	"context"
	"errors"
	"fmt"
	"time"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	"github.com/devctllabs/devctl/internal/deps"
	lintdomain "github.com/devctllabs/devctl/internal/domain/lint"
	"github.com/devctllabs/go-libs/lifecycle"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/lint.go -package mocks -typed . linter

type linter interface {
	// Lint returns collected findings even when a later contract cannot be processed.
	Lint(ctx context.Context, command lintdomain.Command) (lintdomain.Result, error)
}

// lintRuntime contains the application port and cleanup hook used by Action.
type lintRuntime struct {
	linter   linter
	shutdown func(context.Context) error
}

// lintBuilder isolates dependency construction from lint behavior.
type lintBuilder func(ctx context.Context, logger *zap.Logger) (lintRuntime, error)

// lintCmd owns parsed options, a contract family selector, and its runtime factory.
type lintCmd struct {
	opts         lintCmdOpts
	family       string
	buildRuntime lintBuilder
}

// lintCmdOpts receives the common leaf flags bound by urfave/cli.
type lintCmdOpts struct {
	commandruntime.CommonCmdOpts
}

// NewCmd constructs the lint command tree.
func NewCmd() *cli.Command {
	return newLintCmd(lintCmdOpts{}, buildLint)
}

func newLintCmd(opts lintCmdOpts, build lintBuilder) *cli.Command {
	command := newLintLeaf("lint", "", opts, build)
	command.Commands = []*cli.Command{newLintHTTPCmd(lintCmdOpts{}, build), newLintLeaf("grpc", "grpc", lintCmdOpts{}, build), newLintLeaf("kafka", "kafka", lintCmdOpts{}, build)}
	return command
}

// newLintLeaf constructs one executable lint node for the selected contract family.
func newLintLeaf(name, family string, opts lintCmdOpts, build lintBuilder) *cli.Command {
	cmd := &lintCmd{opts: opts, family: family, buildRuntime: build}
	usageText := "devctl lint"
	if family != "" {
		usageText += " " + name
	}
	return &cli.Command{
		Name:        name,
		Usage:       "Lint Project Contracts",
		Description: lintDescription(family),
		UsageText:   usageText + " [--file <path>]",
		Flags:       cmd.opts.CommonFlags(),
		OnUsageError: func(_ context.Context, command *cli.Command, err error, _ bool) error {
			reporter := commandruntime.NewErrorReporter(cmd.opts.NewStderrLogger(command), cmd.opts.Verbose)
			return reporter.ReportError(cli.Exit(err, 2))
		},
		Action: cmd.Action,
	}
}

func lintDescription(family string) string {
	if family == "" {
		return "Lint every supported Contract using committed local inputs. Findings are normal results and exit with status 1 without becoming execution errors."
	}
	return "Lint committed " + family + " Contracts without synchronizing or generating code. Findings are normal results and exit with status 1."
}

// Action lints selected contracts and emits all collected findings.
func (cmd *lintCmd) Action(ctx context.Context, command *cli.Command) error {
	stdout := cmd.opts.NewStdoutLogger(command)
	stderr := cmd.opts.NewStderrLogger(command)
	reporter := commandruntime.NewErrorReporter(stderr, cmd.opts.Verbose)
	if command.Args().Len() != 0 {
		return reporter.ReportError(cli.Exit("lint accepts no positional arguments", 2))
	}
	runtime, err := cmd.buildRuntime(ctx, stderr)
	if err != nil {
		return reporter.ReportError(err)
	}
	result, operationErr := runtime.linter.Lint(ctx, lintdomain.Command{ManifestPath: cmd.opts.ManifestPath, Family: cmd.family})
	shutdownErr := lifecycle.Shutdown(ctx, 5*time.Second, runtime.shutdown)
	dto := lintDTO(result)
	if finalErr := errors.Join(operationErr, shutdownErr); finalErr != nil {
		var options []commandruntime.ErrorOption
		if len(result.Contracts) > 0 {
			options = append(options, commandruntime.WithPartialResult(dto))
		}
		return reporter.ReportError(finalErr, options...)
	}
	stdout.Info("contract lint completed", zap.Any("data", dto))
	if !result.Valid {
		return cli.Exit("", 1)
	}
	return nil
}

// issueDTO is one stable contract lint finding.
type issueDTO struct {
	Code       string              `json:"code"`
	Target     string              `json:"target,omitempty"`
	Path       string              `json:"path,omitempty"`
	Line       int                 `json:"line,omitempty"`
	Column     int                 `json:"column,omitempty"`
	Field      string              `json:"field,omitempty"`
	Parameters *issueParametersDTO `json:"parameters,omitempty"`
}

// issueParametersDTO carries finding-specific facts safe for CLI output.
type issueParametersDTO struct {
	OperationID string `json:"operation_id,omitempty"`
	Location    string `json:"location,omitempty"`
	Type        string `json:"type,omitempty"`
	Subtype     string `json:"subtype,omitempty"`
	SpecPath    string `json:"spec_path,omitempty"`
}

// resultDTO contains all contracts and findings inspected by one lint run.
type resultDTO struct {
	Valid     bool       `json:"valid"`
	Contracts []string   `json:"contracts"`
	Issues    []issueDTO `json:"issues"`
}

// lintDTO converts the domain result into the stable CLI payload.
func lintDTO(result lintdomain.Result) resultDTO {
	contracts := make([]string, len(result.Contracts))
	copy(contracts, result.Contracts)
	issues := make([]issueDTO, 0, len(result.Issues))
	for _, issue := range result.Issues {
		var parameters *issueParametersDTO
		if issue.Parameters != nil {
			parameters = &issueParametersDTO{
				OperationID: issue.Parameters.OperationID, Location: issue.Parameters.Location,
				Type: issue.Parameters.Type, Subtype: issue.Parameters.Subtype, SpecPath: issue.Parameters.SpecPath,
			}
		}
		issues = append(issues, issueDTO{
			Code: issue.Code, Target: issue.Target, Path: issue.Path, Line: issue.Line,
			Column: issue.Column, Field: issue.Field, Parameters: parameters,
		})
	}
	return resultDTO{Valid: result.Valid, Contracts: contracts, Issues: issues}
}

// buildLint constructs and resolves the lazy dependencies owned by lint.
func buildLint(ctx context.Context, logger *zap.Logger) (lintRuntime, error) {
	container, err := deps.New(logger)
	if err != nil {
		return lintRuntime{}, fmt.Errorf("deps.New: %w", err)
	}
	service, err := container.LintService()
	if err != nil {
		shutdownErr := commandruntime.Shutdown(ctx, container)
		return lintRuntime{}, errors.Join(fmt.Errorf("container.LintService: %w", err), shutdownErr)
	}
	return lintRuntime{linter: service, shutdown: container.Shutdown}, nil
}
