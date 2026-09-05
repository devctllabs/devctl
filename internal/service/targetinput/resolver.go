package targetinput

import (
	"context"
	"fmt"

	"github.com/devctllabs/devctl/internal/domain/contract"
	"github.com/devctllabs/devctl/internal/domain/project"
)

//go:generate go tool mockgen -destination mocks/resolver.go -package mocks -typed . EntrypointResolver,SnapshotLoader

// EntrypointResolver resolves a concrete contract entrypoint within a contained location.
type EntrypointResolver interface {
	// ResolveContract returns the contained entrypoint selected by location.
	ResolveContract(ctx context.Context, location contract.Location) (string, error)
}

// SnapshotLoader reconstructs one committed Contract Snapshot without contacting its supplier.
type SnapshotLoader interface {
	// Load reconstructs the Snapshot rooted at treeRoot and validates it against expected.
	Load(ctx context.Context, root, treeRoot string, expected contract.MetadataExpectation) (contract.Snapshot, error)
}

// Resolver attaches concrete local input to a logical Target.
type Resolver struct {
	entrypoints EntrypointResolver
	snapshots   SnapshotLoader
}

func New(entrypoints EntrypointResolver, snapshots SnapshotLoader) *Resolver {
	return &Resolver{entrypoints: entrypoints, snapshots: snapshots}
}

// Resolve attaches the concrete input required to execute target in selected Project.
func (r *Resolver) Resolve(
	ctx context.Context,
	selected project.Project,
	target project.Target,
) (project.Target, error) {
	if target.Source.Type == project.SourceDevctl && (target.Family == "grpc" || target.Family == "kafka") {
		snapshot, err := r.snapshots.Load(
			ctx, selected.Root, target.Location.RelativePath, target.SnapshotExpectation(),
		)
		if err != nil {
			return target, fmt.Errorf("snapshots.Load: %w", err)
		}
		return target.WithSnapshot(snapshot), nil
	}
	if target.Family != "http" {
		return target, nil
	}
	location := target.Location
	location.Root = selected.Root
	input, err := r.entrypoints.ResolveContract(ctx, location)
	if err != nil {
		return target, fmt.Errorf("entrypoints.ResolveContract: %w", err)
	}
	target.Input = input
	return target, nil
}
