package failure

import (
	"context"
	"errors"
)

// Category is the transport-neutral class of an application failure.
type Category string

const (
	InvalidInput Category = "invalid_input"
	NotFound     Category = "not_found"
	Conflict     Category = "conflict"
	Unavailable  Category = "unavailable"
	Unsupported  Category = "unsupported"
	Cancelled    Category = "cancelled"
	Internal     Category = "internal"
)

// Categorized exposes a stable failure class without prescribing one error type.
type Categorized interface {
	error
	// Category returns the stable transport-neutral class of the failure.
	Category() Category
}

// CategoryOf prioritizes cancellation semantics, preserves typed categories, and classifies unknown errors as Internal.
func CategoryOf(err error) Category {
	switch {
	case errors.Is(err, context.Canceled):
		return Cancelled
	case errors.Is(err, context.DeadlineExceeded):
		return Unavailable
	}
	var categorized Categorized
	if errors.As(err, &categorized) {
		return categorized.Category()
	}
	return Internal
}
