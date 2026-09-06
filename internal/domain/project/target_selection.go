package project

import "github.com/devctllabs/devctl/internal/domain/failure"

// TargetSelectionReason identifies why a workflow target selection failed.
type TargetSelectionReason string

const (
	TargetSelectionUnknownFamily        TargetSelectionReason = "unknown_family"
	TargetSelectionTargetNotFound       TargetSelectionReason = "target_not_found"
	TargetSelectionOperationUnsupported TargetSelectionReason = "operation_unsupported"
)

// TargetSelectionError reports one presentation-neutral catalog selection fact.
type TargetSelectionError struct {
	Operation TargetOperation
	Family    string
	Target    string
	Reason    TargetSelectionReason
}

func (e *TargetSelectionError) Error() string { return "target selection failed" }

// Category maps selection policy to the stable transport-neutral failure contract.
func (e *TargetSelectionError) Category() failure.Category {
	switch e.Reason {
	case TargetSelectionUnknownFamily:
		return failure.InvalidInput
	case TargetSelectionTargetNotFound:
		return failure.NotFound
	case TargetSelectionOperationUnsupported:
		return failure.Unsupported
	default:
		return failure.Internal
	}
}

// Resolve applies the workflow selection contract and returns targets in catalog ID order.
func (c TargetCatalog) Resolve(operation TargetOperation, family, id string) ([]Target, error) {
	if family != "" && !knownTargetFamily(family) {
		return nil, &TargetSelectionError{
			Operation: operation, Family: family, Target: id, Reason: TargetSelectionUnknownFamily,
		}
	}
	if id == "" {
		return c.Select(operation, family, ""), nil
	}

	entry, exists := c.entry(id)
	if !exists || family != "" && entry.target.Family != family {
		return nil, &TargetSelectionError{
			Operation: operation, Family: family, Target: id, Reason: TargetSelectionTargetNotFound,
		}
	}
	mask := targetOperationMask(operation)
	if mask == 0 || entry.operations&mask == 0 {
		return nil, &TargetSelectionError{
			Operation: operation, Family: family, Target: id, Reason: TargetSelectionOperationUnsupported,
		}
	}
	return copyCatalogTargets([]targetCatalogEntry{entry}), nil
}

func (c TargetCatalog) entry(id string) (targetCatalogEntry, bool) {
	for _, entry := range c.entries {
		if entry.target.ID == id {
			return entry, true
		}
	}
	return targetCatalogEntry{}, false
}

func knownTargetFamily(family string) bool {
	switch family {
	case "config", "http", "grpc", "kafka":
		return true
	default:
		return false
	}
}
