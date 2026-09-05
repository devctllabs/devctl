package deps

import (
	"fmt"

	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	scaffoldservice "github.com/devctllabs/devctl/internal/service/scaffold"
	"github.com/devctllabs/go-libs/di"
)

func (c *Container) provideScaffoldRepositories() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (scaffoldservice.ProjectRepository, error) {
		return resolve[*projectservice.Service](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (scaffoldservice.WorkspaceRepository, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
