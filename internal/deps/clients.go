package deps

import (
	"fmt"

	bufgenclient "github.com/devctllabs/devctl/internal/client/bufgen"
	generatorclient "github.com/devctllabs/devctl/internal/client/generator"
	gitclient "github.com/devctllabs/devctl/internal/client/git"
	httpclient "github.com/devctllabs/devctl/internal/client/http"
	jsonschemaclient "github.com/devctllabs/devctl/internal/client/jsonschema"
	oapicodegenclient "github.com/devctllabs/devctl/internal/client/oapicodegen"
	"github.com/devctllabs/devctl/internal/client/toolrun"
	generateservice "github.com/devctllabs/devctl/internal/service/generate"
	lintservice "github.com/devctllabs/devctl/internal/service/lint"
	"github.com/devctllabs/devctl/internal/service/materialize"
	"github.com/devctllabs/go-libs/di"
)

func (c *Container) provideClients() error {
	providers := []func() error{
		c.provideToolRunner,
		c.provideMaterializeClients,
		c.provideGeneratorClients,
	}
	for _, provide := range providers {
		if err := provide(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Container) provideToolRunner() error {
	if err := di.Provide(c.di, func(di.Resolver) (*toolrun.OSRunner, error) {
		return toolrun.New(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (toolrun.Runner, error) {
		return resolve[*toolrun.OSRunner](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideMaterializeClients() error {
	if err := di.Provide(c.di, func(di.Resolver) (*httpclient.Client, error) {
		return httpclient.New(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (materialize.HTTPClient, error) {
		return resolve[*httpclient.Client](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(di.Resolver) (*gitclient.Client, error) {
		return gitclient.New(), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (materialize.GitClient, error) {
		return resolve[*gitclient.Client](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}

func (c *Container) provideGeneratorClients() error {
	if err := di.Provide(c.di, func(resolver di.Resolver) (*oapicodegenclient.Client, error) {
		runner, err := resolve[toolrun.Runner](resolver)
		if err != nil {
			return nil, err
		}
		return oapicodegenclient.New(runner), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (*bufgenclient.Client, error) {
		runner, err := resolve[toolrun.Runner](resolver)
		if err != nil {
			return nil, err
		}
		return bufgenclient.New(runner), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (*jsonschemaclient.Client, error) {
		runner, err := resolve[toolrun.Runner](resolver)
		if err != nil {
			return nil, err
		}
		return jsonschemaclient.New(runner), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (generateservice.GeneratorClient, error) {
		openAPI, err := resolve[*oapicodegenclient.Client](resolver)
		if err != nil {
			return nil, err
		}
		proto, err := resolve[*bufgenclient.Client](resolver)
		if err != nil {
			return nil, err
		}
		jsonSchema, err := resolve[*jsonschemaclient.Client](resolver)
		if err != nil {
			return nil, err
		}
		return generatorclient.New(generatorclient.Adapters{
			OpenAPI: openAPI, Proto: proto, JSONSchema: jsonSchema,
		}), nil
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	if err := di.Provide(c.di, func(resolver di.Resolver) (lintservice.ProtoLinter, error) {
		return resolve[*bufgenclient.Client](resolver)
	}); err != nil {
		return fmt.Errorf("di.Provide: %w", err)
	}
	return nil
}
