package scaffold

import (
	"context"
	"io/fs"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

//go:generate go tool mockgen -destination mocks/ports.go -package mocks -typed . ProjectRepository,WorkspaceRepository

// ProjectRepository resolves the valid project selected for scaffolding.
type ProjectRepository interface {
	// LoadProject returns a structurally and semantically valid project or an execution error.
	LoadProject(ctx context.Context, manifestPath string) (projectdomain.Project, error)
}

// WorkspaceRepository exposes contained file facts and per-file atomic publication.
type WorkspaceRepository interface {
	// Walk visits project entries below root without escaping its containment boundary.
	Walk(ctx context.Context, root string, visit fs.WalkDirFunc) error
	// Lstat reports file metadata without following the final symlink named by name.
	Lstat(ctx context.Context, root, name string) (fs.FileInfo, error)
	// ReadBytes reads a contained project file below root.
	ReadBytes(ctx context.Context, root, name string) ([]byte, error)
	// PublishFile atomically publishes content at target below root and reports whether bytes changed.
	PublishFile(ctx context.Context, root, target string, content []byte) (artifact.PublishResult, error)
}
