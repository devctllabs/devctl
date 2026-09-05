package deps

import (
	"fmt"

	"github.com/devctllabs/go-libs/di"
)

func resolve[T any](resolver di.Resolver) (T, error) {
	value, err := di.Resolve[T](resolver)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("di.Resolve: %w", err)
	}
	return value, nil
}
