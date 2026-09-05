// Package toolrun executes project-owned tools without assigning tool-specific policy.
package toolrun

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/devctllabs/devctl/internal/client/toolsafety"
)

// Command describes one local process invocation.
type Command struct {
	Name string
	Args []string
	Dir  string
}

// Runner executes local tool commands.
type Runner interface {
	Run(ctx context.Context, command Command) error
}

// OSRunner executes commands through the operating system.
type OSRunner struct{}

// New returns an operating-system-backed Runner.
func New() *OSRunner { return &OSRunner{} }

// Run executes command and includes bounded combined output in failures.
func (*OSRunner) Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Dir = command.Dir
	output, err := process.CombinedOutput()
	if err == nil {
		return nil
	}
	if cancellation := ctx.Err(); cancellation != nil {
		err = cancellation
	}
	return fmt.Errorf("command.CombinedOutput: %s: %w", toolsafety.BoundedOutput(output), err)
}
