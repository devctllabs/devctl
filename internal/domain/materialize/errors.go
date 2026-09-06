package materialize

import (
	"fmt"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/project"
)

// Operation identifies the materialization stage that failed.
type Operation string

const (
	OperationConfigureRouter Operation = "configure_router"
	OperationValidateSource  Operation = "validate_source"
	OperationReadFile        Operation = "read_file"
	OperationDownload        Operation = "download"
	OperationCheckout        Operation = "checkout"
	OperationBuildSnapshot   Operation = "build_snapshot"
)

// FailureKind maps materialization facts to a transport-neutral failure category.
type FailureKind uint8

const (
	FailureInvalid FailureKind = iota + 1
	FailureNotFound
	FailureUnavailable
	FailureUnsupported
)

// OperationError retains source facts and the underlying I/O or protocol cause.
type OperationError struct {
	Operation  Operation
	SourceType project.SourceType
	Path       string
	Kind       FailureKind
	Cause      error
}

func (e *OperationError) Error() string {
	message := string(e.Operation) + " failed"
	if e.Cause != nil {
		return message + ": " + e.Cause.Error()
	}
	return message
}

func (e *OperationError) Unwrap() error { return e.Cause }

func (e *OperationError) Category() failure.Category {
	switch e.Kind {
	case FailureInvalid:
		return failure.InvalidInput
	case FailureNotFound:
		return failure.NotFound
	case FailureUnavailable:
		return failure.Unavailable
	case FailureUnsupported:
		return failure.Unsupported
	default:
		return failure.Internal
	}
}

// UnsupportedSourceError reports a source type for which no strategy was configured.
type UnsupportedSourceError struct {
	SourceType project.SourceType
}

func (e *UnsupportedSourceError) Error() string {
	return fmt.Sprintf("source type %q is unsupported", e.SourceType)
}

func (e *UnsupportedSourceError) Category() failure.Category { return failure.Unsupported }

// UpstreamManifestError identifies an invalid or inaccessible upstream Devctl project.
type UpstreamManifestError struct {
	Repository string
	Ref        string
	Cause      error
}

func (e *UpstreamManifestError) Error() string {
	return "upstream Devctl manifest is invalid or inaccessible"
}

func (e *UpstreamManifestError) Category() failure.Category { return failure.InvalidInput }
func (e *UpstreamManifestError) Unwrap() error              { return e.Cause }

// ExportNotFoundError reports a requested export absent from an upstream manifest.
type ExportNotFoundError struct{ Name string }

func (e *ExportNotFoundError) Error() string {
	return fmt.Sprintf("upstream export %q was not found", e.Name)
}

func (e *ExportNotFoundError) Category() failure.Category { return failure.NotFound }

// InvalidExportError reports a selected Export that does not match an effective upstream surface.
type InvalidExportError struct{ Name string }

func (e *InvalidExportError) Error() string {
	return fmt.Sprintf("upstream export %q is invalid", e.Name)
}

func (e *InvalidExportError) Category() failure.Category { return failure.InvalidInput }

// UnsupportedExportError reports an export whose contract kind cannot be materialized.
type UnsupportedExportError struct {
	Name string
	Kind string
}

func (e *UnsupportedExportError) Error() string {
	return fmt.Sprintf("upstream export %q has unsupported kind %q", e.Name, e.Kind)
}

func (e *UnsupportedExportError) Category() failure.Category { return failure.Unsupported }

// KafkaTopicMismatchError reports a downstream topic that disagrees with its exported producer.
type KafkaTopicMismatchError struct {
	Expected string
	Actual   string
}

func (e *KafkaTopicMismatchError) Error() string {
	return fmt.Sprintf("Kafka export topic mismatch: expected %q, got %q", e.Expected, e.Actual)
}

func (e *KafkaTopicMismatchError) Category() failure.Category { return failure.InvalidInput }

// KafkaFormatMismatchError reports a downstream format that disagrees with its exported producer.
type KafkaFormatMismatchError struct {
	Expected string
	Actual   string
}

func (e *KafkaFormatMismatchError) Error() string {
	return fmt.Sprintf("Kafka export format mismatch: expected %q, got %q", e.Expected, e.Actual)
}

func (e *KafkaFormatMismatchError) Category() failure.Category { return failure.InvalidInput }
