package audit

import (
	"context"
	"errors"

	kafka "github.com/devctllabs/go-libs/kafka"
	retry "github.com/devctllabs/go-libs/retry"
)

var ErrNotImplemented = errors.New("Kafka consumer handler is not implemented")

// Handler processes batches for this Kafka consumer.
type Handler struct{}

// NewHandler creates a handler whose batch processing must be implemented by the application.
func NewHandler() *Handler { return &Handler{} }

// Handle processes the batch synchronously. It must not retain batch data after returning.
func (h *Handler) Handle(context.Context, *kafka.Batch[[]byte]) error {
	return retry.Permanent(ErrNotImplemented)
}
