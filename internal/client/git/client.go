package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client provides detached temporary Git checkouts without source-selection policy.
type Client struct{}

func New() *Client { return &Client{} }

// WithCheckout invokes use with a detached checkout and always attempts cleanup before returning.
// Callback and cleanup failures are both preserved.
func (c *Client) WithCheckout(ctx context.Context, repository, ref string, use func(root string) error) error {
	workspace, err := os.MkdirTemp("", "devctl-source-")
	if err != nil {
		return fmt.Errorf("os.MkdirTemp: %w", err)
	}
	worktree := &Worktree{root: filepath.Join(workspace, "repository"), workspace: workspace}
	if err := run(ctx, "git", "clone", "--quiet", "--no-checkout", "--", repository, worktree.root); err != nil {
		return errors.Join(err, worktree.Close())
	}
	if err := run(ctx, "git", "-C", worktree.root, "checkout", "--quiet", "--detach", ref); err != nil {
		return errors.Join(err, worktree.Close())
	}
	return errors.Join(use(worktree.root), worktree.Close())
}

// Worktree owns one temporary checkout and its surrounding temporary directory.
type Worktree struct {
	root      string
	workspace string
	closed    bool
}

// Root returns the checkout directory, which remains valid until Close.
func (w *Worktree) Root() string { return w.root }

// Close attempts to remove the complete temporary workspace at most once.
func (w *Worktree) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if err := os.RemoveAll(w.workspace); err != nil {
		return fmt.Errorf("os.RemoveAll: %w", err)
	}
	return nil
}

func run(ctx context.Context, executable string, arguments ...string) error {
	command := exec.CommandContext(ctx, executable, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command.CombinedOutput: %s: %w", bounded(output), err)
	}
	return nil
}

func bounded(data []byte) string {
	const limit = 1024
	value := strings.TrimSpace(string(data))
	if len(value) > limit {
		return value[:limit] + "…"
	}
	return value
}
