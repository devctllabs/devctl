package deps

import (
	"context"
	"fmt"

	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	"github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/scaffold"
	syncservice "github.com/devctllabs/devctl/internal/service/sync"
	"github.com/devctllabs/go-libs/di"
	"go.uber.org/zap"
)

// Container owns the lazy dependency graph and the resources resolved from it.
type Container struct {
	di *di.Container
}

// New registers the complete lazy CLI dependency graph. Providers construct
// dependencies only when a typed getter resolves the selected command root.
func New(logger *zap.Logger) (*Container, error) {
	container := &Container{di: di.New()}
	providers := []func() error{
		func() error { return container.provideLogger(logger) },
		container.provideRepositories,
		container.provideTargetInput,
		container.provideScaffoldRepositories,
		container.provideClients,
		container.provideServices,
		container.provideScaffoldServices,
	}
	for _, provide := range providers {
		if err := provide(); err != nil {
			return nil, err
		}
	}
	return container, nil
}

// ProjectService resolves the project query and mutation root on demand.
func (c *Container) ProjectService() (*project.Service, error) {
	service, err := resolve[*project.Service](c.di)
	if err != nil {
		return nil, fmt.Errorf("di.Resolve: %w", err)
	}
	return service, nil
}

// ScaffoldService resolves the project scaffolding root on demand.
func (c *Container) ScaffoldService() (*scaffold.Service, error) {
	service, err := resolve[*scaffold.Service](c.di)
	if err != nil {
		return nil, fmt.Errorf("di.Resolve: %w", err)
	}
	return service, nil
}

// SyncService resolves the contract synchronization root on demand.
func (c *Container) SyncService() (*syncservice.Service, error) {
	service, err := resolve[*syncservice.Service](c.di)
	if err != nil {
		return nil, fmt.Errorf("di.Resolve: %w", err)
	}
	return service, nil
}

// LintService resolves the contract lint root on demand.
func (c *Container) LintService() (*lintservice.Service, error) {
	service, err := resolve[*lintservice.Service](c.di)
	if err != nil {
		return nil, fmt.Errorf("di.Resolve: %w", err)
	}
	return service, nil
}

// GenService resolves the managed-output generation root on demand.
func (c *Container) GenService() (*generateservice.Service, error) {
	service, err := resolve[*generateservice.Service](c.di)
	if err != nil {
		return nil, fmt.Errorf("di.Resolve: %w", err)
	}
	return service, nil
}

// Shutdown closes every constructed resource in dependency order.
func (c *Container) Shutdown(ctx context.Context) error {
	if err := c.di.Shutdown(ctx); err != nil {
		return fmt.Errorf("di.Shutdown: %w", err)
	}
	return nil
}
