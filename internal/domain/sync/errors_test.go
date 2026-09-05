package sync_test

import (
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	syncdomain "github.com/devctllabs/devctl/internal/domain/sync"
	"github.com/stretchr/testify/require"
)

func TestOperationErrorPreservesCategoryAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("target missing")
	err := &syncdomain.OperationError{Operation: syncdomain.OperationSelectTarget, Kind: syncdomain.FailureNotFound, Cause: cause}

	require.Equal(t, failure.NotFound, failure.CategoryOf(err))
	require.ErrorIs(t, err, cause)
}
