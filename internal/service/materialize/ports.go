package materialize

import (
	"context"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
)

//go:generate go tool mockgen -destination mocks/ports.go -package mocks -typed . FileReader,HTTPClient,GitClient,ManifestRepository

// FileReader reads contained files for snapshot construction.
type FileReader interface {
	// ReadFile returns a regular non-symlink file below root with its relative path and permission bits.
	ReadFile(ctx context.Context, root, name string) (contract.File, error)
	// ReadTree returns every regular non-symlink file below a contained directory in deterministic order.
	ReadTree(ctx context.Context, root, directory string) ([]contract.File, error)
}

// HTTPClient retrieves remote contract bytes.
type HTTPClient interface {
	// Fetch retrieves request.URL and returns its effective URL after redirects.
	// Request.OriginURL defines the scheme, host, and effective port that every request must retain.
	Fetch(ctx context.Context, request materializedomain.HTTPFetchRequest) (materializedomain.HTTPDocument, error)
}

// GitClient provides a temporary checkout for the lifetime of one callback.
type GitClient interface {
	// WithCheckout invokes use with repository at ref and cleans the worktree after use succeeds or fails.
	WithCheckout(ctx context.Context, repository, ref string, use func(root string) error) error
}

// ManifestRepository reads upstream Devctl manifests by exact path.
type ManifestRepository interface {
	// Load returns structural manifest issues as data and access failures as errors.
	Load(ctx context.Context, manifestPath string) (project.LoadManifestResult, error)
}
