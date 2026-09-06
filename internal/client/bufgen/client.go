package bufgen

import (
	"context"
	"fmt"

	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/devctllabs/devctl/internal/client/toolsafety"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"golang.org/x/mod/modfile"
)

// Client runs the project-declared Buf tool in a temporary output workspace.
type Client struct {
	runner toolrun.Runner
}

func New(runner toolrun.Runner) *Client { return &Client{runner: runner} }

// Generate returns unpublished generated Proto output.
func (c *Client) Generate(
	ctx context.Context,
	project projectdomain.Project,
	target projectdomain.Target,
) (generatedomain.Output, error) {
	if err := ctx.Err(); err != nil {
		return generatedomain.Output{}, fmt.Errorf("ctx.Err: %w", err)
	}
	if target.Family != "grpc" && target.Family != "kafka" {
		return generatedomain.Output{}, fmt.Errorf("unsupported generation target kind %q", target.Family)
	}
	if err := toolsafety.RequireDirectory(project.Root, target.Input); err != nil {
		return generatedomain.Output{}, fmt.Errorf("toolsafety.RequireDirectory input: %w", err)
	}
	if _, err := toolsafety.ReadRegularFile(project.Root, target.Config); err != nil {
		return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile config: %w", err)
	}
	if err := requireBufTool(project.Root); err != nil {
		return generatedomain.Output{}, err
	}

	output, err := toolrun.WithTemporaryOutput(ctx, "devctl-buf-generate-", func(temporary string) (generatedomain.Output, error) {
		arguments := []string{"tool", "buf", "generate", target.Input, "--template", target.Config}
		for _, selectedPath := range target.Paths {
			arguments = append(arguments, "--path", selectedPath)
		}
		arguments = append(arguments, "--output", temporary)
		if err := c.runner.Run(ctx, toolrun.Command{Name: "go", Args: arguments, Dir: project.Root}); err != nil {
			return generatedomain.Output{}, fmt.Errorf("runner.Run: %w", err)
		}
		tree, err := toolsafety.ReadRegularTree(temporary, ".")
		if err != nil {
			return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularTree: %w", err)
		}
		if len(tree.Files) == 0 {
			return generatedomain.Output{}, fmt.Errorf("buf produced no generated files")
		}
		return generatedomain.Output{Directory: tree}, nil
	})
	if err != nil {
		return output, fmt.Errorf("toolrun.WithTemporaryOutput: %w", err)
	}
	return output, nil
}

// Lint checks one Proto-backed target with the project-declared Buf tool.
func (c *Client) Lint(ctx context.Context, project projectdomain.Project, target projectdomain.Target) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ctx.Err: %w", err)
	}
	if target.Family != "grpc" && target.Family != "kafka" {
		return fmt.Errorf("unsupported lint target kind %q", target.Family)
	}
	if err := toolsafety.RequireDirectory(project.Root, target.Input); err != nil {
		return fmt.Errorf("toolsafety.RequireDirectory input: %w", err)
	}
	if err := requireBufTool(project.Root); err != nil {
		return err
	}
	arguments := []string{"tool", "buf", "lint", target.Input}
	for _, selectedPath := range target.Paths {
		arguments = append(arguments, "--path", selectedPath)
	}
	if err := c.runner.Run(ctx, toolrun.Command{Name: "go", Args: arguments, Dir: project.Root}); err != nil {
		return fmt.Errorf("runner.Run: %w", err)
	}
	return nil
}

func requireBufTool(root string) error {
	goMod, err := toolsafety.ReadRegularFile(root, "go.mod")
	if err != nil {
		return fmt.Errorf("toolsafety.ReadRegularFile go.mod: %w", err)
	}
	parsed, err := modfile.Parse("go.mod", goMod.Content, nil)
	if err != nil {
		return fmt.Errorf("modfile.Parse: %w", err)
	}
	for _, tool := range parsed.Tool {
		if tool.Path == "github.com/bufbuild/buf/cmd/buf" {
			return nil
		}
	}
	return fmt.Errorf("go.mod does not declare the Buf tool")
}
