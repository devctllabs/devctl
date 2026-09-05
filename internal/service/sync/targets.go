package sync

import (
	"fmt"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

func syncTargets(spec projectdomain.Manifest, family, selected string) ([]projectdomain.Target, error) {
	targets, err := projectdomain.NewTargetCatalog(spec).Resolve(projectdomain.TargetOperationSync, family, selected)
	if err != nil {
		return nil, fmt.Errorf("catalog.Resolve: %w", err)
	}
	return targets, nil
}

func externalContractsRoot(spec projectdomain.Manifest) string {
	if spec.Paths.ExternalContracts != "" {
		return spec.Paths.ExternalContracts
	}
	return "api/external"
}
