package command

import (
	"context"
	"fmt"
	"time"

	"github.com/devctllabs/devctl/internal/deps"
	"github.com/devctllabs/go-libs/lifecycle"
)

const shutdownTimeout = 5 * time.Second

// Shutdown closes resources with the common fresh bounded context convention.
func Shutdown(ctx context.Context, container *deps.Container) error {
	if err := lifecycle.Shutdown(ctx, shutdownTimeout, container.Shutdown); err != nil {
		return fmt.Errorf("lifecycle.Shutdown: %w", err)
	}
	return nil
}
