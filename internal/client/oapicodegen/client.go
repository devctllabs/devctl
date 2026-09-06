package oapicodegen

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/devctllabs/devctl/internal/client/toolsafety"
	"github.com/devctllabs/devctl/internal/domain/artifact"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

// Client runs the project-declared oapi-codegen tool in a temporary output workspace.
type Client struct {
	runner toolrun.Runner
}

func New(runner toolrun.Runner) *Client { return &Client{runner: runner} }

// Generate validates contained inputs and returns unpublished generated output.
func (c *Client) Generate(
	ctx context.Context,
	project projectdomain.Project,
	target projectdomain.Target,
) (generatedomain.Output, error) {
	if err := ctx.Err(); err != nil {
		return generatedomain.Output{}, fmt.Errorf("ctx.Err: %w", err)
	}
	if target.Family != "http" {
		return generatedomain.Output{}, fmt.Errorf("unsupported generation target kind %q", target.Family)
	}
	return c.generateHTTP(ctx, project.Root, target)
}

func (c *Client) generateHTTP(ctx context.Context, root string, target projectdomain.Target) (generatedomain.Output, error) {
	if target.OutputFile == "" || filepath.Base(target.OutputFile) != target.OutputFile {
		return generatedomain.Output{}, fmt.Errorf("invalid generated output filename %q", target.OutputFile)
	}
	if _, err := toolsafety.ReadRegularFile(root, target.Input); err != nil {
		return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile input: %w", err)
	}
	if _, err := toolsafety.ReadRegularFile(root, target.Config); err != nil {
		return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile config: %w", err)
	}
	goMod, err := toolsafety.ReadRegularFile(root, "go.mod")
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile go.mod: %w", err)
	}
	if !bytes.Contains(goMod.Content, []byte("tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen")) {
		return generatedomain.Output{}, fmt.Errorf("go.mod does not declare the oapi-codegen tool")
	}

	output, err := toolrun.WithTemporaryOutput(ctx, "devctl-generate-", func(temporary string) (generatedomain.Output, error) {
		outputPath := filepath.Join(temporary, target.OutputFile)
		command := toolrun.Command{
			Name: "go", Args: []string{"tool", "oapi-codegen", "--config", target.Config, "-o", outputPath, target.Input}, Dir: root,
		}
		if err := c.runner.Run(ctx, command); err != nil {
			return generatedomain.Output{}, fmt.Errorf("runner.Run: %w", err)
		}
		generated, err := toolsafety.ReadRegularFile(temporary, target.OutputFile)
		if err != nil {
			return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile output: %w", err)
		}
		if !bytes.Contains(generated.Content, []byte("Code generated")) {
			return generatedomain.Output{}, fmt.Errorf("oapi-codegen did not create a canonical generated Go file")
		}
		return generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{Path: target.OutputFile, Content: generated.Content, Mode: 0o644}}}}, nil
	})
	if err != nil {
		return output, fmt.Errorf("toolrun.WithTemporaryOutput: %w", err)
	}
	return output, nil
}
