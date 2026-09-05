package toolrun

import (
	"context"
	"fmt"
	"os"
)

// WithTemporaryOutput runs use in a temporary directory and always attempts cleanup.
func WithTemporaryOutput[T any](
	ctx context.Context,
	prefix string,
	use func(string) (T, error),
) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("ctx.Err: %w", err)
	}
	temporary, err := os.MkdirTemp("", prefix)
	if err != nil {
		return zero, fmt.Errorf("os.MkdirTemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	return use(temporary)
}
