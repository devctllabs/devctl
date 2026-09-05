package project_test

import (
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestOperationErrorPreservesCategoryAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("storage failed")
	err := &project.OperationError{Operation: project.OperationLoadManifest, Kind: project.FailureNotFound, Cause: cause}

	require.Equal(t, failure.NotFound, failure.CategoryOf(err))
	require.ErrorIs(t, err, cause)
}
