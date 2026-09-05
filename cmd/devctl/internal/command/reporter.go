package command

import (
	"context"
	"errors"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/failure"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

const urfaveHelpExitCode = 3

// ErrorOption adds caller-approved facts to an error event.
type ErrorOption func(*errorReport)

// WithPartialResult attaches caller-approved completed work to recovery details.
func WithPartialResult(result any) ErrorOption {
	return func(report *errorReport) {
		report.addDetail("partial_result", result)
	}
}

// ErrorReporter owns safe CLI error presentation and process exit classification.
type ErrorReporter struct {
	logger  *zap.Logger
	verbose bool
}

// NewErrorReporter creates a reporter that writes final errors through logger.
func NewErrorReporter(logger *zap.Logger, verbose bool) *ErrorReporter {
	return &ErrorReporter{logger: logger, verbose: verbose}
}

// ReportError writes one final safe error event and returns a silent error with the same cause.
func (r *ErrorReporter) ReportError(err error, options ...ErrorOption) error {
	report := classifyError(err)
	for _, option := range options {
		if option != nil {
			option(&report)
		}
	}
	if err.Error() != "" {
		fields := []zap.Field{zap.String("code", report.code), zap.Int("exit_code", report.exitCode)}
		if len(report.details) > 0 {
			fields = append(fields, zap.Any("details", report.details))
		}
		if r.verbose {
			fields = append(fields, zap.Error(err))
		}
		r.logger.Error(report.message, fields...)
	}
	return &reportedError{cause: err, exitCode: report.exitCode}
}

type errorReport struct {
	message  string
	code     string
	exitCode int
	details  map[string]any
}

func (r *errorReport) addDetail(key string, value any) {
	if r.details == nil {
		r.details = make(map[string]any)
	}
	r.details[key] = value
}

// classifyError maps domain and CLI errors into one safe presentation record.
func classifyError(err error) errorReport {
	exitCode := errorExitCode(err)
	if exitCode == 2 {
		return errorReport{message: err.Error(), code: "usage", exitCode: exitCode}
	}
	if exitCode == 130 {
		return errorReport{message: "operation was cancelled", code: "cancelled", exitCode: exitCode}
	}

	code, message := executionErrorMessage(err)
	report := errorReport{message: message, code: code, exitCode: exitCode}
	var invalidManifest *projectdomain.InvalidManifestError
	if errors.As(err, &invalidManifest) {
		report.addDetail("path", invalidManifest.Path)
		report.addDetail("issues", ValidationIssueDTOs(invalidManifest.Issues))
	}
	var metadataErr *contract.SnapshotMetadataError
	if errors.As(err, &metadataErr) {
		report.addDetail("type", "snapshot_metadata_invalid")
		report.addDetail("field", metadataErr.Field)
		report.addDetail("reason", string(metadataErr.Reason))
		report.addDetail("hint", metadataErr.Hint)
	}
	return report
}

// errorExitCode normalizes framework and cancellation errors to the public CLI contract.
func errorExitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	var exitCoder cli.ExitCoder
	if !errors.As(err, &exitCoder) {
		return 1
	}
	if exitCoder.ExitCode() == urfaveHelpExitCode {
		return 2
	}
	return exitCoder.ExitCode()
}

// ExitCode maps a command error to the process status used by devctl.
func ExitCode(err error) int {
	return errorExitCode(err)
}

// executionErrorMessage maps domain failure categories to stable public codes and messages.
func executionErrorMessage(err error) (string, string) {
	switch failure.CategoryOf(err) {
	case failure.Cancelled:
		return "cancelled", "operation was cancelled"
	case failure.Unavailable:
		return "unavailable", "required dependency is unavailable"
	case failure.InvalidInput:
		return "invalid_input", "input is invalid"
	case failure.NotFound:
		return "not_found", "requested resource was not found"
	case failure.Conflict:
		return "conflict", "operation conflicts with existing state"
	case failure.Unsupported:
		return "unsupported", "requested operation is unsupported"
	case failure.Internal:
		return "internal", "internal error"
	}
	return "internal", "internal error"
}

// reportedError suppresses framework rendering while retaining cause and process status.
type reportedError struct {
	cause    error
	exitCode int
}

func (e *reportedError) Error() string { return "" }

func (e *reportedError) Unwrap() error { return e.cause }

func (e *reportedError) ExitCode() int { return e.exitCode }
