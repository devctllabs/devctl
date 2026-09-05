// Package generator routes generation Targets to their concrete tool adapters.
package generator

import (
	"context"
	"fmt"

	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

//go:generate go tool mockgen -destination mocks/client.go -package mocks -typed . Adapter

// Adapter generates unpublished Managed Output for one supported Target.
type Adapter interface {
	// Generate returns unpublished Managed Output without modifying the Project workspace.
	Generate(ctx context.Context, project projectdomain.Project, target projectdomain.Target) (generatedomain.Output, error)
}

// Adapters contains the concrete tool capabilities selected by Client.
type Adapters struct {
	OpenAPI    Adapter
	Proto      Adapter
	JSONSchema Adapter
}

// Client routes each supported Target to its concrete generator adapter.
type Client struct {
	adapters Adapters
}

// New returns a Target-aware generator using adapters.
func New(adapters Adapters) *Client {
	return &Client{adapters: adapters}
}

// Generate returns unpublished Managed Output for target.
func (c *Client) Generate(
	ctx context.Context,
	project projectdomain.Project,
	target projectdomain.Target,
) (generatedomain.Output, error) {
	adapter, name := c.adapter(target)
	if adapter == nil {
		return generatedomain.Output{}, fmt.Errorf("unsupported generation target %q", target.ID)
	}
	output, err := adapter.Generate(ctx, project, target)
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("%s.Generate: %w", name, err)
	}
	return output, nil
}

func (c *Client) adapter(target projectdomain.Target) (Adapter, string) {
	switch {
	case target.Family == "http":
		return c.adapters.OpenAPI, "openAPI"
	case target.Family == "grpc", target.Family == "kafka" && target.Format == "proto":
		return c.adapters.Proto, "proto"
	case target.Family == "kafka" && target.Format == "json":
		return c.adapters.JSONSchema, "jsonSchema"
	default:
		return nil, ""
	}
}
