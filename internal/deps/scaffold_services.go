package deps

import (
	"fmt"

	scaffoldservice "github.com/devctllabs/devctl/internal/service/scaffold"
	"github.com/devctllabs/go-libs/di"
	"go.uber.org/zap"
)

func (c *Container) provideScaffoldServices() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*scaffoldservice.Service, error) {
		logger, err := resolve[*zap.Logger](resolver)
		if err != nil {
			return nil, err
		}
		projects, err := resolve[scaffoldservice.ProjectRepository](resolver)
		if err != nil {
			return nil, err
		}
		workspace, err := resolve[scaffoldservice.WorkspaceRepository](resolver)
		if err != nil {
			return nil, err
		}
		return scaffoldservice.New(logger.Named("service.scaffold"), projects, workspace), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
