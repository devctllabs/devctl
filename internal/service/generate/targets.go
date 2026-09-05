package generate

import (
	"fmt"
	"path"
	"sort"

	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

func generationTargets(spec projectdomain.Manifest, family, selected string) ([]projectdomain.Target, error) {
	targets, err := projectdomain.NewTargetCatalog(spec).Resolve(projectdomain.TargetOperationGenerate, family, selected)
	if err != nil {
		return nil, fmt.Errorf("catalog.Resolve: %w", err)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		left, right := generationOrder(targets[i]), generationOrder(targets[j])
		if left != right {
			return left < right
		}
		return targets[i].ID < targets[j].ID
	})
	return targets, nil
}

func generationOrder(target projectdomain.Target) int {
	switch target.Family {
	case "config":
		return 0
	case "http":
		if target.Role == "server" {
			return 10
		}
		return 11
	case "grpc":
		if target.Role == "server" {
			return 20
		}
		return 21
	case "kafka":
		if target.Role == "consumer" {
			return 30
		}
		return 31
	default:
		return 100
	}
}

func plannedGeneration(target projectdomain.Target) []generatedomain.Change {
	if target.Family == "kafka" && target.Format == "raw" {
		return nil
	}
	changes := []generatedomain.Change{{Target: target.ID, Path: path.Join(target.OutputDir, target.OutputFile), Action: generatedomain.ChangePlannedPublish}}
	if target.Family == "config" {
		changes = append(changes, generatedomain.Change{Target: target.ID, Path: ".env.example", Action: generatedomain.ChangePlannedPublish})
	}
	return changes
}
