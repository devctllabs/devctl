package materialize

import (
	"context"
	"fmt"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"github.com/devctllabs/devctl/internal/domain/project"
)

// Strategy materializes exactly one project source type.
type Strategy interface {
	// SourceType returns the single source type owned by the strategy.
	SourceType() project.SourceType
	// Materialize returns the selected exact contract closure without publishing it.
	Materialize(ctx context.Context, request materializedomain.Request) (contract.Snapshot, error)
}

type Service struct {
	strategies map[project.SourceType]Strategy
}

// New builds an immutable strategy router from any subset and rejects duplicate source types.
func New(strategies ...Strategy) (*Service, error) {
	byType := make(map[project.SourceType]Strategy, len(strategies))
	for _, strategy := range strategies {
		sourceType := strategy.SourceType()
		if _, exists := byType[sourceType]; exists {
			return nil, &materializedomain.OperationError{Operation: materializedomain.OperationConfigureRouter, SourceType: sourceType, Kind: materializedomain.FailureInvalid}
		}
		byType[sourceType] = strategy
	}
	return &Service{strategies: byType}, nil
}

// Materialize routes request by source type and reports an unsupported source when no strategy was configured.
func (s *Service) Materialize(
	ctx context.Context,
	root string,
	source project.Source,
	reference contract.Reference,
) (contract.Snapshot, error) {
	request := materializedomain.Request{Root: root, Source: source, Reference: reference}
	strategy, exists := s.strategies[source.Type]
	if !exists {
		return contract.Snapshot{}, &materializedomain.UnsupportedSourceError{SourceType: source.Type}
	}
	snapshot, err := strategy.Materialize(ctx, request)
	if err != nil {
		return snapshot, fmt.Errorf("strategy.Materialize: %w", err)
	}
	return snapshot, nil
}
