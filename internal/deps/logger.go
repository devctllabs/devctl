package deps

import (
	"fmt"

	"github.com/devctllabs/go-libs/di"
	"go.uber.org/zap"
)

func (c *Container) provideLogger(logger *zap.Logger) error {
	if err := di.ProvideValue(c.di, logger); err != nil {
		return fmt.Errorf("di.ProvideValue: %w", err)
	}
	return nil
}
