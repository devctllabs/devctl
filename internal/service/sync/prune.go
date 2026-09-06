package sync

import (
	"context"
	"fmt"
	"path"
	"sort"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	syncdomain "github.com/devctllabs/devctl/internal/domain/sync"
)

func (s *Service) pruneStaleTargets(
	ctx context.Context,
	project projectdomain.Project,
	family string,
	preview bool,
) ([]syncdomain.Change, error) {
	type subtree struct{ family, role, targetPrefix string }
	subtrees := []subtree{
		{family: "http", role: "client", targetPrefix: "http-client:"},
		{family: "grpc", role: "client", targetPrefix: "grpc-client:"},
		{family: "kafka", role: "consumer", targetPrefix: "kafka-consumer:"},
		{family: "kafka", role: "producer", targetPrefix: "kafka-producer:"},
	}
	catalog := projectdomain.NewTargetCatalog(project.Manifest).Select(projectdomain.TargetOperationSync, "", "")
	changes := make([]syncdomain.Change, 0)
	for _, subtree := range subtrees {
		if family != "" && subtree.family != family {
			continue
		}
		keep := make([]string, 0)
		for _, target := range catalog {
			if target.Family == subtree.family && target.Role == subtree.role && target.Source.Type != projectdomain.SourceLocal {
				keep = append(keep, target.Name)
			}
		}
		sort.Strings(keep)
		parent := path.Join(externalContractsRoot(project.Manifest), subtree.family, subtree.role)
		removed, err := s.staleDirectories(ctx, staleDirectoryRequest{
			root: project.Root, parent: parent, keep: keep, preview: preview,
		})
		if err != nil {
			operationErr := &syncdomain.OperationError{Operation: syncdomain.OperationPrune, Path: parent, Kind: syncdomain.FailureUnavailable, Cause: err}
			return changes, fmt.Errorf("stale target selection: %w", operationErr)
		}
		sort.Strings(removed)
		action := syncdomain.ChangeRemoved
		if preview {
			action = syncdomain.ChangePlannedRemove
		}
		for _, name := range removed {
			changes = append(changes, syncdomain.Change{Target: subtree.targetPrefix + name, Path: path.Join(parent, name), Action: action})
		}
	}
	return changes, nil
}

type staleDirectoryRequest struct {
	root    string
	parent  string
	keep    []string
	preview bool
}

func (s *Service) staleDirectories(ctx context.Context, request staleDirectoryRequest) ([]string, error) {
	if request.preview {
		removed, err := s.workspace.PreviewPruneDirectories(ctx, request.root, request.parent, request.keep)
		if err != nil {
			return removed, fmt.Errorf("workspace.PreviewPruneDirectories: %w", err)
		}
		return removed, nil
	}
	removed, err := s.workspace.PruneDirectories(ctx, request.root, request.parent, request.keep)
	if err != nil {
		return removed, fmt.Errorf("workspace.PruneDirectories: %w", err)
	}
	return removed, nil
}
