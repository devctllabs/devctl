package deps

import (
	"fmt"

	manifestrepo "github.com/devctllabs/devctl/internal/repository/manifest"
	workspacerepo "github.com/devctllabs/devctl/internal/repository/workspace"
	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	"github.com/devctllabs/devctl/internal/service/materialize"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/projectreadiness"
	syncservice "github.com/devctllabs/devctl/internal/service/sync"
	"github.com/devctllabs/go-libs/di"
)

func (c *Container) provideRepositories() error {
	if err := di.Provide(c.di, func(di.Resolver) (*manifestrepo.FilesystemRepo, error) {
		return manifestrepo.NewFilesystemRepo(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (projectservice.ManifestRepository, error) {
		return resolve[*manifestrepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(di.Resolver) (*workspacerepo.FilesystemRepo, error) {
		return workspacerepo.NewFilesystemRepo(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (projectservice.ManifestLocator, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (projectreadiness.Workspace, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (syncservice.ProjectRepository, error) {
		return resolve[*projectservice.Service](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (syncservice.WorkspaceRepository, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (materialize.FileReader, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (materialize.ManifestRepository, error) {
		return resolve[*manifestrepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (generateservice.ProjectRepository, error) {
		return resolve[*projectservice.Service](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (generateservice.WorkspaceRepository, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (lintservice.ProjectRepository, error) {
		return resolve[*projectservice.Service](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (lintservice.ContractLocator, error) {
		return resolve[*workspacerepo.FilesystemRepo](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
