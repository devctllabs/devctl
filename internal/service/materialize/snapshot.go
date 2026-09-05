package materialize

import (
	"path"
	"sort"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
)

// newSnapshot copies, normalizes, and sorts files while requiring unique contained paths and a present entrypoint.
func newSnapshot(entrypoint string, files []contract.File) (contract.Snapshot, error) {
	normalized, seen, err := normalizeSnapshotFiles(files)
	if err != nil {
		return contract.Snapshot{}, err
	}
	entrypoint = cleanSnapshotPath(entrypoint)
	if _, exists := seen[entrypoint]; !exists {
		return contract.Snapshot{}, invalidSnapshotPath(entrypoint)
	}
	return contract.Snapshot{Entrypoint: entrypoint, Files: normalized}, nil
}

func newProtoSnapshot(moduleRoot, entrypoint string, files []contract.File) (contract.Snapshot, error) {
	normalized, seen, err := normalizeSnapshotFiles(files)
	if err != nil {
		return contract.Snapshot{}, err
	}
	moduleRoot = cleanSnapshotPath(moduleRoot)
	if !safeRelativeOrCurrent(moduleRoot) {
		return contract.Snapshot{}, invalidSnapshotPath(moduleRoot)
	}
	if entrypoint != "" {
		entrypoint = cleanSnapshotPath(entrypoint)
		if _, exists := seen[entrypoint]; !exists {
			return contract.Snapshot{}, invalidSnapshotPath(entrypoint)
		}
	}
	return contract.Snapshot{ModuleRoot: moduleRoot, Entrypoint: entrypoint, Files: normalized}, nil
}

func normalizeSnapshotFiles(files []contract.File) ([]contract.File, map[string]struct{}, error) {
	normalized := make([]contract.File, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		name := cleanSnapshotPath(file.Path)
		if !safeRelative(name) {
			return nil, nil, invalidSnapshotPath(name)
		}
		if _, exists := seen[name]; exists {
			return nil, nil, invalidSnapshotPath(name)
		}
		seen[name] = struct{}{}
		normalized = append(normalized, contract.File{Path: name, Content: append([]byte(nil), file.Content...), Mode: file.Mode})
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })
	return normalized, seen, nil
}

func cleanSnapshotPath(name string) string {
	return path.Clean(strings.ReplaceAll(name, "\\", "/"))
}

func safeRelativeOrCurrent(name string) bool {
	return name == "." || safeRelative(name)
}

func invalidSnapshotPath(name string) error {
	return &materializedomain.OperationError{
		Operation: materializedomain.OperationBuildSnapshot,
		Path:      name,
		Kind:      materializedomain.FailureInvalid,
	}
}
