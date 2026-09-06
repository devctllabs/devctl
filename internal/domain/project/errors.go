package project

import "github.com/devctllabs/devctl/internal/domain/failure"

// Operation identifies the project or manifest stage that failed.
type Operation string

const (
	OperationLoadManifest Operation = "load_manifest"
	OperationSaveManifest Operation = "save_manifest"
	OperationInspectFile  Operation = "inspect_file"
	OperationReadFile     Operation = "read_file"
	OperationInitManifest Operation = "init_manifest"
)

// FailureKind maps project facts to a transport-neutral failure category.
type FailureKind uint8

const (
	FailureInvalid FailureKind = iota + 1
	FailureNotFound
	FailureConflict
	FailureUnavailable
	FailureInternal
)

// OperationError retains project path facts and the underlying execution cause.
type OperationError struct {
	Operation Operation
	Path      string
	Kind      FailureKind
	Cause     error
}

// MutationReason identifies the stable policy fact that rejected a manifest mutation.
type MutationReason string

const (
	MutationUnsupportedOption MutationReason = "unsupported_option"
	MutationUnsupportedValue  MutationReason = "unsupported_value"
	MutationInvalidName       MutationReason = "invalid_name"
	MutationInvalidOptions    MutationReason = "invalid_options"
	MutationInvalidURL        MutationReason = "invalid_url"
	MutationInsecureURL       MutationReason = "insecure_url"
	MutationNotFound          MutationReason = "not_found"
	MutationExistingConflict  MutationReason = "existing_conflict"
)

// MutationError contains presentation-neutral facts about a rejected manifest mutation.
type MutationError struct {
	Reason   MutationReason
	Field    string
	Value    string
	Conflict bool
}

func (e *MutationError) Error() string { return "manifest mutation failed" }

func (e *MutationError) Category() failure.Category {
	if e.Conflict {
		return failure.Conflict
	}
	return failure.InvalidInput
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

// InvalidManifestError reports structural or semantic issues that block a project operation.
type InvalidManifestError struct {
	Path   string
	Issues []Issue
	Cause  error
}

func (e *InvalidManifestError) Error() string { return "manifest is invalid" }

func (e *InvalidManifestError) Category() failure.Category { return failure.InvalidInput }

func (e *InvalidManifestError) Unwrap() error { return e.Cause }
