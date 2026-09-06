package generate

import "github.com/devctllabs/devctl/internal/domain/failure"

// Operation identifies the generation stage that failed.
type Operation string

const (
	OperationSelectTarget   Operation = "select_target"
	OperationLocateContract Operation = "locate_contract"
	OperationRunGenerator   Operation = "run_generator"
	OperationPublishOutput  Operation = "publish_output"
)

// FailureKind maps generation facts to a transport-neutral failure category.
type FailureKind uint8

const (
	FailureNotFound FailureKind = iota + 1
	FailureUnavailable
)

// OperationError retains generation facts and the underlying execution cause.
type OperationError struct {
	Operation Operation
	Target    string
	Path      string
	Kind      FailureKind
	Cause     error
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
	if e.Kind == FailureNotFound {
		return failure.NotFound
	}
	return failure.Unavailable
}
