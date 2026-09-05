package deps

import (
	"context"

	"github.com/devctllabs/go-libs/di"
)

// application is the user-owned composition root. Add application dependencies here.
type application struct{}

// provideApplication is created once. Add newly scaffolded provider calls manually.
func provideApplication(ctx context.Context, graph *di.Container, cfg *Config) error {
	return nil
}
