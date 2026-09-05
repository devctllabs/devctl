package generate_test

import (
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/generate"
	"github.com/stretchr/testify/require"
)

func TestOperationErrorPreservesCategoryAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("tool failed")
	err := &generate.OperationError{Operation: generate.OperationRunGenerator, Kind: generate.FailureUnavailable, Cause: cause}

	require.Equal(t, failure.Unavailable, failure.CategoryOf(err))
	require.ErrorIs(t, err, cause)
}
