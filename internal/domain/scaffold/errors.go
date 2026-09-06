package scaffold

import "github.com/devctllabs/devctl/internal/domain/failure"

// Operation identifies the scaffold stage that failed.
type Operation string

const (
	OperationPlan      Operation = "plan"
	OperationPreflight Operation = "preflight"
	OperationPublish   Operation = "publish"
)

// FailureKind maps scaffold facts to a transport-neutral failure category.
type FailureKind uint8

const (
	FailureConflict FailureKind = iota + 1
	FailureUnavailable
	FailureInternal
)

// OperationError retains scaffold path facts and the underlying execution cause.
type OperationError struct {
	Operation Operation
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
	switch e.Kind {
	case FailureConflict:
		return failure.Conflict
	case FailureUnavailable:
		return failure.Unavailable
	case FailureInternal:
		return failure.Internal
	default:
		return failure.Internal
	}
}
