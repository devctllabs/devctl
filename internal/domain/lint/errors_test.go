package lint_test

import (
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/lint"
	"github.com/stretchr/testify/require"
)

func TestOperationErrorPreservesCategoryAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("read failed")
	err := &lint.OperationError{Operation: lint.OperationReadContract, Kind: lint.FailureUnavailable, Cause: cause}

	require.Equal(t, failure.Unavailable, failure.CategoryOf(err))
	require.ErrorIs(t, err, cause)
}
