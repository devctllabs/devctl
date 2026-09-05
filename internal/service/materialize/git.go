package materialize

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
)

// GitService reads a contract closure from a temporary repository checkout.
type GitService struct {
	client GitClient
	reader FileReader
}

func NewGit(client GitClient, reader FileReader) *GitService {
	return &GitService{client: client, reader: reader}
}

func (s *GitService) SourceType() project.SourceType { return project.SourceGit }

func (s *GitService) Materialize(ctx context.Context, request materializedomain.Request) (contract.Snapshot, error) {
	var snapshot contract.Snapshot
	err := s.client.WithCheckout(ctx, request.Source.Repo, request.Source.Ref, func(root string) error {
		if request.Source.Path != "" {
			if !safeRelative(request.Source.Path) {
				return &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, Path: request.Source.Path, Kind: materializedomain.FailureInvalid}
			}
			root = filepath.Join(root, filepath.FromSlash(request.Source.Path))
		}
		var materializeErr error
		if request.Reference.Format == "proto" {
			snapshot, materializeErr = protoTree(ctx, s.reader, protoTreeRequest{
				root: root, reference: request.Reference, bufConfig: request.Source.Proto.BufConfig,
			})
		} else {
			snapshot, materializeErr = referenceClosure(ctx, s.reader, root, request.Reference.Entrypoint)
		}
		return materializeErr
	})
	if err != nil {
		operationErr := &materializedomain.OperationError{Operation: materializedomain.OperationCheckout, SourceType: project.SourceGit, Kind: materializedomain.FailureUnavailable, Cause: err}
		return snapshot, fmt.Errorf("client.WithCheckout: %w", operationErr)
	}
	return snapshot, nil
}
