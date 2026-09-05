package failure_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	"github.com/stretchr/testify/require"
)

type categorizedError struct {
	category failure.Category
}

func (e categorizedError) Error() string              { return "categorized" }
func (e categorizedError) Category() failure.Category { return e.category }

func TestCategoryOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected failure.Category
	}{
		{name: "typed category", err: fmt.Errorf("repository.Load: %w", categorizedError{category: failure.NotFound}), expected: failure.NotFound},
		{name: "wrapped cancellation", err: fmt.Errorf("client.Do: %w", context.Canceled), expected: failure.Cancelled},
		{name: "deadline", err: context.DeadlineExceeded, expected: failure.Unavailable},
		{name: "unknown", err: errors.New("boom"), expected: failure.Internal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, failure.CategoryOf(test.err))
		})
	}
}
