package deps

import (
	"fmt"

	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	"github.com/devctllabs/devctl/internal/service/contractsnapshot"
	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/targetinput"
	"github.com/devctllabs/go-libs/di"
)

func (c *Container) provideTargetInput() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*contractsnapshot.Loader, error) {
		workspace, err := resolve[*workspacerepo.FilesystemRepo](resolver)
		if err != nil {
			return nil, err
		}
		return contractsnapshot.New(workspace), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (*targetinput.Resolver, error) {
		workspace, err := resolve[*workspacerepo.FilesystemRepo](resolver)
		if err != nil {
			return nil, err
		}
		snapshots, err := resolve[*contractsnapshot.Loader](resolver)
		if err != nil {
			return nil, err
		}
		return targetinput.New(workspace, snapshots), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (generateservice.TargetResolver, error) {
		return resolve[*targetinput.Resolver](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (lintservice.TargetResolver, error) {
		return resolve[*targetinput.Resolver](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (projectservice.TargetResolver, error) {
		return resolve[*targetinput.Resolver](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
