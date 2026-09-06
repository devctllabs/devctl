package lint

import "github.com/devctllabs/devctl/internal/domain/failure"

// Operation identifies the lint execution stage that failed.
type Operation string

const (
	OperationSelectContracts Operation = "select_contracts"
	OperationLocateContract  Operation = "locate_contract"
	OperationReadContract    Operation = "read_contract"
)

// FailureKind maps lint execution facts to a transport-neutral failure category.
type FailureKind uint8

const (
	FailureInvalid FailureKind = iota + 1
	FailureUnavailable
)

// OperationError represents an execution failure, never a lint finding.
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
	if e.Kind == FailureInvalid {
		return failure.InvalidInput
	}
	return failure.Unavailable
}
