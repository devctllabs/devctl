package invoice

import (
	"context"
	"errors"

	kafka "github.com/devctllabs/go-libs/kafka"
	retry "github.com/devctllabs/go-libs/retry"
)

var ErrNotImplemented = errors.New("Kafka consumer handler is not implemented")

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

// Handle processes the batch synchronously. It must not retain batch data after returning.
func (h *Handler) Handle(context.Context, *kafka.Batch[[]byte]) error {
	return retry.Permanent(ErrNotImplemented)
}
