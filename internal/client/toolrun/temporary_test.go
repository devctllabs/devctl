package toolrun_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/stretchr/testify/require"
)

func TestWithTemporaryOutputReturnsValueAndCleansAfterSuccess(t *testing.T) {
	t.Parallel()

	var temporary string

	value, err := toolrun.WithTemporaryOutput(context.Background(), "devctl-toolrun-success-", func(directory string) (string, error) {
		temporary = directory
		require.DirExists(t, directory)
		require.True(t, strings.HasPrefix(filepath.Base(directory), "devctl-toolrun-success-"))
		return "generated", nil
	})

	require.NoError(t, err)
	require.Equal(t, "generated", value)
	require.NoDirExists(t, temporary)
}

func TestWithTemporaryOutputPreservesValueAndErrorAndCleansAfterFailure(t *testing.T) {
	t.Parallel()

	primary := errors.New("generation failed")
	var temporary string

	value, err := toolrun.WithTemporaryOutput(context.Background(), "devctl-toolrun-failure-", func(directory string) (int, error) {
		temporary = directory
		return 42, primary
	})

	require.Equal(t, 42, value)
	require.ErrorIs(t, err, primary)
	require.NoDirExists(t, temporary)
}

func TestWithTemporaryOutputCleansAfterCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var temporary string

	value, err := toolrun.WithTemporaryOutput(ctx, "devctl-toolrun-cancel-", func(directory string) (int, error) {
		temporary = directory
		cancel()
		return 7, ctx.Err()
	})

	require.Equal(t, 7, value)
	require.ErrorIs(t, err, context.Canceled)
	require.NoDirExists(t, temporary)
}

func TestWithTemporaryOutputDoesNotStartAfterCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false

	_, err := toolrun.WithTemporaryOutput(ctx, "devctl-toolrun-cancelled-", func(string) (int, error) {
		called = true
		return 0, nil
	})

	require.ErrorIs(t, err, context.Canceled)
	require.False(t, called)
}
