package materialize

import (
	"context"
	"path/filepath"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
)

// LocalService reads a contract closure contained by the current project.
type LocalService struct {
	reader FileReader
}

func NewLocal(reader FileReader) *LocalService { return &LocalService{reader: reader} }

func (s *LocalService) SourceType() project.SourceType { return project.SourceLocal }

func (s *LocalService) Materialize(ctx context.Context, request materializedomain.Request) (contract.Snapshot, error) {
	if !safeRelative(request.Source.Path) {
		return contract.Snapshot{}, &materializedomain.OperationError{
			Operation: materializedomain.OperationValidateSource,
			Path:      request.Source.Path,
			Kind:      materializedomain.FailureInvalid,
		}
	}
	sourceRoot := filepath.Join(request.Root, filepath.FromSlash(request.Source.Path))
	if request.Reference.Format == "proto" {
		return protoTree(ctx, s.reader, protoTreeRequest{
			root: sourceRoot, reference: request.Reference, bufConfig: request.Source.Proto.BufConfig,
		})
	}
	return referenceClosure(ctx, s.reader, sourceRoot, request.Reference.Entrypoint)
}
