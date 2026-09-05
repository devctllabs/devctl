package scaffold_test

import (
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/scaffold"
	"github.com/stretchr/testify/require"
)

func TestOperationErrorPreservesCategoryAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("conflict")
	err := &scaffold.OperationError{Operation: scaffold.OperationPreflight, Kind: scaffold.FailureConflict, Cause: cause}

	require.Equal(t, failure.Conflict, failure.CategoryOf(err))
	require.ErrorIs(t, err, cause)
}
