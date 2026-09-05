package sync

import "github.com/devctllabs/devctl/internal/domain/failure"

// Operation identifies the synchronization stage that failed.
type Operation string

const (
	OperationSelectTarget Operation = "select_target"
	OperationMaterialize  Operation = "materialize"
	OperationPublish      Operation = "publish"
	OperationPrune        Operation = "prune"
)

// FailureKind maps synchronization facts to a transport-neutral failure category.
type FailureKind uint8

const (
	FailureNotFound FailureKind = iota + 1
	FailureUnavailable
)

// OperationError retains target facts and the underlying execution cause.
type OperationError struct {
	Operation Operation
	Target    string
	Source    string
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
	if e.Operation == OperationMaterialize && e.Cause != nil {
		category := failure.CategoryOf(e.Cause)
		if category != failure.Internal {
			return category
		}
	}
	if e.Kind == FailureNotFound {
		return failure.NotFound
	}
	return failure.Unavailable
}
