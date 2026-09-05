package jsonschema

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/devctllabs/devctl/internal/client/toolsafety"
	"github.com/devctllabs/devctl/internal/domain/artifact"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	platformjsonschema "github.com/devctllabs/devctl/internal/platform/jsonschema"
)

// Client runs project-pinned quicktype in a temporary output directory.
type Client struct {
	runner toolrun.Runner
}

func New(runner toolrun.Runner) *Client { return &Client{runner: runner} }

// Generate returns one parsed Go file without writing managed project output.
func (c *Client) Generate(ctx context.Context, project projectdomain.Project, target projectdomain.Target) (generatedomain.Output, error) {
	if err := ctx.Err(); err != nil {
		return generatedomain.Output{}, fmt.Errorf("ctx.Err: %w", err)
	}
	if target.Family != "kafka" || target.Format != "json" {
		return generatedomain.Output{}, fmt.Errorf("unsupported generation target %q", target.ID)
	}
	input, err := toolsafety.ReadRegularFile(project.Root, target.Input)
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile input: %w", err)
	}
	title, err := platformjsonschema.RootTitle(input.Content)
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("jsonschema.RootTitle: %w", err)
	}
	outputName := target.OutputFile
	if outputName == "" {
		outputName = "schema.gen.go"
	}
	if filepath.IsAbs(outputName) || filepath.Base(outputName) != outputName || outputName == "." {
		return generatedomain.Output{}, fmt.Errorf("invalid generated output filename %q", outputName)
	}
	quicktype, err := exec.LookPath("quicktype")
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("exec.LookPath: quicktype is unavailable; run mise install and execute generation through mise: %w", err)
	}
	output, err := toolrun.WithTemporaryOutput(ctx, "devctl-jsonschema-generate-", func(temporary string) (generatedomain.Output, error) {
		outputPath := filepath.Join(temporary, outputName)
		generatedPackage := packageName(target.ID)
		command := toolrun.Command{
			Name: quicktype,
			Args: []string{
				"--src", target.Input,
				"--src-lang", "schema",
				"--lang", "go",
				"--package", generatedPackage,
				"--top-level", title,
				"--out", outputPath,
				"--omit-empty",
			},
			Dir: project.Root,
		}
		if err := c.runner.Run(ctx, command); err != nil {
			return generatedomain.Output{}, fmt.Errorf("runner.Run: %w", err)
		}
		generated, err := toolsafety.ReadRegularFile(temporary, outputName)
		if err != nil {
			return generatedomain.Output{}, fmt.Errorf("toolsafety.ReadRegularFile output: %w", err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), outputName, generated.Content, parser.AllErrors)
		if err != nil {
			return generatedomain.Output{}, fmt.Errorf("parser.ParseFile: %w", err)
		}
		if parsed.Name.Name != generatedPackage {
			return generatedomain.Output{}, fmt.Errorf("generated package %q does not match %q", parsed.Name.Name, generatedPackage)
		}
		mode := uint32(generated.Mode.Perm())
		if mode == 0 {
			mode = 0o644
		}
		return generatedomain.Output{Directory: artifact.Tree{Files: []artifact.File{{Path: outputName, Content: generated.Content, Mode: mode}}}}, nil
	})
	if err != nil {
		return output, fmt.Errorf("toolrun.WithTemporaryOutput: %w", err)
	}
	return output, nil
}

func packageName(targetID string) string {
	name := targetID
	if _, selected, found := strings.Cut(targetID, ":"); found {
		name = selected
	}
	return strings.ReplaceAll(name, "-", "_")
}
