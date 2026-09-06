package project

import (
	"context"
	"errors"
	"io/fs"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

func projectOperationError(operation projectdomain.Operation, path string, kind projectdomain.FailureKind, cause error) error {
	return &projectdomain.OperationError{Operation: operation, Path: path, Kind: kind, Cause: cause}
}

func manifestAccessFailure(err error) projectdomain.FailureKind {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return projectdomain.FailureNotFound
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return projectdomain.FailureUnavailable
	default:
		return projectdomain.FailureUnavailable
	}
}
