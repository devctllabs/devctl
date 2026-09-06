package sync

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	"github.com/devctllabs/devctl/internal/domain/contract"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	syncdomain "github.com/devctllabs/devctl/internal/domain/sync"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/sync_service.go -package mocks -typed . ProjectRepository,Materializer,WorkspaceRepository

// ProjectRepository resolves the valid project selected for synchronization.
type ProjectRepository interface {
	// LoadProject returns a structurally and semantically valid project or an execution error.
	LoadProject(ctx context.Context, manifestPath string) (projectdomain.Project, error)
}

// Materializer obtains exact contract closures from configured sources.
type Materializer interface {
	// Materialize resolves reference from source without publishing managed output.
	Materialize(ctx context.Context, root string, source projectdomain.Source, reference contract.Reference) (contract.Snapshot, error)
}

// WorkspaceRepository atomically publishes target snapshots and removes stale target directories.
type WorkspaceRepository interface {
	// PublishFile atomically publishes one contained file and reports whether bytes changed.
	PublishFile(ctx context.Context, root, target string, content []byte) (artifact.PublishResult, error)
	// PublishDirectory atomically replaces one contained target with the complete tree and reports whether content changed.
	PublishDirectory(ctx context.Context, root, target string, tree artifact.Tree) (artifact.PublishResult, error)
	// PruneDirectories removes child directories below parent except names in keep and returns removed child names.
	PruneDirectories(ctx context.Context, root, parent string, keep []string) ([]string, error)
	// PreviewPruneDirectories returns the same validated stale child set without mutating the workspace.
	PreviewPruneDirectories(ctx context.Context, root, parent string, keep []string) ([]string, error)
}

type Service struct {
	logger    *zap.Logger
	projects  ProjectRepository
	sources   Materializer
	workspace WorkspaceRepository
}

func New(logger *zap.Logger, projects ProjectRepository, sources Materializer, workspace WorkspaceRepository) *Service {
	return &Service{logger: logger, projects: projects, sources: sources, workspace: workspace}
}

type materializationKey struct {
	source    projectdomain.Source
	reference contract.Reference
}

type syncExecution struct {
	project  projectdomain.Project
	dryRun   bool
	resolved map[materializationKey]artifact.Tree
}

// Sync materializes and publishes selected targets sequentially in deterministic order.
// Dry-run performs no source materialization or filesystem mutation; a failure returns completed changes without rollback.
func (s *Service) Sync(ctx context.Context, command syncdomain.Command) (syncdomain.Result, error) {
	result := syncdomain.Result{Targets: []string{}, Changes: []syncdomain.Change{}, DryRun: command.DryRun}
	project, err := s.projects.LoadProject(ctx, command.ManifestPath)
	if err != nil {
		return result, fmt.Errorf("projects.LoadProject: %w", err)
	}
	targets, err := syncTargets(project.Manifest, command.Family, command.Target)
	if err != nil {
		return result, fmt.Errorf("syncTargets: %w", err)
	}
	execution := syncExecution{
		project:  project,
		dryRun:   command.DryRun,
		resolved: make(map[materializationKey]artifact.Tree),
	}
	for _, target := range targets {
		id, change, err := s.syncTarget(ctx, execution, target)
		if err != nil {
			return result, err
		}
		result.Targets = append(result.Targets, id)
		if change != nil {
			result.Changes = append(result.Changes, *change)
		}
	}
	if command.Target == "" {
		changes, err := s.pruneStaleTargets(ctx, project, command.Family, command.DryRun)
		if err != nil {
			return result, err
		}
		result.Changes = append(result.Changes, changes...)
	}
	s.logger.Debug("source synchronization completed", zap.Int("targets", len(result.Targets)))
	return result, nil
}

func (s *Service) syncTarget(
	ctx context.Context,
	execution syncExecution,
	target projectdomain.Target,
) (string, *syncdomain.Change, error) {
	id := target.ID
	if err := ctx.Err(); err != nil {
		return "", nil, fmt.Errorf("ctx.Err: %w", err)
	}
	if target.Source.Type == projectdomain.SourceLocal {
		return id, nil, nil
	}
	destination := target.Location.RelativePath
	if execution.dryRun {
		return id, &syncdomain.Change{Target: id, Path: destination, Action: syncdomain.ChangePlannedPublish}, nil
	}
	files, err := s.materializedFiles(ctx, execution.project.Root, target, execution.resolved)
	if err != nil {
		return "", nil, err
	}
	published, err := s.workspace.PublishDirectory(ctx, execution.project.Root, destination, files)
	if err != nil {
		operationErr := &syncdomain.OperationError{Operation: syncdomain.OperationPublish, Target: id, Path: destination, Kind: syncdomain.FailureUnavailable, Cause: err}
		return "", nil, fmt.Errorf("workspace.PublishDirectory: %w", operationErr)
	}
	action := syncdomain.ChangeAction(published.Action)
	return id, &syncdomain.Change{Target: id, Path: destination, Action: action}, nil
}

func (s *Service) materializedFiles(
	ctx context.Context,
	projectRoot string,
	target projectdomain.Target,
	resolved map[materializationKey]artifact.Tree,
) (artifact.Tree, error) {
	reference := target.Reference
	key := materializationKey{source: target.Source, reference: reference}
	if files, cached := resolved[key]; cached {
		return files, nil
	}
	snapshot, err := s.sources.Materialize(ctx, projectRoot, target.Source, reference)
	if err != nil {
		operationErr := &syncdomain.OperationError{
			Operation: syncdomain.OperationMaterialize,
			Target:    target.ID, Source: target.SourceName, Path: target.Reference.Entrypoint,
			Kind: syncdomain.FailureUnavailable, Cause: err,
		}
		return artifact.Tree{}, fmt.Errorf("sources.Materialize: %w", operationErr)
	}
	files := managedTree(snapshot)
	resolved[key] = files
	return files, nil
}

func managedTree(snapshot contract.Snapshot) artifact.Tree {
	files := make([]artifact.File, 0, len(snapshot.Files)+1)
	for _, file := range snapshot.Files {
		files = append(files, artifact.File{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: file.Mode})
	}
	if snapshot.Metadata != nil {
		content, err := json.Marshal(snapshot.Metadata)
		if err == nil {
			content = append(content, '\n')
			files = append(files, artifact.File{Path: ".devctl-contract.json", Content: content, Mode: 0o644})
		}
	}
	return artifact.Tree{Files: files}
}
