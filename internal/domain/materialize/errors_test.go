package materialize_test

import (
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/stretchr/testify/require"
)

func TestOperationErrorPreservesCategoryAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("download failed")
	err := &materialize.OperationError{Operation: materialize.OperationDownload, Kind: materialize.FailureUnavailable, Cause: cause}

	require.Equal(t, failure.Unavailable, failure.CategoryOf(err))
	require.ErrorIs(t, err, cause)
}

func TestInvalidExportErrorIsInvalidInput(t *testing.T) {
	t.Parallel()

	err := &materialize.InvalidExportError{Name: "public-api"}

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	require.Contains(t, err.Error(), "public-api")
}

func TestKafkaFormatMismatchErrorIsInvalidInput(t *testing.T) {
	t.Parallel()

	err := &materialize.KafkaFormatMismatchError{Expected: "raw", Actual: "json"}

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	require.Contains(t, err.Error(), "raw")
	require.Contains(t, err.Error(), "json")
}
