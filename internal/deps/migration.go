package deps

import (
	"fmt"

	migrationrepo "github.com/devctllabs/devctl/internal/repository/migration"
	migrationservice "github.com/devctllabs/devctl/internal/service/migration"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/go-libs/di"
)

func (c *Container) provideMigration() error {
	if err := di.Provide(c.di, func(di.Resolver) (*migrationrepo.Repository, error) {
		return migrationrepo.NewRepository(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(di.Resolver) (*migrationrepo.Environment, error) {
		return migrationrepo.NewEnvironment(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (*migrationservice.Service, error) {
		projects, err := resolve[*projectservice.Service](resolver)
		if err != nil {
			return nil, err
		}
		config, err := resolve[*migrationrepo.Environment](resolver)
		if err != nil {
			return nil, err
		}
		repository, err := resolve[*migrationrepo.Repository](resolver)
		if err != nil {
			return nil, err
		}
		return migrationservice.New(projects, config, repository), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
