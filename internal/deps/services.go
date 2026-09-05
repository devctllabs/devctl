package deps

import (
	"fmt"

	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	"github.com/devctllabs/devctl/internal/service/materialize"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/projectreadiness"
	syncservice "github.com/devctllabs/devctl/internal/service/sync"
	"github.com/devctllabs/go-libs/di"
	"go.uber.org/zap"
)

func (c *Container) provideServices() error {
	providers := []func() error{
		c.provideProjectReadiness,
		c.provideProjectService,
		c.provideMaterializeService,
		c.provideSyncService,
		c.provideLintService,
		c.provideGenService,
	}
	for _, provide := range providers {
		if err := provide(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Container) provideProjectReadiness() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*projectreadiness.Checker, error) {
		workspace, err := resolve[projectreadiness.Workspace](resolver)
		if err != nil {
			return nil, err
		}
		return projectreadiness.New(workspace), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideMaterializeService() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*materialize.Service, error) {
		reader, err := resolve[materialize.FileReader](resolver)
		if err != nil {
			return nil, err
		}
		httpClient, err := resolve[materialize.HTTPClient](resolver)
		if err != nil {
			return nil, err
		}
		gitClient, err := resolve[materialize.GitClient](resolver)
		if err != nil {
			return nil, err
		}
		manifests, err := resolve[materialize.ManifestRepository](resolver)
		if err != nil {
			return nil, err
		}
		return materialize.New(
			materialize.NewLocal(reader),
			materialize.NewURL(httpClient),
			materialize.NewGit(gitClient, reader),
			materialize.NewDevctl(gitClient, manifests, reader),
		)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideProjectService() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*projectservice.Service, error) {
		logger, err := resolve[*zap.Logger](resolver)
		if err != nil {
			return nil, err
		}
		manifests, err := resolve[projectservice.ManifestRepository](resolver)
		if err != nil {
			return nil, err
		}
		locator, err := resolve[projectservice.ManifestLocator](resolver)
		if err != nil {
			return nil, err
		}
		inputs, err := resolve[projectservice.TargetResolver](resolver)
		if err != nil {
			return nil, err
		}
		readiness, err := resolve[*projectreadiness.Checker](resolver)
		if err != nil {
			return nil, err
		}
		return projectservice.New(logger.Named("service.project"), projectservice.Dependencies{
			Manifests: manifests, Locator: locator, Inputs: inputs, Readiness: readiness,
		}), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideSyncService() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*syncservice.Service, error) {
		logger, err := resolve[*zap.Logger](resolver)
		if err != nil {
			return nil, err
		}
		projects, err := resolve[syncservice.ProjectRepository](resolver)
		if err != nil {
			return nil, err
		}
		sources, err := resolve[*materialize.Service](resolver)
		if err != nil {
			return nil, err
		}
		workspace, err := resolve[syncservice.WorkspaceRepository](resolver)
		if err != nil {
			return nil, err
		}
		return syncservice.New(logger.Named("service.sync"), projects, sources, workspace), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideLintService() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*lintservice.Service, error) {
		logger, err := resolve[*zap.Logger](resolver)
		if err != nil {
			return nil, err
		}
		projects, err := resolve[lintservice.ProjectRepository](resolver)
		if err != nil {
			return nil, err
		}
		contracts, err := resolve[lintservice.ContractLocator](resolver)
		if err != nil {
			return nil, err
		}
		proto, err := resolve[lintservice.ProtoLinter](resolver)
		if err != nil {
			return nil, err
		}
		inputs, err := resolve[lintservice.TargetResolver](resolver)
		if err != nil {
			return nil, err
		}
		return lintservice.New(logger.Named("service.lint"), lintservice.Dependencies{
			Projects: projects, Contracts: contracts, Inputs: inputs, Proto: proto,
		}), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideGenService() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*generateservice.Service, error) {
		logger, err := resolve[*zap.Logger](resolver)
		if err != nil {
			return nil, err
		}
		projects, err := resolve[generateservice.ProjectRepository](resolver)
		if err != nil {
			return nil, err
		}
		generator, err := resolve[generateservice.GeneratorClient](resolver)
		if err != nil {
			return nil, err
		}
		inputs, err := resolve[generateservice.TargetResolver](resolver)
		if err != nil {
			return nil, err
		}
		workspace, err := resolve[generateservice.WorkspaceRepository](resolver)
		if err != nil {
			return nil, err
		}
		return generateservice.New(logger.Named("service.gen"), generateservice.Dependencies{
			Projects: projects, Inputs: inputs, Generator: generator, Workspace: workspace,
		}), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
